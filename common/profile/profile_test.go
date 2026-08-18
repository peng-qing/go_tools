package profile

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileMemoryHandlers(t *testing.T) {
	t.Chdir(t.TempDir())
	manager := NewProfileManager()

	gcResponse := httptest.NewRecorder()
	manager.ProcessForceGC(gcResponse, httptest.NewRequest("GET", "/gc", nil))
	t.Logf("actual: status=%d, content-type=%q, body=%q", gcResponse.Code, gcResponse.Header().Get("Content-Type"), gcResponse.Body.String())
	t.Logf("expected: status=200, content-type=%q, body contains %q and %q", "text/plain; charset=utf-8", "forced call to GC was successful", "HeapAlloc")
	if gcResponse.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("GC content type = %q", gcResponse.Header().Get("Content-Type"))
	}
	if !strings.Contains(gcResponse.Body.String(), "forced call to GC was successful") ||
		!strings.Contains(gcResponse.Body.String(), "HeapAlloc") {
		t.Fatalf("unexpected GC response: %q", gcResponse.Body.String())
	}

	openResponse := httptest.NewRecorder()
	manager.ProcessMemoryAnalysis(openResponse, httptest.NewRequest("GET", "/open", nil))
	if !manager.inMemoryAnalysis || manager.memoryFile == nil || manager.filename == "" {
		t.Fatalf("analysis did not start: %+v", manager)
	}
	if !strings.Contains(openResponse.Body.String(), "analysis is start") {
		t.Fatalf("unexpected open response: %q", openResponse.Body.String())
	}

	duplicateResponse := httptest.NewRecorder()
	manager.ProcessMemoryAnalysis(duplicateResponse, httptest.NewRequest("GET", "/open", nil))
	if !strings.Contains(duplicateResponse.Body.String(), "can't open again") {
		t.Fatalf("unexpected duplicate response: %q", duplicateResponse.Body.String())
	}

	stopResponse := httptest.NewRecorder()
	manager.ProcessMemoryAnalysisStop(stopResponse, httptest.NewRequest("GET", "/stop", nil))
	if manager.inMemoryAnalysis || manager.memoryFile != nil {
		t.Fatalf("analysis did not stop: %+v", manager)
	}
	if !strings.Contains(stopResponse.Body.String(), "is stop") ||
		!strings.Contains(stopResponse.Body.String(), "HeapAlloc") {
		t.Fatalf("unexpected stop response: %q", stopResponse.Body.String())
	}

	secondStop := httptest.NewRecorder()
	manager.ProcessMemoryAnalysisStop(secondStop, httptest.NewRequest("GET", "/stop", nil))
	if !strings.Contains(secondStop.Body.String(), "analysis is open running") {
		t.Fatalf("unexpected second stop response: %q", secondStop.Body.String())
	}
}

func TestGetMemoryStatsContainsCoreMetrics(t *testing.T) {
	stats := NewProfileManager().getMemoryStats()
	expectedMetrics := []string{"Alloc", "TotalAlloc", "HeapObjects", "NumGC", "GCCPUFraction"}
	t.Logf("actual: %q", stats)
	t.Logf("expected: contains %v", expectedMetrics)
	for _, metric := range expectedMetrics {
		if !strings.Contains(stats, metric) {
			t.Fatalf("memory stats missing %q: %q", metric, stats)
		}
	}
}
