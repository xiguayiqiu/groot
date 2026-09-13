package proot

import (
	"path/filepath"
	"sort"
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
	// 按 GuestPath 长度降序排序，优先匹配最长前缀
	sorted := make([]Binding, len(pt.bindings))
	copy(sorted, pt.bindings)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].GuestPath) > len(sorted[j].GuestPath)
	})

	for _, b := range sorted {
		if guestPath == b.GuestPath {
			return b.HostPath, true
		}
		if strings.HasPrefix(guestPath, b.GuestPath+"/") {
			relativePath := guestPath[len(b.GuestPath):]
			return filepath.Join(b.HostPath, relativePath), true
		}
	}
	return "", false
}

func (pt *PathTranslator) HostToGuest(hostPath string) (string, bool) {
	// 按 HostPath 长度降序排序，优先匹配最长前缀
	sorted := make([]Binding, len(pt.bindings))
	copy(sorted, pt.bindings)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].HostPath) > len(sorted[j].HostPath)
	})

	for _, b := range sorted {
		if hostPath == b.HostPath {
			return b.GuestPath, true
		}
		if strings.HasPrefix(hostPath, b.HostPath+"/") {
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
