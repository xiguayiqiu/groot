package vmm

import (
	"strings"
	"testing"
)

// 测试用 io.Writer：丢弃所有写入数据。
type discardWriter struct{}

func (w *discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// 测试用 io.Writer：记录所有写入的数据。
type recordingWriter struct {
	data []byte
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// nextEvent 非阻塞地检查 event 通道是否已收到事件，返回事件码（0 表示无事件）。
func nextEvent(ch chan int) int {
	select {
	case ev := <-ch:
		return ev
	default:
		return 0
	}
}

// TestSerialConsoleDetectsKernelHalt 验证 serialConsole 能从串口输出中识别出
// guest 关机时内核打印的 "reboot: System halted" 标记（systemd/openrc/runit
// 关机时的最后一行，与 init 系统无关）。
func TestSerialConsoleDetectsKernelHalt(t *testing.T) {
	dst := &recordingWriter{}
	ch := make(chan int, 1)
	mon := &serialConsole{dst: dst, event: ch}

	// 模拟 systemd 关机时串口的最后几行
	for _, line := range []string{
		"[ 66.162862] systemd-shutdown[1]: All filesystems unmounted.\n",
		"[ 66.166707] reboot: System halted\n",
	} {
		if _, err := mon.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}

	if ev := nextEvent(ch); ev != evPowerOff {
		t.Errorf("关机标记 \"System halted\" 应上报 evPowerOff，实际为 %v", ev)
	}

	// tee 部分：dst 应该收到全部原始输出
	if !strings.Contains(string(dst.data), "reboot: System halted") {
		t.Errorf("serialConsole 应把串口输出原样转发到 dst，实际为 %s", string(dst.data))
	}
}

// TestSerialConsoleDetectsKernelPowerDown 验证 ACPI poweroff 标记
// "reboot: Power down" 也能被识别为关机。
func TestSerialConsoleDetectsKernelPowerDown(t *testing.T) {
	mon := &serialConsole{dst: &discardWriter{}, event: make(chan int, 1)}
	if _, err := mon.Write([]byte("[ 10.000000] reboot: Power down\n")); err != nil {
		t.Fatal(err)
	}
	if ev := nextEvent(mon.event); ev != evPowerOff {
		t.Errorf("关机标记 \"Power down\" 应上报 evPowerOff，实际为 %v", ev)
	}
}

// TestSerialConsoleDetectsSystemdPoweringOff 验证 systemd 关机时打印的
// "Powering off" 标记也能被识别为关机。
func TestSerialConsoleDetectsSystemdPoweringOff(t *testing.T) {
	mon := &serialConsole{dst: &discardWriter{}, event: make(chan int, 1)}
	if _, err := mon.Write([]byte("systemd-shutdown[1]: Powering off.\n")); err != nil {
		t.Fatal(err)
	}
	if ev := nextEvent(mon.event); ev != evPowerOff {
		t.Errorf("systemd 关机标记 \"Powering off\" 应上报 evPowerOff，实际为 %v", ev)
	}
}

// TestSerialConsoleDetectsReboot 验证 guest 执行 reboot 时内核打印的
// "reboot: Restarting system" 标记被识别为重启（而不是关机）。
func TestSerialConsoleDetectsReboot(t *testing.T) {
	mon := &serialConsole{dst: &discardWriter{}, event: make(chan int, 1)}
	for _, line := range []string{
		"[ 34.684242] reboot: Restarting system\n",
		"[ 34.684460] reboot: machine restart\n",
	} {
		if _, err := mon.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if ev := nextEvent(mon.event); ev != evReboot {
		t.Errorf("重启标记应上报 evReboot，实际为 %v", ev)
	}
	if !mon.rebooted {
		t.Errorf("命中重启标记后 rebooted 标志应为 true")
	}
	if mon.poweredOff {
		t.Errorf("重启标记不应设置 poweredOff 标志")
	}
}

// TestSerialConsoleIgnoresNormalBoot 验证正常启动/运行期的输出不会误触发。
func TestSerialConsoleIgnoresNormalBoot(t *testing.T) {
	mon := &serialConsole{dst: &discardWriter{}, event: make(chan int, 1)}

	lines := []string{
		"Welcome to litevm!\n",
		"[ OK ] Started Systemd Logger.\n",
		"[ OK ] Reached target Multi-User System.\n",
		"login: \n",
		"# poweroff is a dangerous command, type poweroff to confirm\n",
		"[   42.000000] some driver: acpi power is managed by bmc\n",
		"Press Ctrl+D to continue\n",
	}
	for _, line := range lines {
		if _, err := mon.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}

	if ev := nextEvent(mon.event); ev != 0 {
		t.Errorf("正常启动/运行期的输出不应触发事件，实际为 %v", ev)
	}
	if mon.rebooted || mon.poweredOff {
		t.Errorf("正常启动/运行期的输出不应设置任何标记")
	}
}

// TestSerialConsoleMarkerSplitAcrossChunks 验证标记被拆成多个写入块时也能识别
// （内核打印分批到达，先写 "System hal" 再写 "ted\n" 的情况）。
func TestSerialConsoleMarkerSplitAcrossChunks(t *testing.T) {
	mon := &serialConsole{dst: &discardWriter{}, event: make(chan int, 1)}

	if _, err := mon.Write([]byte("[60.000000] reb")); err != nil {
		t.Fatal(err)
	}
	if _, err := mon.Write([]byte("oot: System hal")); err != nil {
		t.Fatal(err)
	}
	if _, err := mon.Write([]byte("ted\n")); err != nil {
		t.Fatal(err)
	}

	if ev := nextEvent(mon.event); ev != evPowerOff {
		t.Errorf("跨块到达的关机标记应上报 evPowerOff，实际为 %v", ev)
	}
}

// TestSerialConsoleTriggersOnce 验证命中一次标记后只发送一次事件，
// 后续输出不会重复触发。
func TestSerialConsoleTriggersOnce(t *testing.T) {
	mon := &serialConsole{dst: &discardWriter{}, event: make(chan int, 1)}

	if _, err := mon.Write([]byte("reboot: System halted\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := mon.Write([]byte("reboot: Power down\n")); err != nil {
		t.Fatal(err)
	}

	if ev := nextEvent(mon.event); ev != evPowerOff {
		t.Errorf("第一次命中关机标记应上报 evPowerOff，实际为 %v", ev)
	}
	if ev := nextEvent(mon.event); ev != 0 {
		t.Errorf("命中后不应再次发送事件，实际为 %v", ev)
	}
}

// TestSerialConsoleResetRearms 验证 Reset 之后监视可以被重新武装，
// 供 Firecracker 原地重启（进程未退出）的场景使用。
func TestSerialConsoleResetRearms(t *testing.T) {
	mon := &serialConsole{dst: &discardWriter{}, event: make(chan int, 1)}

	if _, err := mon.Write([]byte("reboot: Restarting system\n")); err != nil {
		t.Fatal(err)
	}
	if ev := nextEvent(mon.event); ev != evReboot {
		t.Errorf("第一次重启标记应上报 evReboot，实际为 %v", ev)
	}

	mon.Reset()

	if _, err := mon.Write([]byte("reboot: System halted\n")); err != nil {
		t.Fatal(err)
	}
	if ev := nextEvent(mon.event); ev != evPowerOff {
		t.Errorf("Reset 后应重新武装监视，实际事件为 %v", ev)
	}
}

// TestSerialConsoleForwardsLargeWrites 验证大块写入（超过滚动窗口大小）时
// 依然能完整转发到 dst 并返回正确的字节数。
func TestSerialConsoleForwardsLargeWrites(t *testing.T) {
	dst := &recordingWriter{}
	mon := &serialConsole{dst: dst, event: make(chan int, 1)}

	big := strings.Repeat("normal output line\n", 1024) // 约 20KB，超出 4096 窗口
	n, err := mon.Write([]byte(big))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(big) {
		t.Errorf("Write 应返回 %d 个字节，实际返回 %d", len(big), n)
	}
	if string(dst.data) != big {
		t.Errorf("大块写入应被完整转发到 dst")
	}
	if ev := nextEvent(mon.event); ev != 0 {
		t.Errorf("普通数据不应触发事件，实际为 %v", ev)
	}
}