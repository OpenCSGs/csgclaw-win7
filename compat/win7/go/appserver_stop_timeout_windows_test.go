//go:build windows

package codex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWin7StopWaitsForExitMetadataBeforeReplacement(t *testing.T) {
	version := windows.RtlGetVersion()
	if version.MajorVersion != 6 || version.MinorVersion != 1 {
		t.Skip("native Win7 lifecycle regression")
	}
	original := appServerCommandContext
	appServerCommandContext = func(ctx context.Context, _ string, _ []string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppServerLifecycleHelperProcess$", "--", "terminal-boundary"), nil
	}
	defer func() { appServerCommandContext = original }()
	spec := testAppServerSessionSpec(t.TempDir())
	marker := filepath.Join(spec.RuntimeDir, "exit-metadata-committed")
	deps := testAppServerManagerDeps()
	deps.OnExit = func(_ *Session, _ int) {
		time.Sleep(4 * time.Second)
		_ = os.WriteFile(marker, []byte("complete"), 0600)
	}
	manager := newAppServerManager(deps)
	defer manager.Close()
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	oldPID := manager.liveSession(spec.RuntimeID).cmd.Process.Pid
	if err := manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}); err != nil {
		t.Fatal(err)
	}
	if processAlive(oldPID) {
		t.Fatal("Stop returned while old native process was alive")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("Stop returned before exit metadata: %v", err)
	}
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("replacement start: %v", err)
	}
	if manager.liveSession(spec.RuntimeID).cmd.Process.Pid == oldPID {
		t.Fatal("replacement reused old process")
	}
}

func TestWin7StopHonorsCanceledCallerDuringExitCleanup(t *testing.T) {
	version := windows.RtlGetVersion()
	if version.MajorVersion != 6 || version.MinorVersion != 1 {
		t.Skip("native Win7 lifecycle regression")
	}
	original := appServerCommandContext
	appServerCommandContext = func(ctx context.Context, _ string, _ []string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppServerLifecycleHelperProcess$", "--", "terminal-boundary"), nil
	}
	defer func() { appServerCommandContext = original }()
	release := make(chan struct{})
	deps := testAppServerManagerDeps()
	deps.OnExit = func(_ *Session, _ int) { <-release }
	manager := newAppServerManager(deps)
	spec := testAppServerSessionSpec(t.TempDir())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	live := manager.liveSession(spec.RuntimeID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := manager.Stop(ctx, SessionHandle{RuntimeID: spec.RuntimeID})
	close(release)
	select {
	case <-live.done:
	case <-time.After(30 * time.Second):
		t.Fatal("cleanup did not finish")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop = %v, want canceled caller", err)
	}
	if processAlive(live.cmd.Process.Pid) {
		t.Fatal("canceled stop left native process alive")
	}
}
