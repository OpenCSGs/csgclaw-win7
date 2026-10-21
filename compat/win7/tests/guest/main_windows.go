// Native Win7 desktop fixture setup. Run only inside an isolated test VM.
package main

import (
	"archive/zip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func copyFile(source, target string) {
	check(os.MkdirAll(filepath.Dir(target), 0700))
	in, e := os.Open(source)
	check(e)
	defer in.Close()
	out, e := os.Create(target)
	check(e)
	_, e = io.Copy(out, in)
	check(e)
	check(out.Close())
}
func fileHash(path string) string {
	// Native CryptoAPI avoids expensive emulated AVX hashing on the TCG test VM.
	raw, e := exec.Command(`C:\Windows\System32\certutil.exe`, "-hashfile", path, "SHA256").CombinedOutput()
	check(e)
	for _, line := range strings.Split(string(raw), "\n") {
		candidate := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if len(candidate) == 64 {
			if _, e := hex.DecodeString(candidate); e == nil {
				return strings.ToLower(candidate)
			}
		}
	}
	panic("certutil did not return a SHA-256 digest")
}

func extract(archive, target string) {
	r, e := zip.OpenReader(archive)
	check(e)
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			panic("unsafe ZIP path")
		}
		p := filepath.Join(target, name)
		if f.FileInfo().IsDir() {
			check(os.MkdirAll(p, 0700))
			continue
		}
		check(os.MkdirAll(filepath.Dir(p), 0700))
		in, e := f.Open()
		check(e)
		out, e := os.Create(p)
		check(e)
		_, e = io.Copy(out, in)
		check(e)
		check(in.Close())
		check(out.Close())
	}
}
func main() {
	if len(os.Args) < 3 {
		panic("usage: desktop-guest install <zip> | launch <application-directory> | logs <profile>")
	}
	switch os.Args[1] {
	case "install":
		destination := `C:\CSGClawTest\desktop-20261019`
		extract(os.Args[2], destination)
		for name, expected := range map[string]string{"codex.exe": "0ce90becd690f4a901fbb7270e69d946f99e92af48512ef22e934b7b1a080527", "winpty.dll": "bd04302d63fefe06de1bb6b82a3550f3ee2b11f52c54355e30841c1976122959", "winpty-agent.exe": "9b5b44c40f6bd673c3f91ce7cef4f6112fe0d9e478d52f829f8a90c0d9f1be15", "LICENSE.winpty": "c39e428064b4f3e4fe81a975bf0fd3b845922b431bc4d9a7ffc8bfb091981836"} {
			target := filepath.Join(destination, `resources\backend\csgclaw\bin`, name)
			if _, e := os.Stat(target); e == nil {
				if fileHash(target) != expected {
					panic("installed runtime checksum mismatch: " + name)
				}
				continue
			}
			source := filepath.Join(`C:\CSGClawTest\compat`, name)
			if fileHash(source) != expected {
				panic("previous runtime checksum mismatch: " + name)
			}
			copyFile(source, target)
		}
		fmt.Println("Installed isolated desktop fixture", destination)
	case "seed":
		profile := `C:\CSGClawTest\desktop-profile`
		old := `C:\CSGClawTest\browser-profile`
		{
			check(os.RemoveAll(profile))
			check(filepath.Walk(filepath.Join(old, ".csgclaw"), func(p string, info os.FileInfo, e error) error {
				if e != nil {
					return e
				}
				rel, e := filepath.Rel(old, p)
				if e != nil {
					return e
				}
				if info.IsDir() {
					if info.Name() == ".git" || info.Name() == "logs" || info.Name() == ".codex" || info.Name() == "session-bindings" {
						return filepath.SkipDir
					}
					return os.MkdirAll(filepath.Join(profile, rel), 0700)
				}
				if strings.HasSuffix(info.Name(), ".lock") || strings.HasSuffix(info.Name(), ".db") || strings.HasSuffix(info.Name(), "-wal") || strings.HasSuffix(info.Name(), "-shm") || info.Size() > 10*1024*1024 {
					return nil
				}
				copyFile(p, filepath.Join(profile, rel))
				return nil
			}))
		}
		fmt.Println("Seeded isolated desktop test profile")
	case "launch", "launchFresh", "launchReview":
		profile := `C:\CSGClawTest\desktop-profile`
		if os.Args[1] == "launchFresh" {
			profile = `C:\CSGClawTest\desktop-fresh-profile`
		}
		check(os.MkdirAll(profile, 0700))
		exe := filepath.Join(os.Args[2], "CSGClaw.exe")
		args := []string{"--user-data-dir=" + filepath.Join(profile, "electron"), "--remote-debugging-port=48824", "--remote-debugging-address=127.0.0.1"}
		if os.Args[1] == "launchReview" {
			args = args[1:]
		}
		cmd := exec.Command(exe, args...)
		cmd.Dir = os.Args[2]
		cmd.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile, `PATH=C:\CSGClawTest\MinGit-2.46.2\cmd;`+os.Getenv("PATH"))
		if os.Args[1] != "launchFresh" {
			cmd.Env = append(cmd.Env, "CSGCLAW_DESKTOP_CONFIG="+filepath.Join(profile, `.csgclaw\config.toml`))
		}
		log, e := os.Create(filepath.Join(profile, "launch.log"))
		check(e)
		cmd.Stdout = log
		cmd.Stderr = log
		check(cmd.Start())
		check(log.Close())
		fmt.Println("Desktop PID", cmd.Process.Pid)
		check(cmd.Process.Release())
	case "stop":
		raw, err := strconv.ParseUint(os.Args[2], 10, 32)
		check(err)
		pid := uint32(raw)
		owner, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
		check(e)
		defer windows.CloseHandle(owner)
		image := func(h windows.Handle) string {
			var b [32768]uint16
			size := uint32(len(b))
			check(windows.QueryFullProcessImageName(h, 0, &b[0], &size))
			return windows.UTF16ToString(b[:size])
		}
		expected := `C:\CSGClawTest\desktop-20261019\CSGClaw.exe`
		if !strings.EqualFold(image(owner), expected) {
			panic("refuse to stop unexpected application")
		}
		snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
		check(e)
		defer windows.CloseHandle(snapshot)
		var entry windows.ProcessEntry32
		entry.Size = uint32(unsafe.Sizeof(entry))
		var children []windows.Handle
		for e = windows.Process32First(snapshot, &entry); e == nil; e = windows.Process32Next(snapshot, &entry) {
			if entry.ParentProcessID != pid {
				continue
			}
			h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, entry.ProcessID)
			if e != nil {
				continue
			}
			if strings.EqualFold(image(h), filepath.Join(filepath.Dir(expected), `resources\backend\csgclaw\bin\csgclaw.exe`)) {
				children = append(children, h)
				fmt.Println("Holding actual Go sidecar handle", entry.ProcessID)
			} else {
				windows.CloseHandle(h)
			}
		}
		if len(children) != 1 {
			panic(fmt.Sprint("expected exactly one Go sidecar, got ", len(children)))
		}
		check(windows.TerminateProcess(owner, 0))
		for _, h := range children {
			state, e := windows.WaitForSingleObject(h, 120000)
			windows.CloseHandle(h)
			check(e)
			if state != windows.WAIT_OBJECT_0 {
				panic("Go sidecar did not stop after application owner exit")
			}
		}
		fmt.Println("Application owner stopped; Go sidecar exited through its control channel")
	case "environment":
		script, e := os.ReadFile(os.Args[2])
		check(e)
		command := exec.Command(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", string(script))
		raw, e := command.CombinedOutput()
		check(e)
		var values map[string]string
		check(json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(string(raw)), "\ufeff")), &values))
		if strings.TrimSpace(values["PATH"]) == "" {
			panic("Windows registry PATH was not captured")
		}
		check(json.NewEncoder(os.Stdout).Encode(map[string]any{"valid_json": true, "path_entries": len(strings.Split(values["PATH"], ";")), "powershell2_compatible": true}))
	case "fixtures":
		target := `C:\CSGClawTest\desktop-profile\.csgclaw\agents\agent-win7\.codex\workspace\win7-feature-previews`
		entries, e := os.ReadDir(`C:\CSGClawTest\browser-profile\.csgclaw\agents\agent-win7\.codex\workspace\win7-feature-previews`)
		check(e)
		count := 0
		for _, entry := range entries {
			if !entry.IsDir() {
				name := entry.Name()
				copyFile(filepath.Join(`C:\CSGClawTest\browser-profile\.csgclaw\agents\agent-win7\.codex\workspace\win7-feature-previews`, name), filepath.Join(target, name))
				count++
			}
		}
		fmt.Println("Copied synthetic preview fixtures", count)
	case "verify":
		var manifest struct {
			Files map[string]string `json:"files"`
		}
		raw, e := os.ReadFile(os.Args[2])
		check(e)
		check(json.Unmarshal(raw, &manifest))
		root := filepath.Dir(os.Args[2])
		for name, expected := range manifest.Files {
			if fileHash(filepath.Join(root, filepath.FromSlash(name))) != expected {
				panic("installed file hash mismatch: " + name)
			}
		}
		fmt.Println("Verified installed package SHA-256 files", len(manifest.Files))
	case "secondReview":
		cmd := exec.Command(filepath.Join(os.Args[2], "CSGClaw.exe"))
		cmd.Dir = os.Args[2]
		profile := `C:\CSGClawTest\desktop-profile`
		cmd.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile, "CSGCLAW_DESKTOP_CONFIG="+filepath.Join(profile, `.csgclaw\config.toml`))
		check(cmd.Run())
		fmt.Println("Unmodified EXE opened with no arguments and reused the existing application")
	case "logs":
		check(filepath.Walk(os.Args[2], func(p string, info os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") && info.Size() < 300000 {
				b, e := os.ReadFile(p)
				check(e)
				fmt.Printf("%s\n%s\n", p, b)
			}
			return nil
		}))
	default:
		check(json.NewEncoder(os.Stdout).Encode(map[string]string{"error": "unknown action"}))
	}
}
