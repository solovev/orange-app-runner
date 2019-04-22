package main

import (
	"os"
	"path/filepath"

	"github.com/solovev/orange-app-runner/util"
)

type config struct {
	Debug        bool   `long:"debug" description:"Enable debug output"`
	RootFS       string `long:"rootfs" description:"Path to the root filesystem to use"`
	NetSetGoPath string `long:"nsgpath" description:"Path to the netsetgo binary"`
}

func (cfg *config) checkRootFS() error {
	if len(cfg.RootFS) == 0 {
		return nil
	}

	path, err := filepath.Abs(cfg.RootFS)
	if err != nil {
		return err
	}

	cfg.RootFS = path

	_, err = os.Stat(cfg.RootFS)
	if err != nil {
		return err
	}
	return nil
}

func (cfg *config) checkNetSetGoPath() error {
	path, err := util.CheckIsBinaryExists(cfg.NetSetGoPath)
	if err != nil {
		return err
	}

	cfg.NetSetGoPath = path

	return nil
}
