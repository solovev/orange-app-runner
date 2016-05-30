package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"orange-app-runner/system"
	"orange-app-runner/util"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var (
	cfg *util.Config
)

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cfg = util.NewConfig()

	if cfg.Quiet {
		log.SetOutput(ioutil.Discard)
	}

	if cfg.DisplayWindow {
		restartItself("gnome-terminal")
	}

	if len(cfg.HomeDirectory) > 0 {
		path := cfg.HomeDirectory
		if path[0] != '~' && path[0] != '/' {
			dir, err := os.Getwd()
			if err != nil {
				fmt.Printf("Unable to get working directory: %v.\n", err)
				system.Exit(1)
			}
			path = filepath.Join(dir, cfg.HomeDirectory)
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			err := os.MkdirAll(path, 0777)
			if err != nil {
				fmt.Printf("Error creating home directory \"%s\": %v.\n", path, err)
				system.Exit(1)
			}
			util.Debug("Home directory \"%s\" was just created", path)
		} else {
			util.Debug("Home directory \"%s\" is exists", path)
		}
	}

	var err error
	exitCode := 0

	defaultRunning := cfg.User == system.GetCurrentUserName() || system.IsCurrentUserRoot()
	if defaultRunning {
		exitCode, err = runProcess()
	} else {
		exitCode, err = runProcessViaPTY()
	}

	if err != nil {
		log.Printf("Process killed. Cause: %v\n", err)
	}

	if exitCode != -1 {
		if defaultRunning {
			util.Debug("Exit code of \"%s\": %d", cfg.ProcessPath, exitCode)
		}
		if cfg.ExitCode {
			system.Exit(exitCode)
		}
	}
}

func restartItself(from string) {
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
