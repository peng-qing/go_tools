package profile

import (
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"text/tabwriter"
	"time"
)

// 性能分析管理器
type ProfileManager struct {
	inMemoryAnalysis bool     // 是否内存分析中
	profileIndex     int      // 文件索引
	filename         string   // 文件名
	memoryFile       *os.File // 内存导出文件
}

// NewProfileManager 构造函数
func NewProfileManager() *ProfileManager {
	return &ProfileManager{}
}

// ProcessForceGC 强制调用一次GC
func (p *ProfileManager) ProcessForceGC(w http.ResponseWriter, r *http.Request) {
	runtime.GC()

	memoryStats := p.getMemoryStats()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "The forced call to GC was successful, memory trace:\n %s\n, Try /debug/pprof/memory/open start memory analysis", memoryStats)
}

// ProcessMemoryAnalysis 开始内存分析
func (p *ProfileManager) ProcessMemoryAnalysis(w http.ResponseWriter, r *http.Request) {
	// WEB 界面显示结果 避免重复开启
	if p.inMemoryAnalysis {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Memory Analysis can't open again... Try /debug/pprof/memory/stop to stop")
		return
	}
	p.profileIndex++
	p.inMemoryAnalysis = true
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "pprof memory analysis is start, Try /debug/pprof/memory/stop to stop")
	// 内存文件导出
	p.filename = fmt.Sprintf("memory.profile.%s%d", time.Now().Format("2006--01-02"), p.profileIndex)
	p.memoryFile, _ = os.OpenFile(p.filename, os.O_CREATE|os.O_RDWR, 0644)
	if err := pprof.WriteHeapProfile(p.memoryFile); err != nil {
		fmt.Fprintf(w, "generate memory analysis profile failed, err:%v", err)
		return
	}
}

// ProcessMemoryAnalysisStop 停止内存分析
func (p *ProfileManager) ProcessMemoryAnalysisStop(w http.ResponseWriter, r *http.Request) {
	p.inMemoryAnalysis = false
	memStatus := p.getMemoryStats()
	p.memoryFile.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "memory analtsis is stop, memory trace:\n %s\n, Try /debug/pprof/memory/open start memory analysis", memStatus)
}

// getMemoryStats 获取内存状态字符串
func (p *ProfileManager) getMemoryStats() string {
	var (
		memStats      runtime.MemStats
		memTabBuilder strings.Builder
	)

	runtime.ReadMemStats(&memStats)

	// 使用 tabWriter 对齐列
	tabWriter := tabwriter.NewWriter(&memTabBuilder, 0, 0, 2, ' ', 0)
	// 表头
	fmt.Fprintf(tabWriter, "字段名\t字段值\t说明")
	// 添加字段内容
	fmt.Fprintf(tabWriter, "Alloc\t %d \t当前正在使用的堆内存字节数(≈ HeapAlloc)\n", memStats.Alloc)
	fmt.Fprintf(tabWriter, "TotalAlloc\t %d \t程序运行以来累计分配的堆内存总量\n", memStats.TotalAlloc)
	fmt.Fprintf(tabWriter, "Sys\t %d \t向操作系统申请的内存总量 (堆+栈+runtime)\n", memStats.Sys)
	fmt.Fprintf(tabWriter, "Mallocs\t %d \t累计分配的堆对象数\n", memStats.Mallocs)
	fmt.Fprintf(tabWriter, "Frees\t %d \t累计释放的堆对象数\n", memStats.Frees)
	fmt.Fprintf(tabWriter, "HeapAlloc\t %d \t堆上已分配、仍存活的对象字节数\n", memStats.HeapAlloc)
	fmt.Fprintf(tabWriter, "HeapSys\t %d \t堆向操作系统申请的总字节数\n", memStats.HeapSys)
	fmt.Fprintf(tabWriter, "HeapIdle\t %d \t空闲(未使用)的堆内存字节数\n", memStats.HeapIdle)
	fmt.Fprintf(tabWriter, "HeapInuse\t %d \t正在使用的堆内存字节数\n", memStats.HeapInuse)
	fmt.Fprintf(tabWriter, "HeapReleased\t %d \t已释放回操作系统的堆内存字节数\n", memStats.HeapReleased)
	fmt.Fprintf(tabWriter, "HeapObjects\t %d \t当前存活的堆对象数\n", memStats.HeapObjects)
	fmt.Fprintf(tabWriter, "NumGC\t %d \t完成的垃圾回收次数\n", memStats.NumGC)
	fmt.Fprintf(tabWriter, "PauseTotalNs\t %d \t垃圾回收累计暂停时间(纳秒)\n", memStats.PauseTotalNs)
	fmt.Fprintf(tabWriter, "GCCPUFraction\t %f \tGC 占用 CPU 时间的比例 (0~1)\n", memStats.GCCPUFraction)

	// 写入
	tabWriter.Flush()
	// 以字符串形式返回
	return memTabBuilder.String()
}

// ListenProfile 开始监听
func (p *ProfileManager) ListenProfile(addr string) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[ProfileManager] ListenProfile critical", slog.Any("err", err), slog.String("stack", string(debug.Stack())))
			}
		}()
		// http handler
		http.Handle("/debug/pprof/memory/gc", http.HandlerFunc(p.ProcessForceGC))
		http.Handle("/debug/pprof/memory/open", http.HandlerFunc(p.ProcessMemoryAnalysis))
		http.Handle("/debug/pprof/memory/stop", http.HandlerFunc(p.ProcessMemoryAnalysisStop))

		// start service
		http.ListenAndServe(addr, nil)
	}()
}
