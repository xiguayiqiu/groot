package proot

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestFixAlpineRootfs_HardLinks 验证 fixAlpineRootfs 正确处理 Alpine 的 busybox 硬链接。
//
// Alpine Linux 默认将 /bin/sh、/bin/cat、/bin/ls 等作为到 /bin/busybox 的硬链接
//（多个目录项共享同一 inode）。filepath.Walk 会访问每个硬链接路径，
// 若不去重则会对同一 inode 重复执行 chmod/chown，
// 并且若某个路径因 isExec 检测失败被误设为 0644，
// 将影响整个 inode，致使 busybox 所有 applet 失效。
func TestFixAlpineRootfs_HardLinks(t *testing.T) {
	rootfs := t.TempDir()

	// 创建 bin/ 目录
	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	// 创建 "busybox" 二进制文件（模拟）
	busyboxPath := filepath.Join(rootfs, "bin", "busybox")
	busyboxContent := []byte("#!/bin/sh\necho busybox\n")
	if err := os.WriteFile(busyboxPath, busyboxContent, 0755); err != nil {
		t.Fatal(err)
	}

	// 创建硬链接：/bin/sh、/bin/cat、/bin/ls → /bin/busybox
	links := []string{"sh", "cat", "ls", "ash", "echo"}
	for _, l := range links {
		linkPath := filepath.Join(rootfs, "bin", l)
		if err := os.Link(busyboxPath, linkPath); err != nil {
			t.Fatalf("创建硬链接 %s 失败: %v", l, err)
		}
	}

	// 创建一个非可执行文件（模拟配置文件）
	etcDir := filepath.Join(rootfs, "etc")
	if err := os.MkdirAll(etcDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(etcDir, "test.conf")
	if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// 运行 fixAlpineRootfs（使用当前用户的 uid/gid）
	uid := os.Getuid()
	gid := os.Getgid()
	fixAlpineRootfs(rootfs, uid, gid)

	// 验证 busybox 本身保持可执行
	stat, err := os.Stat(busyboxPath)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode()&0111 == 0 {
		t.Errorf("busybox 应保持可执行权限，但获得 %v", stat.Mode())
	}

	// 验证所有硬链接的权限都正确（因为共享 inode，只需检查一个即可）
	// 但逐一检查以确保没有被意外修改
	for _, l := range links {
		linkPath := filepath.Join(rootfs, "bin", l)
		stat, err := os.Stat(linkPath)
		if err != nil {
			t.Fatalf("stat %s 失败: %v", l, err)
		}
		if stat.Mode()&0111 == 0 {
			t.Errorf("硬链接 /bin/%s 应保持可执行权限，但获得 %v", l, stat.Mode())
		}
	}

	// 验证非可执行文件被设置为 0644
	stat, err = os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode()&0111 != 0 {
		t.Errorf("配置文件应为非可执行，但获得 %v", stat.Mode())
	}

	// 验证所有硬链接共享同一个 inode（确认确实是硬链接）
	bbStat, _ := os.Stat(busyboxPath)
	bbSys := bbStat.Sys().(*syscall.Stat_t)
	for _, l := range links {
		linkPath := filepath.Join(rootfs, "bin", l)
		stat, _ := os.Stat(linkPath)
		sys := stat.Sys().(*syscall.Stat_t)
		if sys.Ino != bbSys.Ino {
			t.Errorf("/bin/%s 的 inode (%d) 与 busybox (%d) 不匹配，可能不是硬链接",
				l, sys.Ino, bbSys.Ino)
		}
	}

	// 验证硬链接数大于 1（确认确实存在硬链接关系）
	if bbStat.Mode()&os.ModeSymlink != 0 {
		t.Fatal("busybox 不应是符号链接")
	}
	bbSys = bbStat.Sys().(*syscall.Stat_t)
	if bbSys.Nlink <= 1 {
		t.Errorf("busybox 的硬链接数应 > 1，实际为 %d", bbSys.Nlink)
	}
}

// TestFixUltimatePermissions_HardLinks 验证 fixUltimatePermissions 也正确处理硬链接。
func TestFixUltimatePermissions_HardLinks(t *testing.T) {
	rootfs := t.TempDir()

	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	// 创建 busybox 二进制
	busyboxPath := filepath.Join(rootfs, "bin", "busybox")
	if err := os.WriteFile(busyboxPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// 创建硬链接
	for _, l := range []string{"sh", "cat", "ls"} {
		if err := os.Link(busyboxPath, filepath.Join(rootfs, "bin", l)); err != nil {
			t.Fatal(err)
		}
	}

	uid := os.Getuid()
	gid := os.Getgid()
	fixUltimatePermissions(rootfs, uid, gid)

	// 验证所有硬链接仍然可执行
	bbStat, _ := os.Stat(busyboxPath)
	bbSys := bbStat.Sys().(*syscall.Stat_t)
	if bbSys.Nlink <= 1 {
		t.Errorf("期望硬链接数 > 1，获得 %d", bbSys.Nlink)
	}
	for _, l := range []string{"sh", "cat", "ls"} {
		linkPath := filepath.Join(rootfs, "bin", l)
		stat, err := os.Stat(linkPath)
		if err != nil {
			t.Fatal(err)
		}
		if stat.Mode()&0111 == 0 {
			t.Errorf("硬链接 /bin/%s 应保持可执行，但获得 %v", l, stat.Mode())
		}
	}
}
