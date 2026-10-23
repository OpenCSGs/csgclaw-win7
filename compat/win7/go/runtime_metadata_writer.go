package codex

import "os"

// Keep the normal writer unless the native Win7 overlay selects its metadata writer.
var writeRuntimeFile = os.WriteFile
var readRuntimeFile = os.ReadFile
