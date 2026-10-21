// Native installer acceptance. All test writes have explicit, disposable names.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const keyName = `Software\Microsoft\Windows\CurrentVersion\Uninstall\OpenCSG.CSGClaw.Win7`
const sentinel = "CSGClaw Win7 installer acceptance - disposable fixture\n"

type state struct {
	InstallDir, DesktopLink, StartLink, UninstallLink, DataSentinel, ExtraSentinel string
	Files                                                                          []string
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func installed() (registry.Key, string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyName, registry.QUERY_VALUE|registry.WOW64_64KEY)
	check(err)
	dir, _, err := k.GetStringValue("InstallLocation")
	check(err)
	return k, dir
}

func hash(path string) string {
	b, err := exec.Command(`C:\Windows\System32\certutil.exe`, "-hashfile", path, "SHA256").CombinedOutput()
	check(err)
	for _, line := range strings.Split(string(b), "\n") {
		value := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if len(value) == 64 {
			if _, err := hex.DecodeString(value); err == nil {
				return strings.ToLower(value)
			}
		}
	}
	panic("certutil returned no SHA256")
}

func addSentinel(path string) {
	if b, err := os.ReadFile(path); err == nil {
		if string(b) != sentinel {
			panic("refuse to overwrite a non-fixture file")
		}
		return
	}
	check(os.MkdirAll(filepath.Dir(path), 0700))
	check(os.WriteFile(path, []byte(sentinel), 0600))
}

func image(handle windows.Handle) string {
	var path [32768]uint16
	size := uint32(len(path))
	check(windows.QueryFullProcessImageName(handle, 0, &path[0], &size))
	return windows.UTF16ToString(path[:size])
}

