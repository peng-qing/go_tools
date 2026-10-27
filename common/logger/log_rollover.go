package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/peng-qing/go_tools/common/container"
	fileutils "github.com/peng-qing/go_tools/common/file_utils"
)

var (
	// 检查 LogFileRollover 实现 WriteCloser
	_ io.WriteCloser = (*LogFileRollover)(nil)
	// 检查 sortableLogFileMeta 实现 sort.Interface
	_ sort.Interface = (*sortableLogFileMeta)(nil)
)

const (
	// 压缩后缀
	compressSuffix = ".gz"
	// 默认单位 MB
	megaByte = 1024 * 1024
	// 默认文件分割大小 100MB
	defaultMaxFileSize = megaByte * 100
	// 追加文件不存在时文件后缀
	defaultFileNotExistSuffix = "-rollover.log"
	// 备份文件名格式化
	backupTimeFormat = "2006-01-02T15-04-05.000"
)

// LogFileMeta 日志文件元数据
type LogFileMeta struct {
	Time     time.Time
	FileInfo os.FileInfo
}

// 日志文件元数据排序 实现 sort.Interface
type sortableLogFileMeta []*LogFileMeta

// Len 排序数据长度 实现 sort.Interface
func (s sortableLogFileMeta) Len() int {
	return len(s)
}

// Less 变焦两个数据 实现 sort.Interface
func (s sortableLogFileMeta) Less(i, j int) bool {
	return s[i].Time.After(s[j].Time)
}

// Swap 交换两个数据 实现 sort.Interface
func (s sortableLogFileMeta) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// LogFileRollover 文件日志轮转
type LogFileRollover struct {
	// 文件名
	Filename string
	// 单位MB
	MaxSize int
	// 旧日志保留数量
	MaxBackups int
	// 日志保存时间 天(24h)
	MaxAge int
	// 是否执行压缩
	// 压缩后会被添加 .gz 后缀
	Compress bool
	// 源文件 通过日志分割器的日志会被追加到该文件
	// 如果初始长度超出 MaxSize 会被切割并重命名加上当前时间信息
	// 然后会使用原始文件名创建一个新日志文件
	file *os.File
	// 当前文件大小
	size int64
	// 锁
	mutex sync.Mutex
	// 负责切割文件协程控制
	once      sync.Once          // 确保只创建一个
	ctx       context.Context    // 控制协程退出
	ctxCancel context.CancelFunc // 上下文取消
	// 通知子协程进行切割&压缩信号
	rolloverChan chan struct{}
}

// NewLogFileRollover 实例化 LogFileRollover
func NewLogFileRollover(filename string, maxSize int, maxBackups int, maxAge int, compress bool) *LogFileRollover {
	return &LogFileRollover{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   compress,
	}
}

// Rotate 主动发起轮转日志
func (l *LogFileRollover) Rotate() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return l.rotate()
}

// Write 实现 io.Writer 接口
func (l *LogFileRollover) Write(p []byte) (n int, err error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// 单次写入大文件数据....
	rewriteSize := int64(len(p))
	if rewriteSize > l.maxFileSize() {
		return 0, fmt.Errorf("logger: write too large (%d>%d)", rewriteSize, l.maxFileSize())
	}

	// 文件不存在 尝试打开或创建目标文件
	if l.file == nil {
		if err := l.tryOpenOrCreateFile(rewriteSize); err != nil {
			return 0, err
		}
	}

	// 写入后超过单文件大小限制 轮转到新文件
	if rewriteSize+l.size > l.maxFileSize() {
		// 轮转到新文件
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = l.file.Write(p)
	l.size += int64(n)

	return n, err
}

// Close 实现 io.Closer 接口
func (l *LogFileRollover) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return l.close()
}

// Sync 实现 WriteSyncer 接口
func (l *LogFileRollover) Sync() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.file != nil {
		return l.file.Sync()
	}

	return nil
}

// maxFileSize 最大文件大小
func (l *LogFileRollover) maxFileSize() int64 {
	if l.MaxSize != 0 {
		return int64(l.MaxSize * megaByte)
	}
	return defaultMaxFileSize
}

