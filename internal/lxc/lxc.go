//go:build !lxc
// +build !lxc

package lxc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"groot/internal/env"
	"groot/internal/logger"
	"groot/internal/network"
	"groot/internal/permission"
	"groot/internal/termux"
)

const (
	lxcDir           = "/data/data/com.termux/files/usr/share/groot/lxc"
	lxcContainersDir = "/data/data/com.termux/files/usr/share/groot/lxc/containers"
	lxcConfigFile    = "config.json"

	// readyFd：子进程（lxc-child）通过 ExtraFiles[0] 继承就绪通知管道写端，
	// 按 Go 标准库约定 ExtraFiles 第 i 项对应子进程 fd 3+i，因此这里固定为 3。
	readyFd = 3

	// 容器初始化超时（父进程等待子进程报告就绪的最长时间）
	initTimeout = 10 * time.Second
)

type ContainerConfig struct {
	Name       string `json:"name"`
	Rootfs     string `json:"rootfs"`
	Shell      string `json:"shell"`
	CreatedAt  string `json:"created_at"`
	Running    bool   `json:"running"`
	PID        int    `json:"pid"`
	NetEnabled bool   `json:"net_enabled"`
}

func Init() error {
	if err := os.MkdirAll(lxcDir, 0755); err != nil {
		return fmt.Errorf("创建 lxc 目录失败: %w", err)
	}
	if err := os.MkdirAll(lxcContainersDir, 0755); err != nil {
		return fmt.Errorf("创建容器目录失败: %w", err)
	}
	return nil
}

func checkRoot() error {
	if !permission.IsRoot() {
		if termux.IsTermux() {
			return fmt.Errorf("lxc 需要 root 权限，请先获取 root 权限")
		}
		return fmt.Errorf("lxc 需要 root 权限")
	}
	return nil
}

func getContainerPath(name string) string {
	return filepath.Join(lxcContainersDir, name)
}

func getConfigPath(name string) string {
	return filepath.Join(getContainerPath(name), lxcConfigFile)
}

func SaveConfig(config ContainerConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return os.WriteFile(getConfigPath(config.Name), data, 0644)
}

func LoadConfig(name string) (*ContainerConfig, error) {
	data, err := os.ReadFile(getConfigPath(name))
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	var config ContainerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	return &config, nil
}

func ListContainers() ([]ContainerConfig, error) {
	entries, err := os.ReadDir(lxcContainersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ContainerConfig{}, nil
		}
		return nil, fmt.Errorf("读取容器目录失败: %w", err)
	}

	var containers []ContainerConfig
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		config, err := LoadConfig(entry.Name())
		if err != nil {
			logger.Warn("加载容器 %s 配置失败: %v", entry.Name(), err)
			continue
		}
		containers = append(containers, *config)
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	return containers, nil
}

func ContainerExists(name string) bool {
	_, err := os.Stat(getConfigPath(name))
	return err == nil
}

// shellExistsInRootfs 检查 shell 是否存在于 rootfs 中。
// Alpine rootfs 使用 busybox 硬链接/符号链接，例如 /bin/ash -> /bin/busybox。
// os.Stat 会跟踪符号链接到主机的绝对路径（/bin/busybox 不存在于宿主机），
// 导致误判为 "shell 不存在"。
// 此函数使用 os.Lstat 检查路径是否存在，并为符号链接正确解析目标：
// 如果符号链接指向绝对路径（如 /bin/busybox），则在 rootfs 内部解析该路径。
func shellExistsInRootfs(rootfs, shell string) bool {
	shellRel := shell
	if shellRel != "" && shellRel[0] == '/' {
		shellRel = shellRel[1:]
	}
	fullPath := filepath.Join(rootfs, shellRel)

	info, err := os.Lstat(fullPath)
	if err != nil {
		return false
	}

	// 如果是符号链接，解析其目标
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(fullPath)
		if err != nil {
			return false
		}
		// 绝对路径需在 rootfs 内部解析（例如 Alpine 的 /bin/ash -> /bin/busybox）
		if filepath.IsAbs(target) {
			resolvedPath := filepath.Join(rootfs, target)
			if _, err := os.Stat(resolvedPath); err != nil {
				return false
			}
		}
		// 相对路径由 os.Stat 自动在正确位置解析，无需额外处理
	}

	return true
}

