#!/usr/bin/env python3
"""litevm vmm reboot/poweroff smoke test driver.

Boots the microVM in a pty, waits for the guest shell, sends a command,
and reports what litevm did:
  - "reboot"   -> expects a SECOND kernel boot banner (VM restarted in place)
  - "poweroff" -> expects litevm to exit (VM powered off cleanly)
"""
import os
import pty
import select
import subprocess
import sys
import time

KERNEL = "/home/yiqiu/projects/groot/kernel/x86_64/vmlinux-6.18.44"
ROOTFS = "/home/yiqiu/projects/groot/rootfs/rootfs.ext4"
ARGS = "console=ttyS0,115200n8 reboot=k panic=1 nomodule init=/bin/sh"
BIN = "/home/yiqiu/projects/groot/litevm"
SOCK = "/tmp/litevm-smoke.socket"


def read_avail(fd, timeout):
    ready, _, _ = select.select([fd], [], [], timeout)
    if not ready:
        return b""
    try:
        return os.read(fd, 65536)
    except OSError:
        return b""


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "reboot"
    cmd = [BIN, "vmm", "run",
           "--kernel", KERNEL, "--rootfs", ROOTFS,
           "--kernel-args", ARGS]
    env = dict(os.environ)
    env["LITEVM_FIRECRACKER_SOCKET"] = SOCK

    master, slave = pty.openpty()
    p = subprocess.Popen(cmd, stdin=slave, stdout=slave, stderr=slave,
                         env=env, close_fds=True, start_new_session=True)
    os.close(slave)

    output = b""
    boot_banners = 0
    deadline = time.time() + 150
    sent = False
    exited_info = None

    try:
        while time.time() < deadline and p.poll() is None:
            chunk = read_avail(master, 0.2)
            if chunk:
                output += chunk
                # count Linux boot banners
                boot_banners = max(boot_banners, output.count(b"Linux version 6.18.44"))
                tail = output[-4096:]
                if not sent:
                    # wait for the init=/bin/sh shell prompt, then send the command
                    if b"# " in tail or b"#\r\n" in tail or b"/bin/sh: can't access tty" in tail:
                        time.sleep(1.0)
                        os.write(master, ("%s -f\n" % mode).encode())
                        sent = True
                        print("[test] sent: %s -f" % mode, flush=True)
        if p.poll() is None:
            # give it a moment more after sending
            end = time.time() + 40
            while time.time() < end:
                chunk = read_avail(master, 0.2)
                if chunk:
                    output += chunk
                    boot_banners = max(boot_banners, output.count(b"Linux version 6.18.44"))
                if p.poll() is not None:
                    break
        exited_info = p.poll()
    finally:
        if p.poll() is None:
            p.kill()
        os.close(master)
        try:
            os.waitpid(p.pid, 0)
        except (ChildProcessError, OSError):
            pass

    print("=" * 60, flush=True)
    print("[test] mode=%s litevm_exit=%s boot_banners=%d output_bytes=%d" %
          (mode, exited_info, boot_banners, len(output)), flush=True)
    # print the final 1500 bytes for debugging
    print(output[-1500:].decode("utf-8", "replace"), flush=True)

    if mode == "reboot":
        # litevm 应该仍然在运行（reboot 重新拉起了 microVM），并且出现了第二次启动横幅
        ok = p.poll() is None and boot_banners >= 2
    else:
        ok = exited_info == 0 and boot_banners == 1
    print("[test] RESULT=%s" % ("PASS" if ok else "FAIL"), flush=True)
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()