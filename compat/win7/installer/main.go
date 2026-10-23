// Installer file transactions are separate from the application and its profiles.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var product = "OpenCSG.CSGClaw.Win7"

type manifest struct {
	Source string            `json:"source_commit"`
	Build  string            `json:"build_id"`
	Order  int64             `json:"source_order"`
	Files  map[string]string `json:"files"`
}

type entry struct {
	Name     string
	Original bool
}

type journal struct {
	Target  string
	Entries []entry
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path+".pending", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(path+".pending", path)
}

func safeName(name string) error {
	if name == "" || strings.ContainsAny(name, "\\:\x00\r\n<>\"|?*") || strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid payload path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		upper := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		reserved := upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" || (len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '0' && upper[3] <= '9')
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part || reserved {
			return fmt.Errorf("invalid payload path %q", name)
		}
	}
	return nil
}

// Refuse symlinks/junctions and file/directory collisions rather than following them.
func regularPath(root, name string) (string, error) {
	if err := safeName(name); err != nil {
		return "", err
	}
	path := root
	parts := strings.Split(name, "/")
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return "", fmt.Errorf("not a regular payload path: %s", path)
		}
	}
	return path, nil
}

func targetPath(input string) (string, error) {
	path, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	if filepath.Dir(path) == path || strings.EqualFold(filepath.Base(path), ".csgclaw") {
		return "", fmt.Errorf("unsafe install directory: %s", path)
	}
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return "", fmt.Errorf("unsafe install directory: %s", p)
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return path, nil
}

func workspace(target string) string { return target + ".csgclaw-upgrade" }

func validateWork(target string) error {
	work := workspace(target)
	path, err := targetPath(work)
	if err != nil || path != work {
		return fmt.Errorf("unsafe transaction directory: %s", work)
	}
	var owner string
	if err := readJSON(filepath.Join(work, "owner.json"), &owner); err != nil {
		return err
	}
	if owner != target {
		return fmt.Errorf("foreign transaction directory: %s", work)
	}
	return nil
}

func loadManifest(root string) (manifest, error) {
	var m manifest
	err := readJSON(filepath.Join(root, "win7-build.json"), &m)
	if err != nil {
		return m, err
	}
	if len(m.Files) == 0 {
		return m, errors.New("empty application manifest")
	}
	seen := map[string]bool{}
	for name := range m.Files {
		if err := safeName(name); err != nil {
			return m, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return m, fmt.Errorf("duplicate payload path %s", name)
		}
		seen[key] = true
	}
	return m, nil
}

func owned(target string) (manifest, error) {
	m, err := loadManifest(target)
	if err != nil {
		return m, err
	}
	data, err := os.ReadFile(filepath.Join(target, "win7-install.ini"))
	matched := false
	for _, line := range strings.Split(string(data), "\n") {
		matched = matched || strings.TrimSpace(line) == "ID="+product
	}
	if err != nil || !matched {
		return m, errors.New("directory is not owned by the Win7 installer")
	}
	return m, nil
}

// Recovery is idempotent even if the installer was killed during rollback.
func recoverFiles(target string) error {
	work := workspace(target)
	if _, err := os.Lstat(work); os.IsNotExist(err) {
		return nil
	}
	if err := validateWork(target); err != nil {
		return err
	}
	var complete bool
	if err := readJSON(filepath.Join(work, "complete.json"), &complete); err == nil && complete {
		return os.RemoveAll(work)
	}
	var j journal
	err := readJSON(filepath.Join(work, "journal.json"), &j)
	if os.IsNotExist(err) {
		return os.RemoveAll(work)
	} // Extraction never changed the old application.
	if err != nil {
		return err
	}
	if j.Target != target {
		return errors.New("transaction target mismatch")
	}
	for i := len(j.Entries) - 1; i >= 0; i-- {
		e := j.Entries[i]
		dest, err := regularPath(target, e.Name)
		if err != nil {
			return err
		}
		backup, err := regularPath(filepath.Join(work, "backup"), e.Name)
		if err != nil {
			return err
		}
		if _, err := os.Stat(backup); err == nil {
			if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				return err
			}
			if err := os.Rename(backup, dest); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		} else if !e.Original {
			if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return os.RemoveAll(work)
}

func prepare(target string) error {
	if err := recoverFiles(target); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(target, "CSGClaw.exe")); err == nil {
		if _, err := owned(target); err != nil {
			return err
		}
	}
	work := workspace(target)
	if err := os.MkdirAll(filepath.Dir(work), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(work, 0700); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(work, "owner.json"), target); err != nil {
		return err
	}
	return os.Mkdir(filepath.Join(work, "stage"), 0700)
}

func verify(root string, m manifest) error {
	for name, expected := range m.Files {
		path, err := regularPath(root, name)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		err = errors.Join(err, f.Close())
		if err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != expected {
			return fmt.Errorf("payload checksum mismatch: %s", name)
		}
	}
	return nil
}