// filename 获取正在写入日志文件名
func (l *LogFileRollover) filename() string {
	if l.Filename != "" {
		return l.Filename
	}

	// 默认路径 ${tmp_dir}/${project}-rollover.log
	filename := filepath.Base(os.Args[0]) + defaultFileNotExistSuffix
	return filepath.Join(os.TempDir(), filename)
}

// filePath 日志文件路径
func (l *LogFileRollover) filePath() string {
	return filepath.Dir(l.filename())
}

// tryOpenOrCreateFile 尝试创建新文件
func (l *LogFileRollover) tryOpenOrCreateFile(rewriteSize int64) error {
	filename := l.filename()
	info, err := os.Stat(filename)
	if err != nil {
		// 文件不存在 创建
		if os.IsNotExist(err) {
			return l.openNew()
		}
		return err
	}

	if info.Size()+rewriteSize >= l.maxFileSize() {
		// 文件大小达到限制 轮转到新文件
		return l.rotate()
	}

	// 尝试追加到当前文件
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// 追加失败 创建新文件
		return l.openNew()
	}

	l.file = file
	l.size = info.Size()

	return l.rollover()
}

// openNew 开启新文件
func (l *LogFileRollover) openNew() error {
	// 创建文件夹
	err := os.MkdirAll(l.filePath(), 0755)
	if err != nil {
		return err
	}
	name := l.filename()
	mode := os.FileMode(0644)

	info, err := os.Stat(name)
	if err == nil {
		// 老文件存在 改名 保持Mode不变
		mode = info.Mode()
		newName := getBackupNameByTime(name, backupTimeFormat)
		if err = os.Rename(name, newName); err != nil {
			return err
		}
	}
	// 截断
	newFile, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	l.file = newFile
	l.size = 0

	return nil
}

// rotate 日志轮转
func (l *LogFileRollover) rotate() error {
	var err error
	// 先关闭旧的
	if err = l.closeFile(); err != nil {
		return err
	}
	// 再开新的
	if err = l.openNew(); err != nil {
		return err
	}

	return l.rollover()
}

// closeFile 关闭文件
func (l *LogFileRollover) closeFile() error {
	if l.file != nil {
		return l.file.Close()
	}

	return nil
}

// rollover 开始日志轮转
func (l *LogFileRollover) rollover() error {
	l.once.Do(func() {
		l.rolloverChan = make(chan struct{}, 1)
		ctx, ctxCancel := context.WithCancel(context.Background())
		l.ctx = ctx
		l.ctxCancel = ctxCancel

		// 首次调用开启子协程进行文件切割
		go l.run()
	})

	// 通知子协程开始分割
	select {
	case l.rolloverChan <- struct{}{}:
	default:
	}

	return nil
}

// run 子协程逻辑
func (l *LogFileRollover) run() {
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-l.rolloverChan:
			_ = l.rolloverExec()
		}
	}
}

