package dsh

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestWin7DSHModuleFileURL(t *testing.T) {
	path := "/agents/安装 100% #/memory.mjs"
	want := "file:///agents/%E5%AE%89%E8%A3%85%20100%25%20%23/memory.mjs"
	if runtime.GOOS == "windows" {
		path = `C:\agents\安装 100% #\memory.mjs`
		want = "file:///C:/agents/%E5%AE%89%E8%A3%85%20100%25%20%23/memory.mjs"
		if got := moduleFileURL(`\\server\share\dsh\context-bridge.mjs`); got != "file://server/share/dsh/context-bridge.mjs" {
			t.Fatalf("UNC module URL=%q", got)
		}
	}
	if got := moduleFileURL(filepath.Clean(path)); got != want {
		t.Fatalf("module URL=%q, want %q", got, want)
	}
}
