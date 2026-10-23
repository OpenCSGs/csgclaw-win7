package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type Step struct {
	Action, Name, Selector, Text, Script, Path string
	Timeout                                    int
}

var client = http.Client{Timeout: 180 * time.Second}
var session string
var tracked []syscall.Handle

func request(method, path string, body any) (any, error) {
	var payload io.Reader
	if method == http.MethodPost || body != nil {
		if body == nil {
			body = map[string]any{}
		}
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(b)
	}
	r, e := http.NewRequest(method, "http://127.0.0.1:48822"+path, payload)
	if e != nil {
		return nil, e
	}
	r.Header.Set("Content-Type", "application/json")
	response, e := client.Do(r)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	b, e := io.ReadAll(response.Body)
	if e != nil {
		return nil, e
	}
	var result map[string]any
	if e = json.Unmarshal(b, &result); e != nil {
		return nil, fmt.Errorf("decode %s: %w", path, e)
	}
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s: %s", method, path, b)
	}
	return result["value"], nil
}

func script(code string) (any, error) {
	return request("POST", "/session/"+session+"/execute/sync", map[string]any{"script": code, "args": []any{}})
}

func element(selector string) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		v, e := request("POST", "/session/"+session+"/element", map[string]string{"using": "css selector", "value": selector})
		if e == nil {
			return v.(map[string]any)["element-6066-11e4-a52e-4f735466cecf"].(string), nil
		}
		if !strings.Contains(e.Error(), "no such element") || time.Now().After(deadline) {
			return "", e
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func screenshot(name string) error {
	v, e := request("GET", "/session/"+session+"/screenshot", nil)
	if e != nil {
		return e
	}
	b, e := base64.StdEncoding.DecodeString(v.(string))
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Base(name), b, 0600)
}