func main() {
	switch os.Args[1] {
	case "inspect":
		k, dir := installed()
		defer k.Close()
		name, _, err := k.GetStringValue("DisplayName")
		check(err)
		if name != "CSGClaw Win7" {
			panic("incorrect uninstall registration")
		}
		command, _, err := k.GetStringValue("UninstallString")
		check(err)
		if command != `"`+filepath.Join(dir, "Uninstall.exe")+`"` {
			panic("incorrect uninstall command")
		}
		var manifest struct {
			Files map[string]string `json:"files"`
		}
		b, err := os.ReadFile(filepath.Join(dir, "win7-build.json"))
		check(err)
		check(json.Unmarshal(b, &manifest))
		st := state{InstallDir: dir}
		for name, expected := range manifest.Files {
			if hash(filepath.Join(dir, filepath.FromSlash(name))) != expected {
				panic("installed hash mismatch: " + name)
			}
			st.Files = append(st.Files, name)
		}
		desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
		check(err)
		programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
		check(err)
		st.DesktopLink = filepath.Join(desktop, "CSGClaw Win7.lnk")
		st.StartLink = filepath.Join(programs, "CSGClaw Win7", "CSGClaw Win7.lnk")
		st.UninstallLink = filepath.Join(programs, "CSGClaw Win7", "卸载 CSGClaw Win7.lnk")
		for _, path := range []string{st.DesktopLink, st.StartLink, st.UninstallLink} {
			_, err := os.Stat(path)
			check(err)
		}
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		script := "$s=New-Object -ComObject WScript.Shell;"
		for _, link := range []struct{ path, target string }{{st.DesktopLink, filepath.Join(dir, "CSGClaw.exe")}, {st.StartLink, filepath.Join(dir, "CSGClaw.exe")}, {st.UninstallLink, filepath.Join(dir, "Uninstall.exe")}} {
			script += "if($s.CreateShortcut(" + quote(link.path) + ").TargetPath -ine " + quote(link.target) + "){throw 'shortcut target mismatch'};"
		}
		check(exec.Command(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoProfile", "-NonInteractive", "-Command", script).Run())
		home, err := os.UserHomeDir()
		check(err)
		st.DataSentinel = filepath.Join(home, ".csgclaw", "installer-acceptance-sentinel.txt")
		st.ExtraSentinel = filepath.Join(dir, "installer-user-files", "keep.txt")
		addSentinel(st.DataSentinel)
		addSentinel(st.ExtraSentinel)
		b, err = json.MarshalIndent(st, "", "  ")
		check(err)
		check(os.WriteFile("installer-state.json", b, 0600))
		check(json.NewEncoder(os.Stdout).Encode(map[string]any{"install_dir": dir, "files_verified": len(st.Files), "uninstall_registration": true, "shortcuts": 3}))
	case "stop":
		k, dir := installed()
		k.Close()
		pid, err := strconv.ParseUint(os.Args[2], 10, 32)
		check(err)
		owner, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
		check(err)
		defer windows.CloseHandle(owner)
		if !strings.EqualFold(image(owner), filepath.Join(dir, "CSGClaw.exe")) {
			panic("refuse to stop a process outside the installed application")
		}
		snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
		check(err)
		defer windows.CloseHandle(snapshot)
		entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
		var children []windows.Handle
		for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
			if entry.ParentProcessID != uint32(pid) {
				continue
			}
			h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, entry.ProcessID)
			if err != nil {
				continue
			}
			if strings.EqualFold(image(h), filepath.Join(dir, `resources\backend\csgclaw\bin\csgclaw.exe`)) {
				children = append(children, h)
			} else {
				windows.CloseHandle(h)
			}
		}
		if len(children) != 1 {
			panic("expected exactly one installed Go sidecar")
		}
		check(windows.TerminateProcess(owner, 0))
		for _, child := range children {
			status, err := windows.WaitForSingleObject(child, 60000)
			windows.CloseHandle(child)
			check(err)
			if status != windows.WAIT_OBJECT_0 {
				panic("installed sidecar did not exit with its owner")
			}
		}
		fmt.Println("Installed application stopped; Go sidecar reclaimed")
	case "check-running-guards":
		k, dir := installed()
		k.Close()
		original := hash(filepath.Join(dir, "ffmpeg.dll"))
		commands := []struct{ path, line string }{
			{os.Args[2], `"` + os.Args[2] + `" /S`},
			{filepath.Join(dir, "Uninstall.exe"), `"` + filepath.Join(dir, "Uninstall.exe") + `" /S _?=` + dir},
		}
		for _, command := range commands {
			cmd := exec.Command(command.path)
			cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: command.line}
			// CmdLine supplies the full native NSIS syntax, including its final unquoted _?= path.
			err := cmd.Run()
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 2 {
				panic(fmt.Sprintf("running application was not protected: %v", err))
			}
		}
		if hash(filepath.Join(dir, "ffmpeg.dll")) != original {
			panic("running-app guard changed the installation")
		}
		fmt.Println("Running application blocked both reinstall and uninstall with exit 2; payload retained")
	case "interrupt-and-retry":
		var st state
		b, err := os.ReadFile("installer-state.json")
		check(err)
		check(json.Unmarshal(b, &st))
		if _, err := os.Stat(st.InstallDir); !os.IsNotExist(err) {
			panic("interruption test requires the previously uninstalled fixture directory")
		}
		cmd := exec.Command(os.Args[2], "/S")
		check(cmd.Start())
		deadline := time.Now().Add(2 * time.Minute)
		for {
			b, err := os.ReadFile(filepath.Join(st.InstallDir, "win7-install.ini"))
			if err == nil && strings.Contains(string(b), "OpenCSG.CSGClaw.Win7") {
				break
			}
			if time.Now().After(deadline) {
				cmd.Process.Kill()
				cmd.Wait()
				panic("installer did not establish early ownership")
			}
			time.Sleep(20 * time.Millisecond)
		}
		check(cmd.Process.Kill())
		cmd.Wait()
		check(exec.Command(os.Args[2], "/S").Run())
		fmt.Println("Fresh installation interrupted after ownership marker; rerun completed successfully")
	case "assert-uninstalled":
		var st state
		b, err := os.ReadFile("installer-state.json")
		check(err)
		check(json.Unmarshal(b, &st))
		deadline := time.Now().Add(time.Minute)
		for {
			_, err = os.Stat(filepath.Join(st.InstallDir, "Uninstall.exe"))
			if os.IsNotExist(err) {
				break
			}
			if time.Now().After(deadline) {
				panic("uninstaller did not finish")
			}
			time.Sleep(time.Second)
		}
		if k, err := registry.OpenKey(registry.CURRENT_USER, keyName, registry.QUERY_VALUE|registry.WOW64_64KEY); err == nil {
			k.Close()
			panic("uninstall registration was retained")
		}
		paths := []string{st.DesktopLink, st.StartLink, st.UninstallLink, filepath.Join(st.InstallDir, "win7-build.json"), filepath.Join(st.InstallDir, "win7-install.ini")}
		for _, name := range st.Files {
			paths = append(paths, filepath.Join(st.InstallDir, filepath.FromSlash(name)))
		}
		for _, path := range paths {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				panic("owned install file remains: " + path)
			}
		}
		for _, path := range []string{st.DataSentinel, st.ExtraSentinel} {
			b, err := os.ReadFile(path)
			check(err)
			if string(b) != sentinel {
				panic("user data was changed during uninstall")
			}
			check(os.Remove(path))
		}
		check(os.Remove(filepath.Dir(st.ExtraSentinel)))
		check(os.Remove(st.InstallDir))
		fmt.Println("Uninstall passed: program files, shortcuts and registration removed; profile and extra user files preserved")
	default:
		panic("expected inspect, stop, check-running-guards, interrupt-and-retry or assert-uninstalled")
	}
}