// The hook is used only by tests to simulate a process crash at a real rename boundary.
func commit(target string, hook func(int) error) (result error) {
	if err := validateWork(target); err != nil {
		return err
	}
	work := workspace(target)
	stage := filepath.Join(work, "stage")
	next, err := owned(stage)
	if err != nil {
		return err
	}
	if err := verify(stage, next); err != nil {
		return err
	}
	previous := manifest{Files: map[string]string{}}
	if _, err := os.Stat(filepath.Join(target, "CSGClaw.exe")); err == nil {
		previous, err = owned(target)
		if err != nil {
			return err
		}
	}
	names := map[string]bool{}
	for name := range previous.Files {
		names[name] = true
	}
	for name := range next.Files {
		names[name] = true
	}
	for _, name := range []string{"win7-build.json", "win7-install.ini", "Uninstall.exe"} {
		names[name] = true
	}
	identities := map[string]string{}
	for name := range names {
		key := strings.ToLower(name)
		if old, ok := identities[key]; ok && old != name {
			return fmt.Errorf("case-only payload rename requires an explicit migration: %s / %s", old, name)
		}
		identities[key] = name
	}
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	j := journal{Target: target}
	for _, name := range keys {
		dest, err := regularPath(target, name)
		if err != nil {
			return err
		}
		_, err = regularPath(stage, name)
		if err != nil {
			return err
		}
		_, err = os.Stat(dest)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		// New payload paths must not overwrite files independently created by the user.
		if err == nil && previous.Files[name] == "" && name != "win7-build.json" && name != "win7-install.ini" && name != "Uninstall.exe" {
			return fmt.Errorf("unowned destination file: %s", name)
		}
		j.Entries = append(j.Entries, entry{Name: name, Original: err == nil})
	}
	if err := writeJSON(filepath.Join(work, "journal.json"), j); err != nil {
		return err
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, recoverFiles(target))
		}
	}()
	for i, e := range j.Entries {
		dest := filepath.Join(target, filepath.FromSlash(e.Name))
		backup := filepath.Join(work, "backup", filepath.FromSlash(e.Name))
		if e.Original {
			if err := os.MkdirAll(filepath.Dir(backup), 0700); err != nil {
				return err
			}
			if err := os.Rename(dest, backup); err != nil {
				return err
			}
		}
		if hook != nil {
			if err := hook(i); err != nil {
				return err
			}
		}
	}
	// Remove the old launcher first and publish the new launcher last, so clicking
	// the existing shortcut during replacement cannot start a mixed application.
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i] == "CSGClaw.exe" {
			return false
		}
		if keys[j] == "CSGClaw.exe" {
			return true
		}
		return keys[i] < keys[j]
	})
	for i, name := range keys {
		dest := filepath.Join(target, filepath.FromSlash(name))
		source := filepath.Join(stage, filepath.FromSlash(name))
		if _, err := os.Stat(source); err == nil {
			if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				return err
			}
			if err := os.Rename(source, dest); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if hook != nil {
			if err := hook(len(keys) + i); err != nil {
				return err
			}
		}
	}
	return nil
}

func finish(target string) error {
	if err := validateWork(target); err != nil {
		return err
	}
	work := workspace(target)
	// A separate durable marker keeps journal.json immutable and crash-readable.
	if err := writeJSON(filepath.Join(work, "complete.json"), true); err != nil {
		return err
	}
	if err := os.RemoveAll(work); err != nil {
		// The new application and metadata are committed. A later prepare retries cleanup.
		fmt.Println("Upgrade completed; retained backup cleanup will retry next time:", err)
	}
	return nil
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New("expected prepare, commit, recover, finish or inspect and an install directory")
	}
	target, err := targetPath(args[1])
	if err != nil {
		return err
	}
	switch args[0] {
	case "prepare":
		return prepare(target)
	case "commit":
		return commit(target, nil)
	case "recover":
		return recoverFiles(target)
	case "finish":
		return finish(target)
	case "inspect":
		if _, err := os.Stat(filepath.Join(target, "CSGClaw.exe")); os.IsNotExist(err) {
			fmt.Println("尚未安装")
			return nil
		}
		m, err := owned(target)
		if err != nil {
			return err
		}
		if m.Build == "" {
			m.Build = m.Source
			if len(m.Build) > 8 {
				m.Build = m.Build[:8]
			}
		}
		fmt.Print(m.Build)
		if len(args) != 4 {
			return nil
		}
		var order int64
		if _, err := fmt.Sscan(args[3], &order); err != nil {
			return err
		}
		if m.Source == args[2] {
			os.Exit(11)
		}
		if m.Order > 0 && order < m.Order {
			os.Exit(12)
		}
		os.Exit(10)
	default:
		return errors.New("unknown installer action")
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
