package codex

import (
	"io/fs"
	"os"
)

var removeManagerSkillTree = os.RemoveAll
var managerSkillTreeCurrent = func(fs.FS, string, string) bool { return false }

func (r *Runtime) managerSkillDirectoryMatches(source fs.FS, sourcePath, target string) bool {
	if r.deps.ReadFile != nil || r.deps.WriteFile != nil || r.deps.RemoveAll != nil {
		return false
	}
	return managerSkillTreeCurrent(source, sourcePath, target)
}

func (r *Runtime) removeManagerSkillDirectory(path string) error {
	if r.deps.RemoveAll != nil {
		return r.deps.RemoveAll(path)
	}
	return removeManagerSkillTree(path)
}