func run(s Step) (any, error) {
	switch s.Action {
	case "start":
		v, e := request("POST", "/session", map[string]any{"capabilities": map[string]any{"alwaysMatch": map[string]any{"browserName": "chrome", "pageLoadStrategy": "eager", "goog:chromeOptions": map[string]any{"debuggerAddress": "127.0.0.1:48824"}, "goog:loggingPrefs": map[string]string{"browser": "ALL"}}}})
		if e != nil {
			return nil, e
		}
		session = v.(map[string]any)["sessionId"].(string)
		os.WriteFile("electron-session.txt", []byte(session), 0600)
		request("POST", "/session/"+session+"/timeouts", map[string]int{"script": 60000, "pageLoad": 120000, "implicit": 0})
		deadline := time.Now().Add(90 * time.Second)
		for {
			handles, err := request("GET", "/session/"+session+"/window/handles", nil)
			if err == nil {
				for _, raw := range handles.([]any) {
					id := raw.(string)
					_, err = request("POST", "/session/"+session+"/window", map[string]string{"handle": id})
					if err != nil {
						continue
					}
					url, err := request("GET", "/session/"+session+"/url", nil)
					if err == nil && strings.HasPrefix(url.(string), "http://127.0.0.1:18080/") {
						os.WriteFile("electron-main-window.txt", []byte(id), 0600)
						return v, nil
					}
				}
			}
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("application renderer target did not appear")
			}
			time.Sleep(time.Second)
		}
	case "clickIfPresent":
		v, e := request("POST", "/session/"+session+"/elements", map[string]string{"using": "css selector", "value": s.Selector})
		if e != nil {
			return nil, e
		}
		for _, raw := range v.([]any) {
			id := raw.(map[string]any)["element-6066-11e4-a52e-4f735466cecf"].(string)
			shown, e := request("GET", "/session/"+session+"/element/"+id+"/displayed", nil)
			if e == nil && shown == true {
				return request("POST", "/session/"+session+"/element/"+id+"/click", map[string]any{})
			}
		}
		return "no visible retained panel", nil
	case "rememberWindow":
		v, e := request("GET", "/session/"+session+"/window", nil)
		if e == nil {
			e = os.WriteFile("electron-main-window.txt", []byte(v.(string)), 0600)
		}
		return v, e
	case "mainWindow":
		b, e := os.ReadFile("electron-main-window.txt")
		if e != nil {
			return nil, e
		}
		return request("POST", "/session/"+session+"/window", map[string]string{"handle": string(b)})
	case "remoteWindow":
		deadline := time.Now().Add(60 * time.Second)
		for {
			v, e := request("GET", "/session/"+session+"/window/handles", nil)
			if e != nil {
				return nil, e
			}
			for _, raw := range v.([]any) {
				id := raw.(string)
				if _, e = request("POST", "/session/"+session+"/window", map[string]string{"handle": id}); e != nil {
					continue
				}
				url, e := request("GET", "/session/"+session+"/url", nil)
				if e == nil && strings.Contains(url.(string), s.Text) {
					return url, nil
				}
			}
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("no remote browser target matching %s", s.Text)
			}
			time.Sleep(time.Second)
		}
	case "browserLogs":
		return request("POST", "/session/"+session+"/log", map[string]string{"type": "browser"})
	case "navigate":
		return request("POST", "/session/"+session+"/url", map[string]string{"url": s.Text})
	case "rect":
		return request("POST", "/session/"+session+"/window/rect", map[string]int{"width": 1440, "height": 1000})
	case "script":
		return script(s.Script)
	case "insertText":
		return request("POST", "/session/"+session+"/goog/cdp/execute", map[string]any{"cmd": "Input.insertText", "params": map[string]string{"text": s.Text}})
	case "clickText":
		selector, _ := json.Marshal(s.Selector)
		wanted, _ := json.Marshal(s.Text)
		v, e := script("return Array.from(document.querySelectorAll(" + string(selector) + ")).find(e=>e.textContent.trim()===" + string(wanted) + ");")
		if e != nil {
			return nil, e
		}
		if v == nil {
			return nil, fmt.Errorf("no matching text %q", s.Text)
		}
		id := v.(map[string]any)["element-6066-11e4-a52e-4f735466cecf"].(string)
		return request("POST", "/session/"+session+"/element/"+id+"/click", map[string]any{})
	case "frame":
		if s.Selector == "" {
			return request("POST", "/session/"+session+"/frame", map[string]any{"id": nil})
		}
		id, e := element(s.Selector)
		if e != nil {
			return nil, e
		}
		return request("POST", "/session/"+session+"/frame", map[string]any{"id": map[string]string{"element-6066-11e4-a52e-4f735466cecf": id}})

	case "click", "keys", "clear":
		deadline := time.Now().Add(30 * time.Second)
		for {
			id, e := element(s.Selector)
			if e != nil {
				return nil, e
			}
			if s.Action == "clear" {
				return request("POST", "/session/"+session+"/element/"+id+"/clear", map[string]any{})
			}
			if s.Action == "keys" {
				return request("POST", "/session/"+session+"/element/"+id+"/value", map[string]string{"text": s.Text})
			}
			v, e := request("POST", "/session/"+session+"/element/"+id+"/click", map[string]any{})
			if e == nil || !strings.Contains(e.Error(), "stale element reference") || time.Now().After(deadline) {
				return v, e
			}
			time.Sleep(250 * time.Millisecond)
		}
	case "wait":
		seconds := s.Timeout
		if seconds == 0 {
			seconds = 90
		}
		deadline := time.Now().Add(time.Duration(seconds) * time.Second)
		for {
			v, e := script(s.Script)
			if e == nil && v == true {
				return true, nil
			}
			if time.Now().After(deadline) {
				return v, fmt.Errorf("wait %s timed out: %v %v", s.Name, v, e)
			}
			time.Sleep(time.Second)
		}
	case "prepareTree":
		if !strings.HasPrefix(filepath.Base(s.Path), "webdriver-") || !strings.HasSuffix(s.Path, "-pids.json") {
			return nil, fmt.Errorf("not a disposable WebDriver PID marker: %s", s.Path)
		}
		e := os.Remove(s.Path)
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		return "test PID marker prepared", nil
	case "trackChildren", "trackCurrentTree":
		seconds := s.Timeout
		if seconds == 0 {
			seconds = 300
		}
		deadline := time.Now().Add(time.Duration(seconds) * time.Second)
		for {
			var b []byte
			var e error
			if s.Action == "trackCurrentTree" {
				var value any
				value, e = script(s.Script)
				if e == nil && value != nil {
					b, e = json.Marshal(value)
				} else if e == nil {
					e = os.ErrNotExist
				}
			} else {
				b, e = os.ReadFile(s.Path)
			}
			if e == nil {
				var pids []int
				if e = json.Unmarshal(b, &pids); e != nil {
					return nil, e
				}
				if len(pids) != 2 {
					return nil, fmt.Errorf("expected parent/child PIDs: %v", pids)
				}
				for _, pid := range pids {
					h, e := syscall.OpenProcess(0x100000|0x1000, false, uint32(pid))
					if e != nil {
						return nil, e
					}
					state, e := syscall.WaitForSingleObject(h, 0)
					if e != nil || state != 258 {
						return nil, fmt.Errorf("PID %d is not running: %d %v", pid, state, e)
					}
					if s.Text != "" {
						var path [32768]uint16
						size := uint32(len(path))
						r, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW").Call(uintptr(h), 0, uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&size)))
						if r == 0 || !strings.EqualFold(syscall.UTF16ToString(path[:size]), s.Text) {
							syscall.CloseHandle(h)
							return nil, fmt.Errorf("PID %d unexpected executable: %s %v", pid, syscall.UTF16ToString(path[:size]), err)
						}
					}

					tracked = append(tracked, h)
				}
				return pids, nil
			}
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("process tree marker not created: %s", s.Path)
			}
			time.Sleep(time.Second)
		}
	case "assertChildrenStopped":
		if len(tracked) != 2 {
			return nil, fmt.Errorf("missing tracked process handles")
		}
		for _, h := range tracked {
			state, e := syscall.WaitForSingleObject(h, 15000)
			syscall.CloseHandle(h)
			if e != nil || state != 0 {
				return nil, fmt.Errorf("process did not exit: %d %v", state, e)
			}
		}
		tracked = nil
		return "both parent and child exited", nil
	case "screenshot":
		return s.Path, screenshot(s.Path)
	case "quit":
		return request("DELETE", "/session/"+session, nil)
	default:
		return nil, fmt.Errorf("unknown action %q", s.Action)
	}
}

