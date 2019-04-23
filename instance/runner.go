package instance

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/solovev/orange-app-runner/system"
	"golang.org/x/sys/unix"
)

func Run(processPath string, processArgs []string, cfg *Config) (int, error) {
	processName := filepath.Base(processPath)

	log.Infof("Starting tracee (%s %v)...\n", processName, processArgs)

	cmd := exec.Command(processPath, processArgs...)

	cmd.Env = append([]string{
		"PS1=" + fmt.Sprintf("[OAR eJudge] \"%s\" $ ", processName),
	}, cfg.Env...)
	cmd.Dir = cfg.WorkingDir

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ptrace := !cfg.AllowCreateProcesses || !cfg.AllowMultiThreading
	log.Debugf("Ptrace - %t [Allow create processes - %t] [Allow multithreading - %t]", ptrace, cfg.AllowCreateProcesses, cfg.AllowMultiThreading)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Ptrace:    ptrace,
		Pdeathsig: syscall.SIGKILL,
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}
	defer cmd.Process.Kill()

	pid := cmd.Process.Pid
	if len(cfg.Affinity) > 0 {
		set, err := system.SetAffinity(cfg.Affinity, pid)
		if err != nil {
			return -1, err
		}
		log.Debugf("Processor affinity was set to: %v\n", set)
	}

	if err := cmd.Wait(); err != nil {
		status := cmd.ProcessState.Sys().(syscall.WaitStatus)
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
	}

	return trace(cmd, cfg)
}

func trace(cmd *exec.Cmd, cfg *Config) (int, error) {
	var ws syscall.WaitStatus

	level := 0
	iterations := 0

	traceePid := cmd.Process.Pid
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
		currentCommand := processCommandName(currentPid, cmd)
		previousCommand := processCommandName(previousPid, cmd)

		return -1, fmt.Errorf("%d | Error at level %d [%s] for [PID: %d (%s), Prev. PID: %d (%s)]: %v", iterations, level, culprit, currentPid, currentCommand, previousPid, previousCommand, err)
	}

	debugStatus := func(pid int, status syscall.WaitStatus) {
		if !cfg.Debug {
			return
		}

		name := processCommandName(pid, nil)

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
		log.Debugf(prefix + " " + msg + "\n")
	}

	err := syscall.PtraceSetOptions(currentPid, options)
	if err != nil {
		return formatError("syscall.PtraceSetOptions (before loop)", err)
	}

	err = syscall.PtraceCont(currentPid, 0)
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

		waitPid, err := syscall.Wait4(-1, &ws, syscall.WALL, nil)
		if err != nil {
			return formatError("syscall.Wait4", err)
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

		err = syscall.PtraceCont(currentPid, 0)
		if err != nil {
			return formatError("syscall.PtraceCont", err)
		}
	}

	status := cmd.ProcessState.Sys().(syscall.WaitStatus)
	debugStatus(traceePid, status)
	debugStatus(currentPid, ws)

	err = errors.New("Unexpected trace completion")
	return formatError("stop tracer", err)
}

func processCommandName(pid int, traceeCommand *exec.Cmd) string {
	var result string
	if traceeCommand != nil && pid == traceeCommand.Process.Pid {
		result = fmt.Sprintf("TRACEE - %s", traceeCommand.Path)
	} else {
		result = system.GetProcessCommand(pid)
	}
	return strings.TrimSpace(result)
}
