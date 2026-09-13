//go:build amd64

package proot

import (
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"

	"groot/internal/logger"
)

type SyscallHandler func(tracee *Tracee) (int, error)

type Tracee struct {
	Pid         int
	Translator  *PathTranslator
	Env         []string
	CustomShell string
	Exited      atomic.Bool
}

type PTraceManager struct {
	Rootfs      string
	CustomShell string
}

func NewPTraceManager(rootfs, customShell string) *PTraceManager {
	return &PTraceManager{
		Rootfs:      rootfs,
		CustomShell: customShell,
	}
}

func (pm *PTraceManager) Run() error {
	translator := NewPathTranslator(pm.Rootfs)

	shell := "/bin/sh"
	if pm.CustomShell != "" {
		shell = pm.CustomShell
	}

	cmd := exec.Command(shell)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Ptrace: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start tracee: %w", err)
	}

	tracee := &Tracee{
		Pid:         cmd.Process.Pid,
		Translator:  translator,
		CustomShell: shell,
	}

	logger.Info("Tracing process: %d", tracee.Pid)

	go func() {
		cmd.Wait()
		tracee.Exited.Store(true)
	}()

	return pm.trace(tracee)
}

func (pm *PTraceManager) trace(tracee *Tracee) error {
	for {
		if tracee.Exited.Load() {
			break
		}

		var status syscall.WaitStatus
		pid, err := syscall.Wait4(tracee.Pid, &status, 0, nil)
		if err != nil {
			return fmt.Errorf("wait4 failed: %w", err)
		}

		if pid != tracee.Pid {
			continue
		}

		if status.Exited() {
			logger.Debug("Tracee exited with status %d", status.ExitStatus())
			break
		}

		if status.Signaled() {
			logger.Debug("Tracee signaled: %v", status.Signal())
			break
		}

		if status.Stopped() {
			sig := status.StopSignal()
			logger.Debug("Tracee stopped: %v", sig)

			// 确定用于 PtraceCont 的信号
			// SIGTRAP 处理后不传递额外信号，其他信号正常传递
			continueSignal := int(sig)
			if sig == syscall.SIGTRAP {
				if err := pm.handleSyscall(tracee); err != nil {
					logger.Warn("Syscall handling failed: %v", err)
				}
				continueSignal = 0 // 不传递额外信号
			}

			if err := syscall.PtraceCont(tracee.Pid, continueSignal); err != nil {
				return fmt.Errorf("ptrace cont failed: %w", err)
			}
		}
	}

	return nil
}

func (pm *PTraceManager) handleSyscall(tracee *Tracee) error {
	var regs syscall.PtraceRegs
	if err := syscall.PtraceGetRegs(tracee.Pid, &regs); err != nil {
		return fmt.Errorf("ptrace get regs failed: %w", err)
	}

	syscallNum := getSyscallNum(&regs)
	logger.Debug("Syscall: %d (args: %v, %v, %v, %v, %v, %v)",
		syscallNum,
		getSyscallArg(&regs, 0),
		getSyscallArg(&regs, 1),
		getSyscallArg(&regs, 2),
		getSyscallArg(&regs, 3),
		getSyscallArg(&regs, 4),
		getSyscallArg(&regs, 5))

	return nil
}

func getSyscallNum(regs *syscall.PtraceRegs) uintptr {
	return uintptr(regs.Orig_rax)
}

func getSyscallArg(regs *syscall.PtraceRegs, idx int) uintptr {
	switch idx {
	case 0:
		return uintptr(regs.Rdi)
	case 1:
		return uintptr(regs.Rsi)
	case 2:
		return uintptr(regs.Rdx)
	case 3:
		return uintptr(regs.R10)
	case 4:
		return uintptr(regs.R8)
	case 5:
		return uintptr(regs.R9)
	default:
		return 0
	}
}

func readString(pid int, addr uintptr, maxLen int) string {
	if maxLen <= 0 || maxLen > 4096 {
		return ""
	}

	buf := make([]byte, maxLen)
	n, err := syscall.PtracePeekData(pid, addr, buf)
	if err != nil {
		return ""
	}

	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func writeString(pid int, addr uintptr, s string) error {
	buf := []byte(s)
	buf = append(buf, 0)
	_, err := syscall.PtracePokeData(pid, addr, buf)
	return err
}