func CreateContainer(rootfs, shell, name string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	if ContainerExists(name) {
		return fmt.Errorf("容器 '%s' 已存在", name)
	}

	absRootfs, err := filepath.Abs(rootfs)
	if err != nil {
		return fmt.Errorf("转换路径失败: %w", err)
	}

	if _, err := os.Stat(absRootfs); err != nil {
		return fmt.Errorf("rootfs 路径不存在: %s", absRootfs)
	}

	// 验证 shell 解释器存在于 rootfs 中，避免启动后无法登录
	// 使用 shellExistsInRootfs 而非 os.Stat，直接对 filepath.Join(rootfs, shell)
	// 的 os.Stat 会错误地跟踪绝对符号链接到宿主机（例如 Alpine 的
	// /bin/ash -> /bin/busybox），导致误判。
	if shell != "" && !shellExistsInRootfs(absRootfs, shell) {
		return fmt.Errorf("shell '%s' 不存在于 rootfs 中，请检查", shell)
	}

	containerPath := getContainerPath(name)
	if err := os.MkdirAll(containerPath, 0755); err != nil {
		return fmt.Errorf("创建容器目录失败: %w", err)
	}

	config := ContainerConfig{
		Name:       name,
		Rootfs:     absRootfs,
		Shell:      shell,
		CreatedAt:  time.Now().Format(time.RFC3339),
		Running:    false,
		PID:        0,
		NetEnabled: true,
	}

	if err := SaveConfig(config); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	logger.Info("容器 '%s' 创建成功", name)
	logger.Info("  Rootfs: %s", absRootfs)
	logger.Info("  Shell: %s", shell)

	return nil
}

func DeleteContainer(name string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	if !ContainerExists(name) {
		return fmt.Errorf("容器 '%s' 不存在", name)
	}

	config, err := LoadConfig(name)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if config.Running {
		return fmt.Errorf("容器 '%s' 正在运行，请先停止", name)
	}

	containerPath := getContainerPath(name)
	if err := os.RemoveAll(containerPath); err != nil {
		return fmt.Errorf("删除容器失败: %w", err)
	}

	logger.Info("容器 '%s' 已删除", name)
	return nil
}

func StartContainer(name string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	if !ContainerExists(name) {
		return fmt.Errorf("容器 '%s' 不存在", name)
	}

	config, err := LoadConfig(name)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if config.Running {
		return fmt.Errorf("容器 '%s' 已经在运行", name)
	}

	if _, err := os.Stat(config.Rootfs); err != nil {
		return fmt.Errorf("rootfs 路径不存在: %s", config.Rootfs)
	}

	// 清理上次异常退出可能残留的宿主机网络资源（veth / NAT 规则）
	if config.NetEnabled {
		network.CleanupNetworkOnHost()
	}

	logger.Info("启动容器 '%s'...", name)
	pid, err := startContainerProcess(*config)
	if err != nil {
		if config.NetEnabled {
			network.CleanupNetworkOnHost()
		}
		return fmt.Errorf("启动容器失败: %w", err)
	}

	config.Running = true
	config.PID = pid
	if err := SaveConfig(*config); err != nil {
		logger.Warn("保存运行状态失败: %v", err)
	}

	logger.Info("容器 '%s' 已启动 (PID: %d)", name, pid)
	return nil
}

