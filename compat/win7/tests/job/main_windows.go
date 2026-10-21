// Test-only desktop launcher for the Windows 7 inherited-job regression.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"csgclaw/internal/processgroup"
)

func main() {
	if len(os.Args) < 2 || !strings.HasPrefix(strings.ToLower(os.Getenv("CSGCLAW_DESKTOP_CONFIG")), `c:\csgclawtest\`) {
		panic("requires an isolated CSGClawTest desktop config and an application path")
	}
	child := exec.Command(os.Args[1], os.Args[2:]...)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := processgroup.StartWithChildJobs(child); err != nil {
		panic(err)
	}
	defer processgroup.Stop(child.Process.Pid, true)
	data, _ := json.Marshal(map[string]int{"app_pid": child.Process.Pid, "launcher_pid": os.Getpid()})
	if err := os.WriteFile(`C:\CSGClawTest\compat\job-app-owner.json`, data, 0600); err != nil {
		panic(err)
	}
	fmt.Println(string(data))
	if err := child.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
