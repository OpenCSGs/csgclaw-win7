package connectorcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIInstallVersionHelper(t *testing.T) {
	delay := os.Getenv("CSGCLAW_TEST_CLI_VERSION_DELAY")
	if delay == "" {
		return
	}
	duration, err := time.ParseDuration(delay)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(duration)
	fmt.Println("fixture " + testVersion)
	os.Exit(0)
}

func TestVerifyPackageColdCLIStartupAndCallerDeadline(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := Package{
		Executable: executable,
		SkillsDir:  filepath.Join(t.TempDir(), "skills"),
		Version:    testVersion,
		VersionArgs: []string{
			"-test.run=^TestCLIInstallVersionHelper$",
		},
	}
	if err := os.MkdirAll(p.SkillsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writePackageManifest(p); err != nil {
		t.Fatal(err)
	}
	t.Run("cold startup", func(t *testing.T) {
		t.Setenv("CSGCLAW_TEST_CLI_VERSION_DELAY", "11s")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := verifyPackage(ctx, p); err != nil {
			t.Fatalf("valid CLI rejected during cold startup: %v", err)
		}
	})
	t.Run("caller deadline", func(t *testing.T) {
		t.Setenv("CSGCLAW_TEST_CLI_VERSION_DELAY", "1m")
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		started := time.Now()
		if err := verifyPackage(ctx, p); err == nil || ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("caller deadline was not honored: %v (context: %v)", err, ctx.Err())
		}
		if elapsed := time.Since(started); elapsed > 30*time.Second {
			t.Fatalf("CLI cancellation took %s", elapsed)
		}
	})
}
