package runner

import (
	"runtime"
	"strings"
)

// labelOS returns the operating system a runs-on label stands for ("linux", "darwin", "windows"), or "" when the
// label says nothing about it (custom labels, self-hosted, groups).
func labelOS(label string) string {
	l := strings.ToLower(strings.TrimSpace(label))
	switch {
	case strings.HasPrefix(l, "ubuntu"), strings.HasPrefix(l, "linux"), strings.HasPrefix(l, "debian"), strings.HasPrefix(l, "alpine"), strings.HasPrefix(l, "fedora"), strings.HasPrefix(l, "centos"):
		return "linux"
	case strings.HasPrefix(l, "macos"), l == "osx":
		return "darwin"
	case strings.HasPrefix(l, "windows"):
		return "windows"
	}
	return ""
}

// targetOS is the OS the job needs according to its labels ("" when it does not say).
func targetOS(labels []string) string {
	for _, l := range labels {
		if os := labelOS(l); os != "" {
			return os
		}
	}
	return ""
}

var hostName = map[string]string{"linux": "Linux", "darwin": "macOS", "windows": "Windows"}

// hostOS returns the name of this machine's OS family as the labels use it.
func hostOS() string { return runtime.GOOS }
