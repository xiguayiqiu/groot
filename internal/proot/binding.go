
package proot

import (
	"path/filepath"
	"strings"
)

type Binding struct {
	HostPath  string
	GuestPath string
}

type PathTranslator struct {
	bindings []Binding
}

func NewPathTranslator(rootfs string) *PathTranslator {
	pt := &PathTranslator{
		bindings: []Binding{
			{
				HostPath:  rootfs,
				GuestPath: "/",
			},
		},
	}

	recommendedBindings := []string{
		"/etc/host.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/resolv.conf",
		"/etc/passwd",
		"/etc/group",
		"/dev/zero",
		"/dev/null",
		"/dev/random",
		"/dev/urandom",
		"/dev/shm",
		"/dev/ptmx",
		"/proc",
		"/sys",
		"/tmp",
	}

	for _, b := range recommendedBindings {
		pt.AddBinding(b, b)
	}

	return pt
}

func (pt *PathTranslator) AddBinding(hostPath, guestPath string) {
	if guestPath == "" {
		guestPath = hostPath
	}

	pt.bindings = append(pt.bindings, Binding{
		HostPath:  hostPath,
		GuestPath: guestPath,
	})
}

func (pt *PathTranslator) GuestToHost(guestPath string) (string, bool) {
	for _, b := range pt.bindings {
		if guestPath == b.GuestPath || strings.HasPrefix(guestPath, b.GuestPath+"/") {
			relativePath := guestPath[len(b.GuestPath):]
			return filepath.Join(b.HostPath, relativePath), true
		}
	}
	return "", false
}

func (pt *PathTranslator) HostToGuest(hostPath string) (string, bool) {
	for _, b := range pt.bindings {
		if hostPath == b.HostPath || strings.HasPrefix(hostPath, b.HostPath+"/") {
			relativePath := hostPath[len(b.HostPath):]
			result := filepath.Join(b.GuestPath, relativePath)
			return result, true
		}
	}
	return "", false
}

func (pt *PathTranslator) TranslatePath(path string) string {
	if translated, ok := pt.GuestToHost(path); ok {
		return translated
	}
	return path
}

func (pt *PathTranslator) TranslatePathBack(path string) string {
	if translated, ok := pt.HostToGuest(path); ok {
		return translated
	}
	return path
}

func cleanPath(path string) string {
	path = filepath.Clean(path)
	if path == "." {
		return "/"
	}
	return path
}

func isPrefix(path string) bool {
	for path != "/" {
		if path == "" {
			return false
		}
		path = filepath.Dir(path)
	}
	return true
}
