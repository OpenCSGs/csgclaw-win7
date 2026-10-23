//go:build windows

package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWin7RuntimeMetadataReadersNeverObservePartialJSON(t *testing.T) {
	v := windows.RtlGetVersion()
	if v.MajorVersion != 6 || v.MinorVersion != 1 {
		t.Skip("native Win7 metadata regression")
	}
	rt := &Runtime{}
	for _, name := range []string{runtimeFileName, sessionFileName} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			payload := []byte(fmt.Sprintf(`{"state":"running","padding":"%s"}`, bytes.Repeat([]byte("x"), 4096)))
			if err := rt.writeFile(path, payload, 0600); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			failures := make(chan error, 1)
			var readers sync.WaitGroup
			for i := 0; i < 4; i++ {
				readers.Add(1)
				go func() {
					defer readers.Done()
					for {
						select {
						case <-done:
							return
						default:
						}
						b, err := rt.readFile(path)
						if err == nil && !json.Valid(b) {
							err = fmt.Errorf("reader observed partial metadata (%d bytes)", len(b))
						}
						if err != nil {
							select {
							case failures <- err:
							default:
							}
							return
						}
					}
				}()
			}
			var writeErr error
			for i := 0; i < 64; i++ {
				if writeErr = rt.writeFile(path, payload, 0600); writeErr != nil {
					break
				}
			}
			close(done)
			readers.Wait()
			if writeErr != nil {
				t.Fatal(writeErr)
			}
			select {
			case err := <-failures:
				t.Fatal(err)
			default:
			}
			leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "."+name+"-*.tmp"))
			if len(leftovers) != 0 {
				t.Fatalf("temporary metadata leaked: %v", leftovers)
			}
		})
	}
}
