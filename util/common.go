package util

import (
	"crypto/sha1"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

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
