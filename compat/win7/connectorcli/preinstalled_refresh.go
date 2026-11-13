package connectorcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// A Win7 rebuild can change the shipped binary without changing the official
// CLI version. Refresh that package while keeping authorization data separate.
func (i Installer) preinstalledChanged(pkg Package) (bool, error) {
	if i.PreinstalledDir == "" {
		return false, nil
	}
	read := func(path string) (packageManifest, error) {
		var manifest packageManifest
		data, err := os.ReadFile(path)
		if err != nil {
			return manifest, err
		}
		err = json.Unmarshal(data, &manifest)
		return manifest, err
	}
	seed, err := read(filepath.Join(i.PreinstalledDir, i.Spec.Executable, i.Spec.Version, "package.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read preinstalled CLI manifest: %w", err)
	}
	if seed.Version != i.Spec.Version || seed.SkillsCommit != i.Spec.Skills.Version {
		return false, nil
	}
	installed, err := read(filepath.Join(filepath.Dir(pkg.SkillsDir), "package.json"))
	if err != nil {
		return false, err
	}
	return seed != installed, nil
}
