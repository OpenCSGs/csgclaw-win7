//go:build windows

package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestWin7ManagerSkillCleanupPreservesJunctionTarget(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	sentinel := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked-skill")
	if b, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Fatalf("junction: %v %s", err, b)
	}
	if win7ManagerSkillTreeCurrent(fstest.MapFS{"keep.txt": &fstest.MapFile{Data: []byte("preserve")}}, ".", link) {
		t.Fatal("junction was accepted as an embedded skill directory")
	}
	if err := removeWin7ManagerSkillDirectory(root); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "preserve" {
		t.Fatalf("junction target changed: %q %v", b, err)
	}
}

func TestWin7ManagerSkillCurrentRequiresExactTree(t *testing.T) {
	source := fstest.MapFS{"SKILL.md": &fstest.MapFile{Data: []byte("skill")}, "references/check.txt": &fstest.MapFile{Data: []byte("reference")}}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "references"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ name, value string }{{"SKILL.md", "skill"}, {"references/check.txt", "reference"}} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(item.name)), []byte(item.value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !win7ManagerSkillTreeCurrent(source, ".", root) {
		t.Fatal("unchanged skill was not reused")
	}
	extra := filepath.Join(root, "stale.txt")
	if err := os.WriteFile(extra, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if win7ManagerSkillTreeCurrent(source, ".", root) {
		t.Fatal("obsolete skill file was retained")
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if win7ManagerSkillTreeCurrent(source, ".", root) {
		t.Fatal("changed skill was not refreshed")
	}
	if err := removeWin7ManagerSkillDirectory(root); err != nil {
		t.Fatal(err)
	}
}

func TestWin7ManagerSkillCleanupNestedReadOnlyTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "basics")
	dir := filepath.Join(root, "scripts", "feishu_setup")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "state.py")
	if err := os.WriteFile(file, []byte("fixture"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := removeWin7ManagerSkillDirectory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("skill tree survived: %v", err)
	}
	if err := removeWin7ManagerSkillDirectory(root); err != nil {
		t.Fatalf("missing tree: %v", err)
	}
}
