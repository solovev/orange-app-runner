package main

import (
	"io/ioutil"
	"log"
	"orange-app-runner/system"
	"orange-app-runner/util"
	"os"
	"os/exec"
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
		system.Exit(0)
	}
}