// startContainerProcess 启动容器守护进程。
//
// 关键设计：
//  1. 子进程通过 Setsid 脱离终端会话成为会话首领，彻底与宿主机 shell 的
//     进程组/作业控制/终端隔离，不会破坏宿主机 shell；
//  2. 子进程 stdin/stdout/stderr 全部指向 /dev/null；
//  3. 通过同步管道 (ExtraFiles[0] -> fd 3) 等待子进程完成 chroot 与挂载后
//     报告就绪，确保启动命令返回时容器已真正可用；
//  4. 网络模式下在子进程的网络命名空间中配置 veth + NAT（与 chroot 模式一致）。
func startContainerProcess(config ContainerConfig) (int, error) {
	exePath := permission.GetExecutablePath()
	if exePath == "" {
		return 0, fmt.Errorf("无法确定 groot 可执行文件路径")
	}

	// 就绪同步管道
	readyR, readyW, err := os.Pipe()
	if err != nil {
		return 0, fmt.Errorf("创建同步管道失败: %w", err)
	}

	args := []string{exePath, "lxc-child", config.Rootfs, config.Shell, config.Name}

	cmd := exec.Command(args[0], args[1:]...)

	// 命名空间：Mount / PID / UTS / IPC，网络模式再加 NET
	var cloneFlags uintptr = syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC
	if config.NetEnabled {
		cloneFlags |= syscall.CLONE_NEWNET
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneFlags,
		Setsid:     true, // 关键：脱离终端会话，绝不影响宿主机 shell
	}

	// 守护进程化：标准输入输出全部指向 /dev/null
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	// 就绪通知管道：子进程继承为 fd 3
	cmd.ExtraFiles = []*os.File{readyW}

	if err := cmd.Start(); err != nil {
		readyR.Close()
		readyW.Close()
		return 0, fmt.Errorf("启动容器进程失败: %w", err)
	}

	pid := cmd.Process.Pid
	readyW.Close()

	// 等待子进程完成初始化（最多 initTimeout）
	if err := waitForReady(readyR, pid); err != nil {
		readyR.Close()
		return 0, err
	}
	readyR.Close()

	// 网络模式：在子进程的网络命名空间中配置 veth / NAT / DNS
	if config.NetEnabled {
		time.Sleep(100 * time.Millisecond)
		if err := network.SetupNetworkInChildNs(pid); err != nil {
			logger.Warn("设置容器网络失败: %v", err)
		} else {
			_ = network.SetupChildDns(config.Rootfs)
			logger.Info("容器网络配置完成")
		}
	}

	return pid, nil
}

// waitForReady 等待子进程完成初始化。子进程成功时写入 "OK\n"。
// 若收到非 OK 内容、超时或管道提前关闭，都给出明确错误。
func waitForReady(r *os.File, pid int) error {
	r.SetDeadline(time.Now().Add(initTimeout))
	msg := ""

	for {
		b := make([]byte, 1)
		n, e := r.Read(b)
		if e != nil {
			if errors.Is(e, os.ErrDeadlineExceeded) {
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
					return fmt.Errorf("容器进程启动后立即退出")
				}
				return fmt.Errorf("容器初始化超时，请检查 rootfs 是否完整")
			}
			return fmt.Errorf("等待容器就绪失败: %v", e)
		}
		if n == 0 {
			break
		}
		if b[0] == '\n' {
			if msg == "OK" {
				return nil
			}
			return fmt.Errorf("容器初始化失败: %s", msg)
		}
		msg += string(b)
	}

	// 管道在读到换行前就关闭：子进程异常退出
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		return fmt.Errorf("容器进程启动后立即退出")
	}
	return fmt.Errorf("容器初始化异常结束，请检查 rootfs 是否完整")
}
func StopContainer(name string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	if !ContainerExists(name) {
		return fmt.Errorf("容器 '%s' 不存在", name)
	}

	config, err := LoadConfig(name)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if !config.Running {
		return fmt.Errorf("容器 '%s' 未运行", name)
	}

	pid := config.PID
	if pid > 0 {
		logger.Info("正在停止容器 '%s' (PID: %d)...", name, pid)

		// 先优雅停止：SIGTERM 被容器 init（lxc-child）捕获后会卸载文件系统并退出
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			logger.Warn("发送 SIGTERM 失败: %v", err)
		}

		// 等待最多 5 秒
		for i := 0; i < 50; i++ {
			if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// 仍未退出则强制终止
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
			logger.Warn("容器 %s 未响应 SIGTERM，强制终止...", name)
			syscall.Kill(pid, syscall.SIGKILL)
			time.Sleep(300 * time.Millisecond)
		}
	}

	// 清理宿主机端网络资源
	if config.NetEnabled {
		network.CleanupNetworkOnHost()
	}

	config.Running = false
	config.PID = 0
	if err := SaveConfig(*config); err != nil {
		logger.Warn("保存停止状态失败: %v", err)
	}

	logger.Info("容器 '%s' 已停止", name)
	return nil
}

