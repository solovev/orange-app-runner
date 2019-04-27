package instance

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/solovev/orange-app-runner/system"
	"golang.org/x/sys/unix"
)

var (
	ErrRealTimeLimitExceeded = defineTracerError(2, errors.New("Real time limit was exceeded"))
	ErrMemoryLimitExceeded   = defineTracerError(3, errors.New("Memory (RSS) limit was exceeded"))
)

type traceeInstance struct {
	process *os.Process
	pgid    int

	stopc chan bool
	errc  chan error
}

func (t *traceeInstance) kill(reason *TracerError) {
	if err := t.process.Kill(); err != nil {
		log.Debugf("[Tracee.kill] Killing error: %v\n", err)
	}

	if err := syscall.Kill(-t.pgid, syscall.SIGKILL); err != nil {
		log.Debugf("[Tracee.kill] Killing group error: %v\n", err)
	}

	select {
	case t.errc <- reason:
	default:
		log.Debugf("[Tracee.kill] Reason was not sended to channel: %v\n", reason)
	}
}

func Run(processPath string, processArgs []string, cfg *Config) (int, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tracee := &traceeInstance{
		stopc: make(chan bool),
		errc:  make(chan error, 1),
	}
	defer close(tracee.stopc)

	processName := filepath.Base(processPath)
	log.Infof("Starting tracee (%s %v)...\n", processName, processArgs)

	processArgs = append([]string{processName}, processArgs...)

	ptrace := !cfg.AllowCreateProcesses || !cfg.AllowMultiThreading
	log.Debugf("Ptrace - %t [Allow create processes - %t] [Allow multithreading - %t]", ptrace, cfg.AllowCreateProcesses, cfg.AllowMultiThreading)

	inReader, inWriter, err := os.Pipe()
	if err != nil {
		return -1, err
	}
	defer inWriter.Close()

	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return -1, err
	}
	defer outWriter.Close()

	errReader, errWriter, err := os.Pipe()
	if err != nil {
		return -1, err
	}
	defer errWriter.Close()

	go io.Copy(inWriter, os.Stdin)
	go io.Copy(os.Stdout, outReader)
	go io.Copy(os.Stderr, errReader)

	files := []*os.File{inReader, outWriter, errWriter}

	process, err := os.StartProcess(processPath, processArgs, &os.ProcAttr{
		Files: files,
		Dir:   cfg.WorkingDir,
		Env:   append([]string{}, cfg.Env...),
		Sys: &syscall.SysProcAttr{
			Ptrace:    true,
			Setpgid:   true,
			Pdeathsig: syscall.SIGKILL,
		},
	})
	if err != nil {
		return -1, err
	}

	tracee.process = process

	for _, fd := range files {
		if err = fd.Close(); err != nil {
			return -1, err
		}
	}

	pid := process.Pid
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return -1, err
	}
	tracee.pgid = pgid
	log.Debugf("Tracee pgid is: %d\n", tracee.pgid)

	if err = setAffinity(pid, cfg); err != nil {
		return -1, err
	}

	if cfg.RealTimeLimit > 0 {
		go startKillingTimer(tracee, cfg)
	}

	// go startCheckingCPUTime(tracee, cfg)

	_, status, err := wait(pid)
	if err != nil {
		return -1, err
	}

	switch {
	case status.Exited():
		return status.ExitStatus(), nil
	case status.Stopped():
		signal := status.StopSignal()
		if !ptrace {
			return -1, fmt.Errorf("Wait status of tracee is \"Stopped\" (%s), but ptrace is disabled", signal.String())
		}
		if signal != syscall.SIGTRAP {
			return -1, err
		}
		log.Debugf("[PID %d] Status is \"Stopped\" (SIGTRAP: %s)", pid, signal.String())
	default:
		return -1, err
	}

	exitCode, tErr := trace(tracee, cfg)

	select {
	case tErr = <-tracee.errc:
		log.Debugf("Tracee was terminated due to exceeding one of the established limits: \"%v\"\n", tErr)
	default:
	}

	return exitCode, tErr
}

