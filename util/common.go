package util

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func GetProcessBaseName(path string) string {
	return filepath.Base(strings.Replace(path, "\\", "/", -1))
}

func Debug(msg string, a ...interface{}) {
	if debug_enabled && !quiet {
		log.Println("[DEBUG]", fmt.Sprintf(msg, a...))
	}
}

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
