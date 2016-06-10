package system

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// StartPTY создает псевдотерминал и выполняет в нем команду <cmd>.
func StartPTY(cmd *exec.Cmd) (*os.File, error) {
	// Когда процесс открывает /dev/ptmx, то он получает файл для основного псевдотерминала
	// (PTM, pseudo-terminal master), а в каталоге /dev/pts создается устройство
	// подчиненного псевдотерминала (PTS, pseudo-terminal-slave).
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	// Получаем путь к файлу, только что созданного, подчиненного псевдотерминала
	// на основе дескриптора файла основного псевдотерминала.
	slavePath, err := ptsName(pty)
	if err != nil {
		return nil, err
	}

	// Снимаем блокировку с подчиненного псевдотерминала (PTS).
	err = unlockpt(pty)
	if err != nil {
		return nil, err
	}

	// Открываем файл интерфейса подчиненного псевдотерминала (PTS) для дальнейшего использования.
	// Атрибует O_NOCTTY показывает, что открываемый файл не будет является
	// управляющим терминалом процесса.
	tty, err := os.OpenFile(slavePath, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	// Убеждаемся, что файл интерфейса будет закрыт после выхода текущей из функции.
	defer tty.Close()

	// Передаем потоки ввода и вывода перед запуском нашего процесса в интерфейс псевдотерминала.
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

// ptsName получает файл устройства подчиненного псевдотерминала (PTS)
// в соответствии с основным (PTM), на который ссылается дескриптор.
func ptsName(f *os.File) (string, error) {
	var n int32
	err := ioctl(f.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// unlockpt разблокирует устройство подчиненного псевдотерминала (PTS)
// в соответствии с основным (PTM), на который ссылается дескриптор.
func unlockpt(f *os.File) error {
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
