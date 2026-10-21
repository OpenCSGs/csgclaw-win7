//go:build windows

package codex

import (
	"time"

	"golang.org/x/sys/windows"
)

func init() {
	version := windows.RtlGetVersion()
	if version.MajorVersion == 6 && version.MinorVersion == 1 {
		// Win7 may finish process-tree and runtime metadata cleanup after the
		// original three-second budget, especially under CPU/disk contention.
		// Still wait for live.done: starting a replacement before OnExit ends
		// would let stale exit metadata overwrite the new running session.
		appServerStopTimeout = 30 * time.Second
	}
}
