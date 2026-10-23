// Fault injection for an isolated Win7 desktop profile. Never ship this helper.
package main

import (
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string) map[string]any {
	b, e := os.ReadFile(path)
	check(e)
	var v map[string]any
	check(json.Unmarshal(b, &v))
	return v
}
func main() {
	if len(os.Args) != 3 {
		panic("usage: recovery-fault.exe ISOLATED_PROFILE INSTALLED_APP_DIRECTORY")
	}
	profile, installed := filepath.Clean(os.Args[1]), filepath.Clean(os.Args[2])
	if !strings.HasPrefix(strings.ToLower(profile), `c:\csgclawtest\`) {
		panic("requires an isolated CSGClawTest profile")
	}
	metadata := filepath.Join(profile, `.csgclaw\agents\agent-manager\.codex\runtime.json`)
	m := read(metadata)
	if m["agent_id"] != "agent-manager" || m["state"] != "running" {
		panic("manager is not running")
	}
	pid := uint32(m["pid"].(float64))
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	check(e)
	defer windows.CloseHandle(h)
	var buf [32768]uint16
	n := uint32(len(buf))
	check(windows.QueryFullProcessImageName(h, 0, &buf[0], &n))
	if !strings.EqualFold(windows.UTF16ToString(buf[:n]), filepath.Join(installed, `resources\backend\csgclaw\bin\codex.exe`)) {
		panic("manager PID is not the installed Codex")
	}
	check(windows.TerminateProcess(h, 1))
	s, e := windows.WaitForSingleObject(h, 30000)
	check(e)
	if s != windows.WAIT_OBJECT_0 {
		panic("old Codex did not exit")
	}
	deadline := time.Now().Add(40 * time.Second)
	for {
		m = read(metadata)
		if m["state"] == "exited" || m["state"] == "stopped" || m["state"] == "failed" {
			break
		}
		if time.Now().After(deadline) {
			panic("exit metadata did not settle")
		}
		time.Sleep(100 * time.Millisecond)
	}
	path := filepath.Join(profile, `.csgclaw\state.json`)
	b, e := os.ReadFile(path)
	check(e)
	backup := filepath.Join(profile, fmt.Sprintf("recovery-state-before-%d.json", time.Now().UnixNano()))
	check(os.WriteFile(backup, b, 0600))
	var state map[string]any
	check(json.Unmarshal(b, &state))
	found := false
	for _, raw := range state["agents"].(map[string]any)["items"].([]any) {
		a := raw.(map[string]any)
		if a["id"] != "agent-manager" {
			continue
		}
		a["model_config"].(map[string]any)["env_restart_required"] = true
		a["runtime"].(map[string]any)["state"] = "failed"
		delete(a, "desired_state")
		a["updated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		found = true
	}
	if !found {
		panic("manager record missing")
	}
	b, e = json.Marshal(state)
	check(e)
	f, e := os.CreateTemp(filepath.Dir(path), "recovery-*.tmp")
	check(e)
	temporary := f.Name()
	defer os.Remove(temporary)
	_, e = f.Write(b)
	check(e)
	check(f.Close())
	for i := 0; ; i++ {
		e = os.Rename(temporary, path)
		if e == nil || i == 39 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	check(e)
	fmt.Printf("Injected offline manager with pending configuration; old Codex PID %d exited, desktop backend retained.\n", pid)
}