// rolloverExec 执行文件轮转
func (l *LogFileRollover) rolloverExec() error {
	if l.MaxAge == 0 && l.MaxBackups == 0 && !l.Compress {
		return nil
	}

	var err error
	logFileMetas, err := l.loadFileList()
	if err != nil {
		return err
	}

	// 需要被删除的数据
	removed := make([]*LogFileMeta, 0)
	// 移除超出备份数量的日志
	if l.MaxBackups > 0 {
		savedSet := container.NewSet[string]()
		remained := make([]*LogFileMeta, 0, len(logFileMetas))
		for _, logFileMeta := range logFileMetas {
			filename := strings.TrimSuffix(logFileMeta.FileInfo.Name(), compressSuffix)
			// .log 和对应的 .log.gz 按同一个备份计算
			if !savedSet.Contains(filename) {
				if savedSet.Size() >= l.MaxBackups {
					removed = append(removed, logFileMeta)
					continue
				}
				savedSet.Add(filename)
			}
			remained = append(remained, logFileMeta)
		}
		logFileMetas = remained
	}

	// 移除到期
	if l.MaxAge > 0 {
		remained := make([]*LogFileMeta, 0)
		// 截止时间
		diff := time.Duration(int64(24*l.MaxAge) * int64(time.Hour))
		cutOffTime := time.Now().Add(-1 * diff)
		for _, logFileMeta := range logFileMetas {
			// 早于截止时间 删除
			if logFileMeta.Time.Before(cutOffTime) {
				removed = append(removed, logFileMeta)
				continue
			}
			remained = append(remained, logFileMeta)
		}
		logFileMetas = remained
	}

	// 删除
	for _, logFileMeta := range removed {
		errRemove := os.Remove(filepath.Join(l.filePath(), logFileMeta.FileInfo.Name()))
		if errRemove != nil {
			err = errors.Join(err, errRemove)
			continue
		}
	}

	// 压缩
	if l.Compress {
		for _, logFileMeta := range logFileMetas {
			if strings.HasSuffix(logFileMeta.FileInfo.Name(), compressSuffix) {
				continue
			}
			// 压缩文件
			filename := filepath.Join(l.filePath(), logFileMeta.FileInfo.Name())
			errCompress := fileutils.CompressFileByGzip(filename, filename+compressSuffix)
			if errCompress != nil {
				err = errors.Join(err, errCompress)
				continue
			}
		}
	}

	return err
}

// close 关闭日志轮转器
func (l *LogFileRollover) close() error {
	if err := l.closeFile(); err != nil {
		return err
	}

	if l.ctxCancel != nil {
		l.ctxCancel()
		close(l.rolloverChan)
	}

	return nil
}

// loadFileList 获取文件夹下的由当前 WriteSyncer 产生的日志文件并返回按时间大到小排序的日志文件元数据
func (l *LogFileRollover) loadFileList() ([]*LogFileMeta, error) {
	entries, err := os.ReadDir(l.filePath())
	if err != nil {
		return nil, err
	}

	filename := filepath.Base(l.filename())
	ext := filepath.Ext(l.filename())
	// 前缀 path/file_
	prefix := strings.TrimSuffix(filename, ext) + "_"

	logFileMetas := make([]*LogFileMeta, 0)
	for _, entry := range entries {
		// 跳过文件夹
		if entry.IsDir() {
			continue
		}
		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}
		// 解析日志名
		if fileTime, err := timeFromFileName(fileInfo.Name(), prefix, ext, backupTimeFormat); err == nil {
			// 未压缩日志
			logFileMetas = append(logFileMetas, &LogFileMeta{
				Time:     fileTime,
				FileInfo: fileInfo,
			})
			continue
		}
		if fileTime, err := timeFromFileName(fileInfo.Name(), prefix, ext+compressSuffix, backupTimeFormat); err == nil {
			// 已经压缩
			logFileMetas = append(logFileMetas, &LogFileMeta{
				Time:     fileTime,
				FileInfo: fileInfo,
			})
			continue
		}
		// 不满足条件的文件都不处理
	}

	// 按时间排序
	sort.Sort(sortableLogFileMeta(logFileMetas))

	return logFileMetas, err
}

// getBackupNameByTime 根据时间获取备份新命名
func getBackupNameByTime(name string, layout string) string {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := strings.TrimSuffix(filename, ext)
	nowTm := time.Now()

	return filepath.Join(dir, fmt.Sprintf("%s_%s%s", prefix, nowTm.Format(layout), ext))
}

// timeFromFileName 从日志文件名获取时间信息
func timeFromFileName(name, prefix, ext, layout string) (time.Time, error) {
	// 前缀
	if !strings.HasPrefix(name, prefix) {
		return time.Time{}, errors.New("logger: invalid file name not prefix")
	}
	// 后缀
	if !strings.HasSuffix(name, ext) {
		return time.Time{}, errors.New("logger: invalid file name not suffix")
	}

	ts := name[len(prefix) : len(name)-len(ext)]

	return time.ParseInLocation(layout, ts, time.Local)
}
