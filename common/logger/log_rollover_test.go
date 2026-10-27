package logger

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogFileRolloverWrite(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "app.log")
	rollover := NewLogFileRollover(filename, 1, 0, 0, false)
	t.Cleanup(func() {
		if err := rollover.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	for _, data := range [][]byte{[]byte("first line\n"), []byte("second line\n")} {
		n, err := rollover.Write(data)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if n != len(data) {
			t.Fatalf("Write() wrote %d bytes, want %d", n, len(data))
		}
	}
	if err := rollover.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	got, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if want := "first line\nsecond line\n"; string(got) != want {
		t.Fatalf("log contents = %q, want %q", got, want)
	}
}

func TestLogFileRolloverRejectsWriteLargerThanLimit(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "app.log")
	rollover := NewLogFileRollover(filename, 1, 0, 0, false)

	data := bytes.Repeat([]byte("x"), megaByte+1)
	n, err := rollover.Write(data)
	if err == nil {
		t.Fatal("Write() error = nil, want an oversized-write error")
	}
	if n != 0 {
		t.Fatalf("Write() wrote %d bytes, want 0", n)
	}
	if !strings.Contains(err.Error(), "write too large") {
		t.Fatalf("Write() error = %q, want it to contain %q", err, "write too large")
	}
	if _, statErr := os.Stat(filename); !os.IsNotExist(statErr) {
		t.Fatalf("oversized write created the log file; Stat() error = %v", statErr)
	}
}

func TestLogFileRolloverRotatesAtSizeLimit(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "app.log")
	rollover := NewLogFileRollover(filename, 1, 0, 0, false)
	t.Cleanup(func() {
		if err := rollover.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	first := bytes.Repeat([]byte("a"), megaByte)
	if _, err := rollover.Write(first); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	if _, err := rollover.Write([]byte("next file\n")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if err := rollover.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	active, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile(active) error = %v", err)
	}
	if want := "next file\n"; string(active) != want {
		t.Fatalf("active log contents = %q, want %q", active, want)
	}

	backups := backupFiles(t, filename)
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want 1; backups = %v", len(backups), backups)
	}
	info, err := os.Stat(backups[0])
	if err != nil {
		t.Fatalf("Stat(backup) error = %v", err)
	}
	if info.Size() != int64(len(first)) {
		t.Fatalf("backup size = %d, want %d", info.Size(), len(first))
	}
}

func TestLogFileRolloverRotate(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "app.log")
	rollover := NewLogFileRollover(filename, 1, 0, 0, false)
	t.Cleanup(func() {
		if err := rollover.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	if _, err := rollover.Write([]byte("before rotation\n")); err != nil {
		t.Fatalf("Write(before) error = %v", err)
	}
	if err := rollover.Rotate(); err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if _, err := rollover.Write([]byte("after rotation\n")); err != nil {
		t.Fatalf("Write(after) error = %v", err)
	}
	if err := rollover.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	active, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile(active) error = %v", err)
	}
	if want := "after rotation\n"; string(active) != want {
		t.Fatalf("active log contents = %q, want %q", active, want)
	}

	backups := backupFiles(t, filename)
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want 1; backups = %v", len(backups), backups)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if want := "before rotation\n"; string(backup) != want {
		t.Fatalf("backup contents = %q, want %q", backup, want)
	}
}

func TestLogFileRolloverConcurrentWrites(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "app.log")
	rollover := NewLogFileRollover(filename, 4, 0, 0, false)

	const (
		writers = 12
		lines   = 80
	)
	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		writer := writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			for line := 0; line < lines; line++ {
				if _, err := fmt.Fprintf(rollover, "%02d:%03d\n", writer, line); err != nil {
					t.Errorf("Write(%d, %d) error = %v", writer, line, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if err := rollover.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	gotLines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(gotLines) != writers*lines {
		t.Fatalf("line count = %d, want %d", len(gotLines), writers*lines)
	}
	seen := make(map[string]struct{}, len(gotLines))
	for _, line := range gotLines {
		seen[line] = struct{}{}
	}
	for writer := 0; writer < writers; writer++ {
		for line := 0; line < lines; line++ {
			want := fmt.Sprintf("%02d:%03d", writer, line)
			if _, ok := seen[want]; !ok {
				t.Fatalf("missing log line %q", want)
			}
		}
	}
}

func TestLogFileRolloverRemovesExcessBackups(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "app.log")
	rollover := NewLogFileRollover(filename, 1, 2, 0, false)

	times := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
	}
	for _, tm := range times {
		writeBackup(t, filename, tm, []byte(tm.Format(time.RFC3339)))
	}

	if err := rollover.rolloverExec(); err != nil {
		t.Fatalf("rolloverExec() error = %v", err)
	}

	backups := backupFiles(t, filename)
	if len(backups) != 2 {
		t.Fatalf("backup count = %d, want 2; backups = %v", len(backups), backups)
	}
	newest := []string{
		backupName(filename, times[2]),
		backupName(filename, times[3]),
	}
	sort.Strings(newest)
	if !equalStrings(backups, newest) {
		t.Fatalf("remaining backups = %v, want %v", backups, newest)
	}
}

