// Test-only command worker for an isolated UTM guest. Never ship with the application.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

type Task struct {
	ID        string   `json:"id"`
	Downloads []string `json:"downloads"`
	Command   []string `json:"command"`
	Spawn     bool     `json:"spawn"`
	Timeout   int      `json:"timeout"`
	Env       []string `json:"env"`
	Files     []string `json:"files"`
}

func main() {
	dir := `C:\CSGClawTest\compat`
	os.Chdir(dir)
	base := "http://10.0.2.2:48817/"
	client := http.Client{Timeout: 15 * time.Second}
	downloadClient := http.Client{Timeout: 10 * time.Minute}
	last := ""
	queue := "task.json"
	if len(os.Args) > 1 {
		queue = os.Args[1]
	}
	if queue != "task.json" && queue != "task-pty.json" {
		panic("invalid queue")
	}
	// One executor per queue prevents duplicate UI commands and conflicting results.
	name, err := syscall.UTF16PtrFromString(`Local\CSGClawWin7TestWorker-` + queue)
	if err != nil {
		panic(err)
	}
	mutex, _, status := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if mutex == 0 {
		panic(status)
	}
	defer syscall.CloseHandle(syscall.Handle(mutex))
	if status == syscall.Errno(183) {
		fmt.Println("Test queue already has an executor")
		return
	}
	fmt.Println("CSGClaw Win7 test worker. Host localhost:48817 only. Ctrl+C exits.")
	for {
		time.Sleep(2 * time.Second)
		r, e := client.Get(base + queue)
		if e != nil {
			continue
		}
		var t Task
		e = json.NewDecoder(r.Body).Decode(&t)
		r.Body.Close()
		if e != nil || t.ID == "" || t.ID == last {
			continue
		}
		last = t.ID
		result := map[string]interface{}{"task_id": t.ID}
		start := time.Now()
		fmt.Println("task", t.ID)
		for _, name := range t.Downloads {
			if filepath.Base(name) != name {
				result["error"] = "invalid download name"
				break
			}
			resp, e := downloadClient.Get(base + name)
			if e != nil {
				result["error"] = e.Error()
				break
			}
			if resp.StatusCode != 200 {
				result["error"] = resp.Status
				resp.Body.Close()
				break
			}
			f, e := os.Create(filepath.Join(dir, name))
			if e != nil {
				result["error"] = e.Error()
				resp.Body.Close()
				break
			}
			_, e = io.Copy(f, resp.Body)
			f.Close()
			resp.Body.Close()
			if e != nil {
				result["error"] = e.Error()
				break
			}
		}
		if _, failed := result["error"]; !failed && len(t.Command) > 0 {
			cmd := exec.Command(t.Command[0], t.Command[1:]...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), t.Env...)
			if t.Spawn {
				f, e := os.Create(filepath.Join(dir, t.ID+".log"))
				if e != nil {
					result["error"] = e.Error()
				} else {
					cmd.Stdout = f
					cmd.Stderr = f
					e = cmd.Start()
					f.Close()
					if e != nil {
						result["error"] = e.Error()
					} else {
						result["pid"] = cmd.Process.Pid
						go cmd.Wait()
					}
				}
			} else {
				var out bytes.Buffer
				// A timed-out WebDriver probe must not hang this executor in
				// taskkill/WMI, or in pipes retained by another process.
				cmd.WaitDelay = 5 * time.Second
				cmd.Stdout = &out
				cmd.Stderr = &out
				e = cmd.Start()
				if e == nil {
					done := make(chan error, 1)
					go func() { done <- cmd.Wait() }()
					seconds := t.Timeout
					if seconds == 0 {
						seconds = 180
					}
					ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
					select {
					case e = <-done:
					case <-ctx.Done():
						_ = cmd.Process.Kill()
						e = <-done
						result["timedOut"] = true
					}
					cancel()
					if cmd.ProcessState != nil {
						result["exit"] = cmd.ProcessState.ExitCode()
					}
				}
				if e != nil {
					result["error"] = e.Error()
				}
				result["output"] = out.String()
			}
		}
		files := map[string]string{}
		for _, name := range t.Files {
			if filepath.Base(name) == name {
				b, e := os.ReadFile(filepath.Join(dir, name))
				if e == nil {
					files[name] = string(b)
				}
			}
		}
		result["files"] = files
		result["duration"] = time.Since(start).String()
		b, _ := json.MarshalIndent(result, "", "  ")
		response, e := client.Post(base+"results", "application/json", bytes.NewReader(b))
		if e == nil {
			response.Body.Close()
		}
		// Independent queues must not truncate the same last-result file.
		// Report first, then atomically retain a queue-local diagnostic copy.
		if f, err := os.CreateTemp(dir, "worker-result-*.tmp"); err == nil {
			name := f.Name()
			_, writeErr := f.Write(b)
			closeErr := f.Close()
			if writeErr == nil && closeErr == nil {
				_ = os.Rename(name, filepath.Join(dir, "worker-last-"+queue))
			}
			_ = os.Remove(name)
		}
		fmt.Println("completed", t.ID)
	}
}
