package util

import (
	"crypto/sha1"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ы проверяет, есть ли директория по указанному пути, если ее не существует - создает
func GetProcessHomeDirectory(path string) (string, error) {
	length := len(path)
	if length == 0 {
		return "", nil
	}

	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// Создаем директорию по указанному пути.
	err = os.MkdirAll(path, 0777)
	if err != nil {
		return "", err
	}
	return path, nil
}

func ExcludeFlag(flag string) []string {
	args := []string{}
	for _, arg := range os.Args {
		if arg != "-"+flag {
			args = append(args, arg)
		}
	}
	return args
}

// RestartItself перезапускает OAR
// (./oar [<options>] <program> [<parameters>]) в новом терминале без параметра "-w".
// Пример:
//	Оригинальная команда: "./oar -w -x -1 ./command"
//	Перезапуск в новом терминале "./oar -x -1 ./command"
func RestartItself(from string) error {
	if from == "gnome-terminal" {
		terminalArgs := []string{"-x"}

		args := ExcludeFlag("w")
		terminalArgs = append(terminalArgs, args...)

		cmd := exec.Command(from, terminalArgs...)
		err := cmd.Run()
		if err != nil {
			return err
		}
		log.Println("Redirected to new terminal.")
	}
	return nil
}

// GetProcessBaseName возвращает только имя процесса, указанное в полном пути <path>.
// Пример:
//	path: "/home/folder/command"
//	return:	"command"
func GetProcessBaseName(path string) string {
	return filepath.Base(strings.Replace(path, "\\", "/", -1))
}

// Debug выводит на экран отладочную информацию.
func Debug(msg string, a ...interface{}) {
	if debug && !quiet {
		log.Println("[DEBUG]", fmt.Sprintf(msg, a...))
	}
}

func IsFileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// CreateFile пересоздает файл <name> в директории <dir>
func CreateFile(name string) (*os.File, error) {
	path, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); err == nil {
		err := os.Remove(path)
		if err != nil {
			return nil, err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// OpenFile открывает файл <name> в директории <dir>, eсли такой не найден, создает новый.
// Если параметр <name> - абсолютный путь, то <dir> игнорируется
func OpenFile(name string) (*os.File, error) {
	pathToFile, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(pathToFile)
	if err != nil {
		f, err = os.Create(pathToFile)
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}

// GetHash возвращает хэшированную (SHA1) строку <value>
func GetHash(value string) string {
	hex := sha1.New()
	hex.Write([]byte(os.Args[0]))
	hex.Write([]byte(value))
	return fmt.Sprintf("%x", hex.Sum(nil))
}
