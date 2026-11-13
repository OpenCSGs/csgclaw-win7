package connectorcli

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestWin7PreinstalledRebuildReplacesValidCachedCLI(t *testing.T) {
	i, source, transport := preinstalledFixture(t)
	ctx := context.Background()
	installed, err := i.Install(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(source.Executable)
	if err != nil {
		t.Fatal(err)
	}
	updated = append(updated, []byte("# updated HTTPS roots\n")...)
	if err := os.WriteFile(source.Executable, updated, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writePackageManifest(source); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(ctx, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(installed.Executable)
	if err != nil || string(data) != string(updated) || transport.calls != 0 {
		t.Fatalf("cached rebuild was not refreshed offline: requests=%d read=%v", transport.calls, err)
	}
	// A modified seed must fail verification without damaging the valid cache.
	if err := os.WriteFile(source.Executable, []byte("corrupt"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writePackageManifest(source); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(ctx, nil); err == nil || !strings.Contains(err.Error(), "version verification") {
		t.Fatalf("corrupt seed was accepted: %v", err)
	}
	data, err = os.ReadFile(installed.Executable)
	if err != nil || string(data) != string(updated) {
		t.Fatal("valid cached package was damaged")
	}
}