func LoginContainer(name string, customShell string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	if !ContainerExists(name) {
		return fmt.Errorf("容器 '%s' 不存在", name)
	}

	config, err := LoadConfig(name)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if !config.Running {
		return fmt.Errorf("容器 '%s' 未运行，请先使用 'groot lxc start %s' 启动", name, name)
	}

	// 验证容器进程是否存活
	if config.PID > 0 {
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", config.PID)); err != nil {
			config.Running = false
			config.PID = 0
			SaveConfig(*config)
			return fmt.Errorf("容器 '%s' 进程已退出，请重新启动", name)
		}
	}

	shell := config.Shell
	if customShell != "" {
		shell = customShell
	}

	// 规范化 shell 路径并验证容器内存在
	// 使用 shellExistsInRootfs 而非 os.Stat，直接对 os.Stat(filepath.Join(rootfs, shell))
	// 会错误地跟踪绝对绝对符号链接到宿主机（例如 Alpine 的 /bin/ash -> /bin/busybox），
	// 导致误判。shellExistsInRootfs 会正确处理符号链接。
	shellRel := shell
	if shellRel == "" {
		shellRel = "/bin/sh"
		shell = shellRel
	}
	if shellRel[0] == '/' {
		shellRel = shellRel[1:]
	}
	if !shellExistsInRootfs(config.Rootfs, shellRel) {
		return fmt.Errorf("容器内找不到 shell: %s", shell)
	}

	// 查找 nsenter
	nsenter, nserr := termux.SafeLookPath("nsenter")
	if nserr != nil {
		return fmt.Errorf("未找到 nsenter，请安装 util-linux: %v", nserr)
	}

	nsArgs := []string{
		"-t", fmt.Sprintf("%d", config.PID),
		"--mount",
		"--pid",
	}
	// 容器启用网络时进入其网络命名空间（获得容器内的 10.0.0.2/veth）
	if config.NetEnabled {
		nsArgs = append(nsArgs, "--net")
	}
	// 进入容器 UTS 命名空间，让 hostname 显示容器主机名；再切换到容器 rootfs
	// 注意：nsenter 的 --root 和 --wd 选项参数为「可选」（[=<dir>），
	// 必须使用等号语法 --root=<path> / --wd=<dir>，否则 nsenter 会把
	// 路径当作要执行的程序，导致 "failed to execute" 错误（错误 127）。
	nsArgs = append(nsArgs, "--uts")
	nsArgs = append(nsArgs, "--root="+config.Rootfs)
	nsArgs = append(nsArgs, "--wd=/")
	nsArgs = append(nsArgs, "--")
	// 使用包装命令先 cd / 再 exec 登录 shell：
	// 即使 --wd=/ 在某些 nsenter 版本中不可靠，cd / 也能确保 CWD 在
	// 登录 shell 初始化时已经是有效的 /，避免 "shell-init: error
	// retrieving current directory" 报错。
	nsArgs = append(nsArgs, shell, "-c", "cd / 2>/dev/null; exec "+shell+" -l")

	cmd := exec.Command(nsenter, nsArgs...)
	cmd.Env = loginEnv(config.Rootfs, shell)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("[lxc] 容器: %s (PID: %d)\n", name, config.PID)
	fmt.Println("    输入 exit 或 Ctrl+D 退出容器")

	return cmd.Run()
}

