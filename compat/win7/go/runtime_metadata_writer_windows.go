//go:build windows

package codex

import (
	"errors"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

func init() {
	version := windows.RtlGetVersion()
	if version.MajorVersion == 6 && version.MinorVersion == 1 {
		writeRuntimeFile = writeWin7RuntimeFile
		readRuntimeFile = readWin7RuntimeFile
	}
}

// Fixed lock striping keeps storage bounded while preventing our readers from
// holding Win7 file handles open during replacement of the same metadata.
var win7MetadataLocks [32]sync.RWMutex

func win7MetadataLock(path string) *sync.RWMutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.ToLower(filepath.Clean(path))))
	return &win7MetadataLocks[hash.Sum32()%uint32(len(win7MetadataLocks))]
}

func readWin7RuntimeFile(path string) ([]byte, error) {
	name := filepath.Base(path)
	if name != runtimeFileName && name != sessionFileName {
		return os.ReadFile(path)
	}
	lock := win7MetadataLock(path)
	lock.RLock()
	defer lock.RUnlock()
	return os.ReadFile(path)
}

// Exit callbacks must commit metadata before a replacement session can start.
// Win7 can stall FileEndOfFileInfo on an existing, concurrently observed file.
// Replace complete metadata instead of exposing an in-place truncation window.
func writeWin7RuntimeFile(path string, data []byte, mode os.FileMode) error {
	name := filepath.Base(path)
	if name != runtimeFileName && name != sessionFileName {
		return os.WriteFile(path, data, mode)
	}
	lock := win7MetadataLock(path)
	lock.Lock()
	defer lock.Unlock()
	file, err := os.CreateTemp(filepath.Dir(path), "."+name+"-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err = file.Chmod(mode); err == nil {
		_, err = file.Write(data)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	for attempt := 0; ; attempt++ {
		err = os.Rename(temporary, path)
		if err == nil || attempt == 39 || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return err
		}
		// Readers opened without FILE_SHARE_DELETE can briefly delay replacement.
		time.Sleep(25 * time.Millisecond)
	}
}
