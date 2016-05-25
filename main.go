package main

import (
	"fmt"
	"log"
	"oar/system"
	"oar/util"
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

	/*
	*	Adding "./" before process path, if its not a system command
	 */
	if cfg.ProcessPath == cfg.BaseName {
		_, err := exec.LookPath(cfg.ProcessPath)
		if err != nil {
			cfg.ProcessPath = fmt.Sprintf("%s%s", "./", cfg.ProcessPath)
			util.Debug("Prefix \"./\" was added to %s", cfg.BaseName)
		}
	}

	var err error
	exitCode := 0
	if len(cfg.User) == 0 || cfg.User == system.GetCurrentUserName() || system.IsCurrentUserRoot() {
		exitCode, err = runProcess()
	}
	if err != nil {
		log.Printf("Process killed. Cause: %v\n", err)
	}

	util.Debug("Exit code of \"%s\": %d", cfg.ProcessPath, exitCode)
	if cfg.ExitCode {
		system.Exit(exitCode)
	}
}