// loginEnv 构建进入容器时的干净环境变量（避免宿主机变量污染容器）
func loginEnv(rootfs, shell string) []string {
	term := os.Getenv("TERM")
	if term == "" {
		term = "xterm-256color"
	}

	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"USER=root",
		"LOGNAME=root",
		"SHELL=" + shell,
		"PWD=/",
		"TERM=" + term,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"EDITOR=vi",
		"VISUAL=vi",
		"PAGER=less",
	}
}

func ListRunningContainers() ([]ContainerConfig, error) {
	containers, err := ListContainers()
	if err != nil {
		return nil, err
	}

	var running []ContainerConfig
	for _, c := range containers {
		if c.Running {
			if c.PID > 0 {
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", c.PID)); err == nil {
					running = append(running, c)
					continue
				}
			}
			// 进程不存在，标记为停止
			c.Running = false
			c.PID = 0
			SaveConfig(c)
		}
	}

	return running, nil
}

func GetContainerStatus(name string) (string, error) {
	if !ContainerExists(name) {
		return "", fmt.Errorf("容器 '%s' 不存在", name)
	}

	config, err := LoadConfig(name)
	if err != nil {
		return "", fmt.Errorf("加载配置失败: %w", err)
	}

	if config.Running {
		if config.PID > 0 {
			if _, err := os.Stat(fmt.Sprintf("/proc/%d", config.PID)); err == nil {
				return "运行中", nil
			}
		}
		config.Running = false
		config.PID = 0
		SaveConfig(*config)
	}

	return "已停止", nil
}

func PrintContainerList(showAll bool) error {
	containers, err := ListContainers()
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		fmt.Println("没有找到任何容器")
		fmt.Println("使用 'groot lxc [rootfs] [shell] -name [别名]' 创建容器")
		return nil
	}

	fmt.Println()
	fmt.Println("lxc 容器列表")
	fmt.Println("====================================")
	fmt.Printf("%-15s %-10s %-8s %-30s\n", "名称", "状态", "PID", "Rootfs")
	fmt.Println("------------------------------------")

	for _, c := range containers {
		status := "已停止"
		pidStr := "-"
		if c.Running {
			if c.PID > 0 {
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", c.PID)); err == nil {
					status = "运行中"
					pidStr = fmt.Sprintf("%d", c.PID)
				} else {
					c.Running = false
					c.PID = 0
					SaveConfig(c)
				}
			}
		}
		fmt.Printf("%-15s %-10s %-8s %-30s\n", c.Name, status, pidStr, c.Rootfs)
	}

	fmt.Println()
	return nil
}

func PrintContainerPs(name string) error {
	var containers []ContainerConfig

	if name != "" {
		if !ContainerExists(name) {
			return fmt.Errorf("容器 '%s' 不存在", name)
		}
		config, err := LoadConfig(name)
		if err != nil {
			return err
		}
		if !config.Running {
			fmt.Printf("容器 '%s' 未运行\n", name)
			return nil
		}
		containers = append(containers, *config)
	} else {
		var err error
		containers, err = ListRunningContainers()
		if err != nil {
			return err
		}
	}

	if len(containers) == 0 {
		fmt.Println("没有运行中的容器")
		return nil
	}

	fmt.Println()
	fmt.Println("lxc 容器进程")
	fmt.Println("====================================")

	for _, c := range containers {
		fmt.Printf("\n容器: %s (PID: %d)\n", c.Name, c.PID)
		fmt.Println("------------------------------------")

		procPath := fmt.Sprintf("/proc/%d/task", c.PID)
		if entries, err := os.ReadDir(procPath); err == nil {
			fmt.Printf("  线程数: %d\n", len(entries))
		}

		statusPath := fmt.Sprintf("/proc/%d/status", c.PID)
		if data, err := os.ReadFile(statusPath); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Name:") ||
					strings.HasPrefix(line, "State:") ||
					strings.HasPrefix(line, "Pid:") ||
					strings.HasPrefix(line, "PPid:") {
					fmt.Printf("  %s\n", strings.TrimSpace(line))
				}
			}
		}
	}

	fmt.Println()
	return nil
}