func main() {
	if b, e := os.ReadFile("electron-session.txt"); e == nil {
		session = strings.TrimSpace(string(b))
	}
	b, e := os.ReadFile(os.Args[1])
	if e != nil {
		panic(e)
	}
	var steps []Step
	if e = json.Unmarshal(b, &steps); e != nil {
		panic(e)
	}
	resultPath := strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1])) + ".results.json"
	results := []map[string]any{}
	failed := false
	for _, s := range steps {
		start := time.Now()
		v, e := run(s)
		r := map[string]any{"action": s.Action, "name": s.Name, "value": v, "elapsed": time.Since(start).String()}
		if e != nil {
			r["error"] = e.Error()
			failed = true
			screenshot(strings.TrimSuffix(resultPath, ".json") + ".failure.png")
		}
		results = append(results, r)
		fmt.Println(s.Action, s.Name, e)
		b, _ = json.MarshalIndent(results, "", "  ")
		os.WriteFile(resultPath, b, 0600)
		progress, _ := json.Marshal(map[string]any{"kind": "webdriver-progress", "scenario": os.Args[1], "step": r})
		progressClient := http.Client{Timeout: 3 * time.Second}
		if response, err := progressClient.Post("http://10.0.2.2:48817/results", "application/json", bytes.NewReader(progress)); err == nil {
			response.Body.Close()
		}
		if failed {
			break
		}
	}
	if failed {
		os.Exit(1)
	}
}
