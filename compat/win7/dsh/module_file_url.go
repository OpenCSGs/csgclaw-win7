package dsh

import (
	"net/url"
	"path/filepath"
	"strings"
)

// Node's ESM loader interprets a Windows drive letter as a URL scheme.
// Plugin names must be file URLs, with filename characters escaped as data.
func moduleFileURL(path string) string {
	path = filepath.ToSlash(path)
	host := ""
	if strings.HasPrefix(path, "//") {
		host, path, _ = strings.Cut(strings.TrimPrefix(path, "//"), "/")
		path = "/" + path
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Host: host, Path: path}).String()
}
