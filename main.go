package main

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/reexec"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
	"github.com/solovev/orange-app-runner/instance"
	"github.com/solovev/orange-app-runner/system"
)

var (
	cfg instance.Config

	processPath string
	processArgs []string
)

const (
	defaultProcess        = "/bin/sh"
	wrapper        string = "ejudge_tracer"
)

func init() {
	args, err := flags.Parse(&cfg)

	if err != nil {
		log.Fatalln(err)
	}

	if len(args) > 0 {
		processPath = args[0]
		if len(args) > 1 {
			processArgs = args[1:]
		}
	} else {
		processPath = defaultProcess
		log.Warnf("Path to target binary is not specified, changing to %s...\n", processPath)
	}

	log.SetFormatter(&log.TextFormatter{})

	if cfg.Debug {
		log.SetLevel(log.DebugLevel)
	}

	reexec.Register(wrapper, startTracer)
	if reexec.Init() {
		os.Exit(0)
	}
}

func startTracer() {
	if len(cfg.RootFS) > 0 {
		path, err := filepath.Abs(cfg.RootFS)
		if err != nil {
			log.Fatalln(err)
		}

		log.Infof("Root filesystem path: \"%s\"\n", path)

		if err := system.MountProc(path); err != nil {
			log.WithFields(log.Fields{
				"path":  path,
				"error": err,
			}).Fatal("Failed to mount /proc")
		}

		if err := system.PivotRoot(path); err != nil {
			log.WithFields(log.Fields{
				"path":  path,
				"error": err,
			}).Fatal("Error running pivot_root")
		}
	}

	log.Infof("Setting hostname: \"%s\"\n", wrapper)
	if err := syscall.Sethostname([]byte(wrapper)); err != nil {
		log.WithFields(log.Fields{
			"hostname": wrapper,
			"error":    err,
		}).Fatal("Error running hostname")
	}

	if len(cfg.NetSetGoPath) > 0 {
		wait := 3 * time.Second
		log.Infof("Waiting for network for %v...\n", wait)
		if err := system.WaitForNetwork(wait); err != nil {
			log.Fatalf("Error waiting for network: %v\n", err)
		}
	}

	exitCode, err := instance.Run(processPath, processArgs, &cfg)
	if err != nil {
		log.Fatalf("Error running tracee process: %v\n", err)
	}

	log.Infof("Tracee is terminated. Exit code: %d\n", exitCode)

	if cfg.PropagateExitCode {
		os.Exit(exitCode)
	}
}

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if len(cfg.RootFS) > 0 {
		err := cfg.CheckRootFS()
		if err != nil {
			log.WithFields(log.Fields{
				"path":  cfg.RootFS,
				"error": err,
			}).Fatal("Failed to locate rootfs directory")
		}
	} else {
		log.Warn("Path to root filesystem is not specified, mount namespace cloning is disabled")
	}

	if len(cfg.NetSetGoPath) > 0 {
		err := cfg.CheckNetSetGoPath()
		if err != nil {
			log.WithFields(log.Fields{
				"path":  cfg.NetSetGoPath,
				"error": err,
			}).Fatal("Failed to locate \"netsetgo\" binary file")
		}
	} else {
		log.Warn("Path to \"netsetgo\" binary file is not specified, spawned process will not have any network connectivity")
	}

	args := []string{wrapper}
	args = append(args, os.Args[1:]...)

	uid := os.Getuid()
	gid := os.Getgid()

	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		log.Warn(err)
	}
	log.Infof("Starting tracer for \"%s\" (As: \"%s\", UID: %d, GID: %d): %v...\n", processPath, u.Username, uid, gid, args)

	cmd := reexec.Command(args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	var cf uintptr
	cf = syscall.CLONE_NEWUTS |
		syscall.CLONE_NEWIPC |
		syscall.CLONE_NEWPID |
		syscall.CLONE_NEWNET |
		syscall.CLONE_NEWUSER

	if len(cfg.RootFS) > 0 {
		cf |= syscall.CLONE_NEWNS
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cf,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      uid,
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      gid,
				Size:        1,
			},
		},
	}

	if err := cmd.Start(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Error starting the reexec.Command")
	}

	if len(cfg.NetSetGoPath) > 0 {
		args = []string{"-pid", strconv.Itoa(cmd.Process.Pid)}
		log.Infof("Starting \"netsetgo\" (%s), args: %v\n", cfg.NetSetGoPath, args)

		nsgcmd := exec.Command(cfg.NetSetGoPath, args...)
		if err := nsgcmd.Run(); err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Fatal("Error starting \"netsetgo\" binary")
		}
	}

	if err := cmd.Wait(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Error waiting for the reexec.Command")
	}
}
