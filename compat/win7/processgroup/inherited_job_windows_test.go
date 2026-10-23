//go:build windows

package processgroup

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWin7InheritedJobLifecycle(t *testing.T) {
	version := windows.RtlGetVersion()
	if version.MajorVersion > 6 || version.MajorVersion == 6 && version.MinorVersion >= 2 {
		t.Skip("Windows 7 job inheritance regression")
	}
	for _, tc := range []struct {
		name  string
		flags uint32
		deny  bool
	}{
		{name: "explicit-breakaway", flags: windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK},
		{name: "silent-breakaway", flags: windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK},
		{name: "restricted-launcher", deny: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Run the real ownership, descendant cleanup and owner-crash tests
			// from inside a launcher job, as a desktop sidecar can be started.
			child := exec.Command(os.Args[0], "-test.run=^(TestStopAfterParentExited|TestCooperatingChildJobCleanup)$", "-test.v")
			var output bytes.Buffer
			child.Stdout, child.Stderr = &output, &output
			if err := start(child, windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE|tc.flags); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = Stop(child.Process.Pid, true) })
			err := child.Wait()
			if tc.deny {
				if err == nil || !strings.Contains(output.String(), "inherited job does not allow child breakaway") {
					t.Fatalf("restricted launcher must fail without running unmanaged children: err=%v output=%s", err, output.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("inherited job lifecycle failed: %v\n%s", err, output.String())
			}
		})
	}
}
