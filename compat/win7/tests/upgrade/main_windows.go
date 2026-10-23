// Isolated native acceptance: never changes the production app or its registry keys.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const bridge = "http://10.0.2.2:48817/"

var log bytes.Buffer

func execute(path string, expected int, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout, cmd.Stderr = &log, &log
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	code := 0
	if err != nil {
		if cmd.ProcessState == nil {
			return err
		}
		code = cmd.ProcessState.ExitCode()
	}
	fmt.Fprintf(&log, "%s %v -> %d\n", filepath.Base(path), args, code)
	if code != expected {
		return fmt.Errorf("%s exit %d, expected %d", path, code, expected)
	}
	return nil
}

func download(dir, name string) error {
	client := http.Client{Timeout: 120 * time.Second}
	r, err := client.Get(bridge + name)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("download %s: %s", name, r.Status)
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r.Body)
	return errors.Join(err, f.Close())
}

func expect(path, body string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(data) != body {
		return fmt.Errorf("wrong file contents: %s", path)
	}
	return nil
}

func acceptance() error {
	dir := `C:\CSGClawTest\upgrade-acceptance`
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, name := range []string{"upgrade-native-tests.exe", "upgrade-fixture-old.exe", "upgrade-fixture-new.exe", "upgrade-fixture-downgrade.exe"} {
		if err := download(dir, name); err != nil {
			return err
		}
	}
	if err := execute(filepath.Join(dir, "upgrade-native-tests.exe"), 0, "-test.v"); err != nil {
		return err
	}
	target := filepath.Join(dir, "app")
	old := filepath.Join(dir, "upgrade-fixture-old.exe")
	next := filepath.Join(dir, "upgrade-fixture-new.exe")
	if err := execute(old, 0, "/S", "/D="+target); err != nil {
		return err
	}
	if err := expect(filepath.Join(target, "obsolete.dll"), "old"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, "user-notes.txt"), []byte("retain user file"), 0600); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	profile := filepath.Join(home, ".csgclaw", "upgrade-acceptance-sentinel.txt")
	if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
		return err
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		return fmt.Errorf("fixture sentinel already exists: %s", profile)
	}
	if err := os.WriteFile(profile, []byte("retain profile"), 0600); err != nil {
		return err
	}
	// This fixture executable blocks replacement while its installed image is running.
	running := exec.Command(filepath.Join(target, "CSGClaw.exe"), "hold")
	if err := running.Start(); err != nil {
		return err
	}
	err = execute(next, 2, "/S", "/D="+target)
	_ = running.Process.Kill()
	_ = running.Wait()
	if err != nil {
		return err
	}
	if err := expect(filepath.Join(target, "obsolete.dll"), "old"); err != nil {
		return err
	}
	// No /D: the new installer must discover the previous registered directory.
	if err := execute(next, 0, "/S"); err != nil {
		return err
	}
	if err := expect(filepath.Join(target, "replacement.dll"), "new"); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(target, "obsolete.dll")); !os.IsNotExist(err) {
		return fmt.Errorf("obsolete file survived: %v", err)
	}
	if err := expect(filepath.Join(target, "user-notes.txt"), "retain user file"); err != nil {
		return err
	}
	if err := expect(profile, "retain profile"); err != nil {
		return err
	}
	if err := execute(next, 0, "/S"); err != nil {
		return err
	} // Same revision repairs in place.
	if err := execute(filepath.Join(dir, "upgrade-fixture-downgrade.exe"), 2, "/S"); err != nil {
		return err
	}
	if err := expect(filepath.Join(target, "resources", "app.asar"), "new"); err != nil {
		return err
	}
	if err := execute(filepath.Join(target, "Uninstall.exe"), 0, "/S", "_?="+target); err != nil {
		return err
	}
	if err := expect(filepath.Join(target, "user-notes.txt"), "retain user file"); err != nil {
		return err
	}
	if err := expect(profile, "retain profile"); err != nil {
		return err
	}
	if err := os.Remove(profile); err != nil {
		return err
	}
	fmt.Fprintln(&log, "PASS: running guard, legacy upgrade, directory discovery, obsolete cleanup, data retention, repair, downgrade rejection, uninstall")
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "hold" {
		time.Sleep(5 * time.Minute)
		return
	}
	err := acceptance()
	result := map[string]any{"task_id": "win7-installer-upgrade-acceptance", "output": log.String(), "exit": 0}
	if err != nil {
		result["error"] = err.Error()
		result["exit"] = 1
	}
	body, _ := json.Marshal(result)
	client := http.Client{Timeout: 30 * time.Second}
	r, uploadErr := client.Post(bridge+"results", "application/json", bytes.NewReader(body))
	if uploadErr == nil {
		r.Body.Close()
	}
	_ = os.WriteFile(`C:\CSGClawTest\upgrade-acceptance\result.json`, body, 0600)
	fmt.Println(strings.TrimSpace(string(body)))
}
