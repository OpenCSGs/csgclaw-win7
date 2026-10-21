package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// macOS exposes /var through a symlink; production deliberately refuses links.
	if root, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		_ = os.Setenv("TMPDIR", root)
	}
	os.Exit(m.Run())
}

func fixture(t *testing.T, root, revision string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	m := manifest{Source: revision, Build: revision, Order: 1, Files: map[string]string{}}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256([]byte(body))
		m.Files[name] = hex.EncodeToString(h[:])
	}
	if err := writeJSON(filepath.Join(root, "win7-build.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "win7-install.ini"), []byte("[Product]\r\nID="+product+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Uninstall.exe"), []byte(revision), 0600); err != nil {
		t.Fatal(err)
	}
}

func content(t *testing.T, root, name, expected string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil || string(b) != expected {
		t.Fatalf("%s = %q, %v; want %q", name, b, err, expected)
	}
}

func setupUpgrade(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "CSGClaw 中文")
	fixture(t, target, "old", map[string]string{"CSGClaw.exe": "old app", "old.dll": "obsolete", "resources/app.asar": "old renderer"})
	if err := os.WriteFile(filepath.Join(target, "user-notes.txt"), []byte("my notes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepare(target); err != nil {
		t.Fatal(err)
	}
	fixture(t, filepath.Join(workspace(target), "stage"), "new", map[string]string{"CSGClaw.exe": "new app", "new.dll": "new runtime", "resources/app.asar": "new renderer"})
	return target
}

func TestUpgradeRetainsDataAndRemovesObsoleteFiles(t *testing.T) {
	target := setupUpgrade(t)
	if err := commit(target, nil); err != nil {
		t.Fatal(err)
	}
	if err := finish(target); err != nil {
		t.Fatal(err)
	}
	content(t, target, "CSGClaw.exe", "new app")
	content(t, target, "user-notes.txt", "my notes")
	if _, err := os.Stat(filepath.Join(target, "old.dll")); !os.IsNotExist(err) {
		t.Fatal("obsolete file survived", err)
	}
	if _, err := os.Stat(workspace(target)); !os.IsNotExist(err) {
		t.Fatal("transaction directory survived", err)
	}
}

func TestFailureRestoresOldApplicationAtEveryBoundary(t *testing.T) {
	for fail := 0; fail < 14; fail++ {
		target := setupUpgrade(t)
		err := commit(target, func(i int) error {
			if i == fail {
				return errors.New("simulated disk failure")
			}
			return nil
		})
		if err == nil {
			t.Fatalf("boundary %d was not tested", fail)
		}
		content(t, target, "CSGClaw.exe", "old app")
		content(t, target, "old.dll", "obsolete")
		content(t, target, "resources/app.asar", "old renderer")
		content(t, target, "user-notes.txt", "my notes")
		if _, err := os.Stat(filepath.Join(target, "new.dll")); !os.IsNotExist(err) {
			t.Fatal("new file survived rollback", err)
		}
	}
}

func TestCrashRecovery(t *testing.T) {
	if target := os.Getenv("CSGCLAW_UPGRADE_CRASH_FIXTURE"); target != "" {
		_ = commit(target, func(i int) error {
			if i == 3 {
				os.Exit(77)
			}
			return nil
		})
		os.Exit(78)
	}
	target := setupUpgrade(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashRecovery$")
	cmd.Env = append(os.Environ(), "CSGCLAW_UPGRADE_CRASH_FIXTURE="+target)
	if err := cmd.Run(); err == nil || cmd.ProcessState.ExitCode() != 77 {
		t.Fatal("crash fixture did not terminate at rename", err)
	}
	if err := prepare(target); err != nil {
		t.Fatal(err)
	}
	content(t, target, "CSGClaw.exe", "old app")
	content(t, target, "old.dll", "obsolete")
	content(t, target, "user-notes.txt", "my notes")
}

func TestExtractionInterruptionDoesNotTouchOldApplication(t *testing.T) {
	target := setupUpgrade(t)
	if err := prepare(target); err != nil {
		t.Fatal(err)
	}
	content(t, target, "CSGClaw.exe", "old app")
	content(t, target, "user-notes.txt", "my notes")
}

func TestFirstInstallAndInterruptedFinalization(t *testing.T) {
	target := filepath.Join(t.TempDir(), "fresh install")
	if err := prepare(target); err != nil {
		t.Fatal(err)
	}
	fixture(t, filepath.Join(workspace(target), "stage"), "new", map[string]string{"CSGClaw.exe": "new app"})
	if err := commit(target, nil); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(workspace(target), "complete.json"), true); err != nil {
		t.Fatal(err)
	}
	if err := prepare(target); err != nil {
		t.Fatal(err)
	}
	content(t, target, "CSGClaw.exe", "new app")
}

func TestMetadataFailureRestoresApplication(t *testing.T) {
	target := setupUpgrade(t)
	if err := commit(target, nil); err != nil {
		t.Fatal(err)
	}
	// NSIS calls recovery if registering the completed payload fails.
	if err := recoverFiles(target); err != nil {
		t.Fatal(err)
	}
	content(t, target, "CSGClaw.exe", "old app")
	content(t, target, "resources/app.asar", "old renderer")
	content(t, target, "user-notes.txt", "my notes")
}

func TestCorruptionAndUserFileCollisionAreRejected(t *testing.T) {
	for _, corrupt := range []bool{true, false} {
		target := setupUpgrade(t)
		if corrupt {
			if err := os.WriteFile(filepath.Join(workspace(target), "stage", "new.dll"), []byte("bad"), 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(filepath.Join(target, "new.dll"), []byte("user file"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := commit(target, nil); err == nil {
			t.Fatal("unsafe upgrade accepted")
		}
		content(t, target, "CSGClaw.exe", "old app")
		content(t, target, "user-notes.txt", "my notes")
	}
}

func TestForeignInstallAndTransactionAreRejected(t *testing.T) {
	target := filepath.Join(t.TempDir(), "CSGClaw")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "CSGClaw.exe"), []byte("portable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepare(target); err == nil {
		t.Fatal("foreign application accepted")
	}
	if err := os.Mkdir(workspace(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := recoverFiles(target); err == nil {
		t.Fatal("foreign workspace accepted")
	}
	content(t, target, "CSGClaw.exe", "portable")
}
