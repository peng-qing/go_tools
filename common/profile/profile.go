package profile

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strings"
	"text/tabwriter"
)

// 性能分析管理器
type ProfileManager struct {
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
	fmt.Fprintf(w, "The forced call to GC was successful, new memory trace: %s\n, Try /debug/pprof/memory to for memory analysis", memoryStats)
}

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
	// 添加字段内容 TODO
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")
	fmt.Fprintln(tabWriter, "Alloc\tuint64\t当前正在使用的堆内存字节数 (≈ HeapAlloc)")

	// 写入
	tabWriter.Flush()
	// 以字符串形式返回
	return memTabBuilder.String()
}
