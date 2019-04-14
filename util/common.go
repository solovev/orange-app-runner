package util

import (
	"crypto/sha1"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/solovev/orange-app-runner/system"
)

// CreateHomeDirectory создает директорию по указанному пути, если ее не существует
func CreateHomeDirectory(path string) (string, error) {
	length := len(path)
	if length == 0 || (path[0] != '~' && path[0] != '/') {
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}

		// Если путь не указан, выбираем текущую директорию в качестве "домашней"
		// Если путь относительный, конкатенируем его с текущий директорией.
		if length == 0 {
			path = dir
		} else {
			path = filepath.Join(dir, path)
		}
	}
	// Создаем директорию по указанному пути.
	err := os.MkdirAll(path, 0777)
	if err != nil {
		return "", err
	}
	return path, nil
}

// RestartItself перезапускает OAR
// (./oar [<options>] <program> [<parameters>]) в новом терминале без параметра "-w".
// Пример:
//	Оригинальная команда: "./oar -w -x -1 ./command"
//	Перезапуск в новом терминале "./oar -x -1 ./command"
func RestartItself(from string) {
	if from == "gnome-terminal" {
		terminalArgs := []string{"-x"}
		for _, arg := range os.Args {
			if arg != "-w" {
				terminalArgs = append(terminalArgs, arg)
			}
		}
		cmd := exec.Command(from, terminalArgs...)
		err := cmd.Run()
		if err != nil {
			log.Printf("Unable to open new \"%s\" terminal: %v\n", from, err)
			system.Exit(1)
		}
		log.Println("Redirected to new terminal.")
	}
	system.Exit(0)
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

// CreateFile пересоздает файл <name> в директории <dir>
func CreateFile(dir, name string) (*os.File, error) {
	path := path.Join(dir, name)
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
func OpenFile(dir, name string) (*os.File, error) {
	path := path.Join(dir, name)
	f, err := os.Open(path)
	if err != nil {
		f, err = os.Create(path)
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