func startCheckingCPUTime(tracee *traceeInstance, cfg *Config) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	log.Debugln("Goroutine \"startCheckingCPUTime\" started")
	defer log.Debugln("Goroutine \"startCheckingCPUTime\" terminated")

	ticker := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-tracee.stopc:
			return
		case <-ticker.C:
			var usage syscall.Rusage
			if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage); err != nil {
				tErr := createTracerError("syscall.Getrusage", err)
				tracee.kill(tErr)
				return
			}
			log.Debugf("> Rusage: [Utime: %d] [Stime: %d] [Maxrss: %d]\n", usage.Utime, usage.Stime, usage.Maxrss)
			// usage.
		}
	}
}

func startKillingTimer(tracee *traceeInstance, cfg *Config) {
	if cfg.RealTimeLimit < 0 {
		return
	}

	log.Debugln("Goroutine \"startKillingTimer\" started")
	defer log.Debugln("Goroutine \"startKillingTimer\" terminated")

	value := time.Duration(cfg.RealTimeLimit)
	log.Debugf("Tracee will be terminated after %dms\n", value)

	for {
		select {
		case <-tracee.stopc:
			return
		case <-time.After(value * time.Millisecond):
			tracee.kill(ErrRealTimeLimitExceeded)
			return
		}
	}
}