func TestLogFileRolloverRemovesExpiredBackups(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "app.log")
	rollover := NewLogFileRollover(filename, 1, 0, 2, false)

	oldName := writeBackup(t, filename, time.Now().Add(-72*time.Hour), []byte("old"))
	recentName := writeBackup(t, filename, time.Now().Add(-24*time.Hour), []byte("recent"))

	if err := rollover.rolloverExec(); err != nil {
		t.Fatalf("rolloverExec() error = %v", err)
	}
	if _, err := os.Stat(oldName); !os.IsNotExist(err) {
		t.Fatalf("expired backup still exists; Stat() error = %v", err)
	}
	if _, err := os.Stat(recentName); err != nil {
		t.Fatalf("recent backup was removed; Stat() error = %v", err)
	}
}

func TestLogFileRolloverCompressesBackups(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "app.log")
	rollover := NewLogFileRollover(filename, 1, 0, 0, true)
	want := []byte("content to compress\n")
	backup := writeBackup(t, filename, time.Now().Add(-time.Hour), want)

	if err := rollover.rolloverExec(); err != nil {
		t.Fatalf("rolloverExec() error = %v", err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("uncompressed backup still exists; Stat() error = %v", err)
	}

	gzFile, err := os.Open(backup + compressSuffix)
	if err != nil {
		t.Fatalf("Open(compressed backup) error = %v", err)
	}
	defer gzFile.Close()
	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	got, err := io.ReadAll(gzReader)
	if err != nil {
		t.Fatalf("ReadAll(compressed backup) error = %v", err)
	}
	if err := gzReader.Close(); err != nil {
		t.Fatalf("gzip reader Close() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("compressed contents = %q, want %q", got, want)
	}
}

func TestLoadFileListIncludesCompressedBackups(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "app.log")
	rollover := NewLogFileRollover(filename, 1, 0, 0, false)
	tm := time.Date(2026, 2, 3, 4, 5, 6, 7*int(time.Millisecond), time.Local)
	compressedName := writeGzipBackup(t, filename, tm, []byte("compressed"))

	metas, err := rollover.loadFileList()
	if err != nil {
		t.Fatalf("loadFileList() error = %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("loadFileList() returned %d files, want 1", len(metas))
	}
	if got := filepath.Join(dir, metas[0].FileInfo.Name()); got != compressedName {
		t.Fatalf("loaded file = %q, want %q", got, compressedName)
	}
	if !metas[0].Time.Equal(tm) {
		t.Fatalf("loaded time = %v, want %v", metas[0].Time, tm)
	}
}

func backupFiles(t *testing.T, filename string) []string {
	t.Helper()
	ext := filepath.Ext(filename)
	pattern := filepath.Join(filepath.Dir(filename), strings.TrimSuffix(filepath.Base(filename), ext)+"_*"+ext)
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob(%q) error = %v", pattern, err)
	}
	sort.Strings(files)
	return files
}

func backupName(filename string, tm time.Time) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filepath.Base(filename), ext)
	return filepath.Join(filepath.Dir(filename), fmt.Sprintf("%s_%s%s", base, tm.Format(backupTimeFormat), ext))
}

func writeBackup(t *testing.T, filename string, tm time.Time, data []byte) string {
	t.Helper()
	name := backupName(filename, tm)
	if err := os.WriteFile(name, data, 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", name, err)
	}
	return name
}

func writeGzipBackup(t *testing.T, filename string, tm time.Time, data []byte) string {
	t.Helper()
	name := backupName(filename, tm) + compressSuffix
	file, err := os.Create(name)
	if err != nil {
		t.Fatalf("Create(%q) error = %v", name, err)
	}
	gz := gzip.NewWriter(file)
	if _, err := gz.Write(data); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
	return name
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
