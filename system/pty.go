package system

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

func StartPTY(cmd *exec.Cmd) (*os.File, error) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	sname, err := ptsName(pty)
	if err != nil {
		return nil, err
	}

	err = unlock(pty)
	if err != nil {
		return nil, err
	}

	tty, err := os.OpenFile(sname, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	defer tty.Close()

	cmd.Stdout = tty
	cmd.Stdin = tty
	cmd.Stderr = tty
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}
	} else {
		cmd.SysProcAttr.Setctty = true
		cmd.SysProcAttr.Setsid = true
	}
	err = cmd.Start()
	if err != nil {
		pty.Close()
		return nil, err
	}
	return pty, err
}

func ptsName(f *os.File) (string, error) {
	var n int32
	err := ioctl(f.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

func unlock(f *os.File) error {
	var u int32
	return ioctl(f.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&u)))
}

func ioctl(fd, cmd, ptr uintptr) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, ptr)
	if e != 0 {
		return e
	}
	return nil
}