// ChildMain 容器 init 进程入口（在容器命名空间中运行，容器内表现为 PID 1）。
//
// 关键点：
//  1. 已由父进程通过 SysProcAttr.Setsid 脱离终端会话，这里不与宿主终端交互；
//  2. 挂载点必须使用「rootfs 前缀的绝对路径」（旧实现挂在了宿主路径上，
//     chroot 后容器内根本看不到 /proc /sys /dev/pts /tmp，导致环境残缺）；
//  3. 挂载完成后通过 fd 3（readyFd，父进程继承来的就绪管道）报告 "OK\n"，
//     「start」命令返回时容器已真正可用；
//  4. 挂载位于独立的 mount namespace，容器退出时由内核自动清理，不影响宿主。
func ChildMain(rootfs, shell, containerName string) error {
	if rootfs == "" {
		return fmt.Errorf("rootfs 参数不能为空")
	}

	ready := os.NewFile(readyFd, "lxc-ready")

	// 设置容器主机名（UTS 命名空间隔离，不影响宿主机）
	hostname := env.GetHostname(rootfs, "groot")
	if containerName != "" {
		hostname = containerName
	}
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		logger.Warn("设置容器主机名失败: %v", err)
	}

	// 挂载容器所需的虚拟文件系统（目标使用 rootfs 前缀路径）
	type MountEntry struct {
		source string
		rel    string
		fstype string
		flags  uintptr
	}

	entries := []MountEntry{
		{"proc", "proc", "proc", syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV},
		{"sysfs", "sys", "sysfs", syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV},
		{"/dev", "dev", "", syscall.MS_BIND | syscall.MS_REC},
		{"/dev/pts", "dev/pts", "", syscall.MS_BIND},
		{"/dev/shm", "dev/shm", "", syscall.MS_BIND},
		{"tmpfs", "tmp", "tmpfs", 0},
		{"/run", "run", "", syscall.MS_BIND},
	}

	var mounted []string
	unmountAll := func() {
		for i := len(mounted) - 1; i >= 0; i-- {
			path := filepath.Join(rootfs, mounted[i])
			_ = syscall.Unmount(path, syscall.MNT_DETACH)
		}
	}

	for _, m := range entries {
		target := filepath.Join(rootfs, m.rel)

		// 确保挂载点目录存在（bind 挂载要求目标存在）
		if err := os.MkdirAll(target, 0755); err != nil {
			logger.Warn("创建挂载点目录 %s 失败: %v", target, err)
			continue
		}

		// 若已挂载则跳过（例如 rootfs 中留有上次残留挂载时）
		if err := syscall.Mount(m.source, target, m.fstype, m.flags, ""); err != nil {
			logger.Warn("挂载 %s -> %s 失败: %v", m.source, target, err)
			continue
		}
		mounted = append(mounted, m.rel)
	}

	// 切换 root 到容器
	if err := syscall.Chroot(rootfs); err != nil {
		_, _ = ready.WriteString("ERR chroot 失败\n")
		ready.Close()
		return fmt.Errorf("chroot 失败: %w", err)
	}

	if err := os.Chdir("/"); err != nil {
		logger.Warn("chdir / 失败: %v", err)
		_ = os.Chdir("/")
	}

	// 报告就绪，父进程随即返回，容器转入后台运行
	_, _ = ready.WriteString("OK\n")
	ready.Close()

	logger.Info("容器 init 就绪，等待退出信号...")

	// 作为容器 init（PID 1）保持运行，等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	logger.Info("收到退出信号，容器正在关闭...")
	unmountAll()

	return nil
}
