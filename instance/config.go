package instance

import (
	"os"
	"path/filepath"

	"github.com/solovev/orange-app-runner/util"
)

type Config struct {
	Debug bool `long:"debug" description:"Enable debug output"`

	RootFS       string `long:"rootfs" description:"Set path to the root filesystem to use"`
	NetSetGoPath string `long:"nsgpath" description:"Set path to the netsetgo binary"`

	Env               []string `long:"env" description:"Add environment variable (by default, system's environment variables is completely ignored)"`
	Affinity          []int    `short:"a" long:"affinity" description:"Add an index of CPU to the list of cores that the process can use. If not specified, child process will be use all available cores. Specify \"-1\" to use single most unload CPU core"`
	WorkingDir        string   `short:"d" long:"dir" description:"Set path to working directory for process"`
	PropagateExitCode bool     `short:"x" long:"exit" description:"Enable exit code propagation (return exit code from tracee application)"`

	AllowCreateProcesses bool `long:"allow-create-processes" description:"Allow to spawn child processes by tracee process"`
	AllowMultiThreading  bool `long:"allow-multithreading" description:"Allow tracee process to clone himself for new thread creation"`
	MaxPtraceIterations  int  `long:"max-ptrace-iterations" description:"Set limit of number of ptrace loop iterations (debug purposes)" optional:"yes" optional-value:"-1" default:"-1"`
}

func (cfg *Config) CheckRootFS() error {
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

func (cfg *Config) CheckNetSetGoPath() error {
	path, err := util.CheckIsBinaryExists(cfg.NetSetGoPath)
	if err != nil {
		return err
	}

	cfg.NetSetGoPath = path

	return nil
}