func trace(tracee *traceeInstance, cfg *Config) (int, error) {
	process := tracee.process

	var ws syscall.WaitStatus

	level := 0
	iterations := 0

	traceePid := process.Pid
	previousPid := 0
	currentPid := traceePid

	options := unix.PTRACE_O_EXITKILL
	options |= syscall.PTRACE_O_TRACECLONE
	options |= syscall.PTRACE_O_TRACEFORK
	options |= syscall.PTRACE_O_TRACEVFORK
	options |= syscall.PTRACE_O_TRACEVFORKDONE
	options |= syscall.PTRACE_O_TRACEEXEC
	options |= syscall.PTRACE_O_TRACEEXIT

	formatError := func(culprit string, err error) (int, error) {
		currentCommand := processCommandName(currentPid, traceePid)
		previousCommand := processCommandName(previousPid, traceePid)

		return -1, fmt.Errorf("%d | Error at level %d [%s] for [PID: %d (%s), Prev. PID: %d (%s)]: %v", iterations, level, culprit, currentPid, currentCommand, previousPid, previousCommand, err)
	}

	debugStatus := func(pid int, status syscall.WaitStatus) {
		if !cfg.Debug {
			return
		}

		name := processCommandName(pid, traceePid)

		prefix := "[%d | Process \"%s\" (%d) at level %d] "
		switch {
		case status.Exited():
			log.Debugf(prefix+"Status is \"Exited\", code: %d\n", iterations, name, pid, level, status.ExitStatus())
		case status.Signaled():
			log.Debugf(prefix+"Status is \"Signaled\" - %s\n", iterations, name, pid, level, status.Signal().String())
		case status.Stopped():
			log.Debugf(prefix+"Status is \"Stopped\" - %s\n", iterations, name, pid, level, status.StopSignal().String())
		case status.Continued():
			log.Debugf(prefix+"Status is \"Continued\"\n", iterations, name, pid, level)
		default:
			log.Debugf(prefix+"Unknown status\n", iterations, name, pid, level)
		}
	}

	debugMessage := func(msg string, a ...interface{}) {
		if !cfg.Debug {
			return
		}

		prefix := fmt.Sprintf("[Iteration: %d | Level: %d]", iterations, level)
		msg = fmt.Sprintf(msg, a...)
		log.Debugln(prefix + " " + msg)
	}

	err := syscall.PtraceSetOptions(currentPid, options)
	if err != nil {
		return formatError("syscall.PtraceSetOptions (before loop)", err)
	}

	err = syscall.PtraceSyscall(currentPid, 0)
	if err != nil {
		return formatError("syscall.PtraceCont (before loop)", err)
	}

	maxIterations := cfg.MaxPtraceIterations
	log.Debugf("Starting ptrace loop (Max iterations: %d)...\n", maxIterations)
	for {
		iterations++
		debugMessage("-=-=-=- Ptrace iteration [PID: %d, Prev. PID: %d]", currentPid, previousPid)

		if maxIterations >= 0 && iterations > maxIterations {
			err = fmt.Errorf("Ptrace iterations limit (%d/%d) exceeded", iterations, maxIterations)
			return -1, err
		}

		var usage syscall.Rusage
		waitPid, err := syscall.Wait4(-1, &ws, syscall.WALL, &usage)
		if err != nil {
			return formatError("syscall.Wait4", err)
		}

		memLim := cfg.MemoryLimit
		if memLim >= 0 {
			size := usage.Maxrss

			percent := int(float64(size) / float64(memLim) * 100.0)
			debugMessage("Memory consumption: %d%% (%d/%d)\n", percent, size, memLim)

			if size >= memLim {
				return -1, ErrMemoryLimitExceeded
			}
		}

		if waitPid <= 0 {
			err := fmt.Errorf("Waited pid is %d", waitPid)
			return formatError("syscall.Wait4 pid check", err)
		}

		if currentPid != waitPid {
			previousPid = currentPid
			currentPid = waitPid
			level++

			debugMessage("Current process is changed, now: %d (Previous: %d)", currentPid, previousPid)
		}

		debugStatus(currentPid, ws)

		exited := ws.Exited()
		signaled := ws.Signaled()
		if exited || signaled {
			if currentPid == traceePid {
				debugMessage("Before loop exit, tracee status [exited: %t] [signaled: %t]", exited, signaled)
				if exited {
					return ws.ExitStatus(), nil
				}
				err = fmt.Errorf("Signal: %s", ws.Signal().String())
				return formatError("Tracee signaled", err)
			}
			debugMessage("Child process %d exited", currentPid)
			continue
		}

		if ws.Stopped() {
			switch ws.StopSignal() {
			case syscall.SIGXCPU:
				err = errors.New("CPU time limit exceeded")
				return formatError("syscall.SIGXCPU", err)
			case syscall.SIGSEGV:
				err = errors.New("Segmentation fault (memory access violation)")
				return formatError("syscall.SIGSEGV", err)
			}

			trap := ws.TrapCause()
			if trap == syscall.PTRACE_EVENT_CLONE {
				culprit := "Trap Cause: PTRACE_EVENT_CLONE"
				debugMessage("%s (%d)", culprit, trap)

				if !cfg.AllowMultiThreading {
					err = errors.New("Cloning processes is not allowed")
					return formatError(culprit, err)
				}
			} else if trap == syscall.PTRACE_EVENT_VFORK_DONE {
				debugMessage("Trap Cause: PTRACE_EVENT_VFORK_DONE (%d)", trap)
			} else if trap == syscall.PTRACE_EVENT_EXIT {
				debugMessage("Trap Cause: PTRACE_EVENT_EXIT (%d)", trap)
				level--
			} else {
				var trapName string
				switch trap {
				case syscall.PTRACE_EVENT_FORK:
					trapName = "PTRACE_EVENT_FORK"
				case syscall.PTRACE_EVENT_VFORK:
					trapName = "PTRACE_EVENT_VFORK"
				case syscall.PTRACE_EVENT_EXEC:
					trapName = "PTRACE_EVENT_EXEC"
				}

				if len(trapName) == 0 {
					debugMessage("Unknown trap cause: %d (%s)", trap, ws.StopSignal().String())
				} else {
					culprit := fmt.Sprintf("Trap Cause: %s", trapName)
					debugMessage("%s (%d)", culprit, trap)

					if !cfg.AllowCreateProcesses {
						err = errors.New("Spawning child processes is not allowed")
						return formatError(culprit, err)
					}
				}
			}
		} else {
			debugMessage("Status is not handled")
		}

		// err := syscall.PtraceSetOptions(currentPid, options)
		// if err != nil {
		// 	return formatError("syscall.PtraceSetOptions", err)
		// }

		err = syscall.PtraceSyscall(currentPid, 0)
		if err != nil {
			return formatError("syscall.PtraceCont", err)
		}
	}
}

func processCommandName(pid, traceePid int) string {
	var result string

	result = system.GetProcessCommand(pid)
	if pid == traceePid {
		result = fmt.Sprintf("Tracee | %s", result)
	}
	return strings.TrimSpace(result)
}
