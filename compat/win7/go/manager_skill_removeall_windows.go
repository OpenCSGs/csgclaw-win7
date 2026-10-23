//go:build windows

package codex

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func init() {
	v := windows.RtlGetVersion()
	if v.MajorVersion == 6 && v.MinorVersion == 1 {
		removeManagerSkillTree = removeWin7ManagerSkillDirectory
		managerSkillTreeCurrent = win7ManagerSkillTreeCurrent
	}
}

// Avoid Go's relative NtCreateFile removal path when refreshing manager skills
// on Win7. Pin each directory against writes and renames while enumerating it,
// and inspect the handle before traversing. Reparse points are unlinked, never
// followed, so a custom junction cannot redirect cleanup outside this tree.
func removeWin7ManagerSkillDirectory(path string) error {
	h, info, err := openWin7ManagerSkillPath(path)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 && info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			windows.CloseHandle(h)
			return readErr
		}
		for _, entry := range entries {
			if err = removeWin7ManagerSkillDirectory(filepath.Join(path, entry.Name())); err != nil {
				windows.CloseHandle(h)
				return err
			}
		}
	}
	if err = windows.CloseHandle(h); err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Check the complete embedded tree instead of deleting unchanged skills on
// every restart. Extra files and any reparse point require a fresh projection.
func win7ManagerSkillTreeCurrent(source fs.FS, sourcePath, target string) bool {
	h, info, err := openWin7ManagerSkillPath(target)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return false
	}
	want, err := fs.Stat(source, sourcePath)
	if err != nil || want.IsDir() != (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		return false
	}
	if !want.IsDir() {
		a, err := fs.ReadFile(source, sourcePath)
		if err != nil {
			return false
		}
		b, err := os.ReadFile(target)
		return err == nil && bytes.Equal(a, b)
	}
	wanted, err := fs.ReadDir(source, sourcePath)
	if err != nil {
		return false
	}
	actual, err := os.ReadDir(target)
	if err != nil || len(actual) != len(wanted) {
		return false
	}
	for i, entry := range wanted {
		if entry.Name() != actual[i].Name() || !win7ManagerSkillTreeCurrent(source, pathpkg.Join(sourcePath, entry.Name()), filepath.Join(target, entry.Name())) {
			return false
		}
	}
	return true
}

func openWin7ManagerSkillPath(path string) (windows.Handle, windows.ByHandleFileInformation, error) {
	var info windows.ByHandleFileInformation
	full, err := filepath.Abs(path)
	if err != nil {
		return windows.InvalidHandle, info, err
	}
	if !strings.HasPrefix(full, `\\?\`) {
		if strings.HasPrefix(full, `\\`) {
			full = `\\?\UNC\` + full[2:]
		} else {
			full = `\\?\` + full
		}
	}
	wide, err := windows.UTF16PtrFromString(full)
	if err != nil {
		return windows.InvalidHandle, info, err
	}
	var h windows.Handle
	for attempt := 0; ; attempt++ {
		h, err = windows.CreateFile(wide, windows.FILE_READ_ATTRIBUTES|windows.FILE_LIST_DIRECTORY, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err == nil || attempt == 39 || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		return windows.InvalidHandle, info, &os.PathError{Op: "open manager skill", Path: path, Err: err}
	}
	if err = windows.GetFileInformationByHandle(h, &info); err != nil {
		windows.CloseHandle(h)
		return windows.InvalidHandle, info, &os.PathError{Op: "inspect manager skill", Path: path, Err: err}
	}
	return h, info, nil
}
