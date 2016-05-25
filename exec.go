package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"oar/system"
	"oar/util"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func runProcess() (int, error) {
	util.Debug("Starting process: %s %v", cfg.ProcessPath, cfg.ProcessArgs)

	var cmd *exec.Cmd
	if cfg.DisplayWindow && cfg.Terminal == "gnome-terminal" {
		terminalArgs := []string{"-x"}
		for _, arg := range os.Args {
			if arg != "-w" {
				terminalArgs = append(terminalArgs, arg)
			}
		}
		cmd = exec.Command(cfg.Terminal, terminalArgs...)
		err := cmd.Run()
		if err != nil {
			return -1, err
		}
		log.Println("Redirected to new terminal.")
		system.Exit(0)
	}
	cmd = exec.Command(cfg.ProcessPath, cfg.ProcessArgs...)
	cmd.Env = cfg.Environment
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Ptrace = true
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	if len(cfg.User) > 0 && cfg.User != system.GetCurrentUserName() && system.IsCurrentUserRoot() {
		uid, gid, err := system.FindUser(cfg.User)
		if err != nil {
			return -1, err
		}
		util.Debug("Set process credential to %s [UID: %d, GID: %d]", cfg.User, uid, gid)
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}
	}

	wg := &sync.WaitGroup{}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	wg.Add(2)
	if len(cfg.OutputFile) > 0 {
		f, err := util.CreateFile(cfg.HomeDirectory, cfg.OutputFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to create \"%s\": %v", cfg.OutputFile, err)
		}
		defer f.Close()
		go fromPipe(stdout, f, wg)
	} else {
		go fromPipe(stdout, nil, wg)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, err
	}
	if len(cfg.ErrorFile) > 0 {
		f, err := util.CreateFile(cfg.HomeDirectory, cfg.ErrorFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to create \"%s\": %v", cfg.ErrorFile, err)
		}
		defer f.Close()
		go fromPipe(stderr, f, wg)
	} else {
		go fromPipe(stderr, nil, wg)
	}

	if len(cfg.InputFile) > 0 {
		f, err := util.OpenFile(cfg.HomeDirectory, cfg.InputFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to open \"%s\": %v", cfg.InputFile, err)
		}
		defer f.Close()
		cmd.Stdin = f
	} else {
		cmd.Stdin = os.Stdin
	}

	err = cmd.Start()
	if err != nil {
		return -1, err
	}
	defer cmd.Process.Kill()

	pid := cmd.Process.Pid
	util.Debug("Process id: %d", pid)

	if cfg.SingleCore {
		err := system.SetCPUAffinity(pid)
		if err != nil {
			return -1, fmt.Errorf("Unable to set cpu affinity: %v", err)
		}
	}

	var storeFile *os.File
	if len(cfg.StoreFile) > 0 {
		storeFile, err = util.OpenFile(cfg.HomeDirectory, cfg.StoreFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to open storage file: %v", err)
		}
		defer storeFile.Close()
	}
	go measureUsage(storeFile, cmd.Process)
	go func() {
		timeLimit := cfg.TimeLimit.Value()
		if timeLimit > 0 {
			select {
			case <-time.After(cfg.TimeLimit.Value()):
				checkError(cmd.Process, fmt.Errorf("Time limit [%s] exceeded", cfg.TimeLimit.String()))
			}
		}
	}()
	// - - - - - - - - - - - - - - - - - - - - - -
	var ws syscall.WaitStatus
	/*
	* There we need catch RUsage (first mem, cpu usage)
	* and start reading stat file in another goroutine
	 */
	waitPid, err := syscall.Wait4(-1, &ws, syscall.WALL, nil)
	if err != nil {
		return -1, fmt.Errorf("Error [syscall.Wait4] for \"%s\": %v", cfg.BaseName, err)
	}
	if waitPid != pid { // Or... is it normal? I don't think so
		return -1, fmt.Errorf("Error [syscall.Wait4]: First waited PID (%d) not equal to \"%s\" PID (%d)", waitPid, cfg.BaseName, pid)
	}

	options := syscall.PTRACE_O_TRACEFORK
	options |= syscall.PTRACE_O_TRACEVFORK
	options |= syscall.PTRACE_O_TRACECLONE
	options |= syscall.PTRACE_O_TRACEEXIT
	parentPid := 0
	for {
		syscall.PtraceSetOptions(waitPid, options)
		syscall.PtraceCont(waitPid, 0)

		parentPid = waitPid

		waitPid, err = syscall.Wait4(-1, &ws, syscall.WALL, nil)
		if err != nil {
			return -1, fmt.Errorf("Error [syscall.Wait4] for [PID: %d, PPID %d]: %v", waitPid, parentPid, err)
		}
		command := system.GetProcessCommand(waitPid)
		util.Debug("Waited PID: %d, PPID: %d, CMD: %s", waitPid, parentPid, command)
		if ws.Exited() {
			util.Debug(" - Process [PID: %d] finished", waitPid)
			if waitPid == pid {
				break /* Break ptrace loop if first process (cfg.BaseName) finished. */
			}
			continue
		}

		if ws.Signaled() {
			util.Debug(" - Process [PID: %d] signaled: %v", waitPid, ws)
			continue
		}

		sigtrap := uint32(syscall.SIGTRAP)
		sigsegv := uint32(syscall.SIGSEGV)
		if !cfg.AllowCreateProcesses {
			switch uint32(ws) >> 8 {
			case sigtrap | (syscall.PTRACE_EVENT_CLONE << 8):
				if !cfg.MultiThreadedProcess {
					return -1, fmt.Errorf("Process attempt to clone himself")
				}
				clonePid, err := syscall.PtraceGetEventMsg(waitPid)
				if err != nil {
					util.Debug("Unable to retrieve id of cloned process: %v", err)
				}
				util.Debug("Process [%d] just maked clone [%d]", waitPid, clonePid)
			case sigtrap | (syscall.PTRACE_EVENT_FORK << 8):
				fallthrough
			case sigtrap | (syscall.PTRACE_EVENT_VFORK << 8):
				fallthrough
			case sigtrap | (syscall.PTRACE_EVENT_VFORK_DONE << 8):
				fallthrough
			case sigtrap | (syscall.PTRACE_EVENT_EXEC << 8):
				return -1, fmt.Errorf("Attempt to create new process")
			case sigsegv:
				/*
				* Here, we need to kill broken process, like:
				* 	err = syscall.Kill(waitPid, 9)
				* But if we just return from this function
				* defer of cmd.Process.Kill was triggered
				* and all child processes must be killed by Deathsig ?
				 */
				return -1, fmt.Errorf("Segmentation fault! [PID %d, PPID %d]", waitPid, parentPid)
			}
		} else { // If spawning new process is allowed... just for debug
			switch uint32(ws) >> 8 {
			case sigtrap | (syscall.PTRACE_EVENT_EXIT << 8):
				util.Debug(" - Detected exit event.")
			case sigtrap | (syscall.PTRACE_EVENT_CLONE << 8):
				nPid, err := syscall.PtraceGetEventMsg(waitPid)
				if err != nil {
					util.Debug("- [PTRACE_EVENT_CLONE] Ptrace event message retrieval failed: %v", err)
				}
				util.Debug("- Ptrace clone [%d] event detected", nPid)
			case sigtrap | (syscall.PTRACE_EVENT_FORK << 8):
				nPid, err := syscall.PtraceGetEventMsg(waitPid)
				if err != nil {
					util.Debug("- [PTRACE_EVENT_FORK] Ptrace event message retrieval failed: %v", err)
				}
				util.Debug("- Ptrace fork [%d] event detected", nPid)
			case sigtrap | (syscall.PTRACE_EVENT_VFORK << 8):
				nPid, err := syscall.PtraceGetEventMsg(waitPid)
				if err != nil {
					util.Debug("- [PTRACE_EVENT_VFORK] Ptrace event message retrieval failed: %v", err)
				}
				util.Debug("- Ptrace vfork [%d] event detected", nPid)
			case sigtrap | (syscall.PTRACE_EVENT_VFORK_DONE << 8):
				nPid, err := syscall.PtraceGetEventMsg(waitPid)
				if err != nil {
					util.Debug("- [PTRACE_EVENT_VFORK_DONE] Ptrace event message retrieval failed: %v", err)
				}
				util.Debug("- Ptrace vfork done [%d] event detected", nPid)
			case sigtrap | (syscall.PTRACE_EVENT_EXEC << 8):
				util.Debug("- Ptrace exec event detected")
			case sigtrap | (0x80 << 8): // PTRACE_EVENT_STOP
				util.Debug("- Ptrace stop event detected")
			case sigtrap:
				util.Debug("- Sigtrap detected")
			case uint32(syscall.SIGCHLD):
				util.Debug("- Sigchld detected")
			case uint32(syscall.SIGSTOP):
				util.Debug("- Sigstop detected")
			case sigsegv:
				util.Debug("- Sigsegv detected.")
				return -1, fmt.Errorf("Segmentation fault! [PID %d, PPID %d]", waitPid, parentPid)
			default:
				util.Debug(" - Process [%d] stopped for unknown reasons [Status %v, Signal %d]", waitPid, ws, ws.StopSignal())
			}
		}
	}
	// - - - - - - - - - - - - - - - - - - - - - -
	wg.Wait()

	return ws.ExitStatus(), nil
}

func measureUsage(storage *os.File, process *os.Process) {
	if _, err := os.Stat(fmt.Sprintf("/proc/%d/stat", process.Pid)); err == nil {
		processTimeB, _, err := system.GetProcessStats(process.Pid)
		checkError(process, err)
		cpuTimeB, err := system.GetTotalCPUTime()
		checkError(process, err)

		cores := 1

		if !cfg.SingleCore {
			cores, err = system.GetCPUCount()
			checkError(process, err)
		}
		util.Debug("Process using %d cpu cores.", cores)

		idle := 0
		idleLimit := cfg.IdleLimit.Seconds()

		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-ticker.C:
				cpuTimeA, err := system.GetTotalCPUTime()
				checkError(process, err)

				processTimeA, processMemory, err := system.GetProcessStats(process.Pid)
				checkError(process, err)

				load := float64(uint64(cores)*(processTimeA-processTimeB)) / float64(cpuTimeA-cpuTimeB)
				if idleLimit > 0 {
					if cfg.RequiredLoad.Value() > load {
						idle++
					} else {
						idle = 0
					}
				}
				stringMemory := util.StringifyMemory(processMemory)
				stringLoad := util.StringifyLoad(load)
				util.Debug(" - [Memory: %s/%s, Load: %s/%s]", stringMemory, cfg.MemoryLimit.String(), stringLoad, cfg.RequiredLoad.String())

				if storage != nil {
					storage.WriteString(fmt.Sprintf("%s,%f,%d\n", time.Now().Format("15:04:05"), load, processMemory))
					err = storage.Sync()
					checkError(process, err)
				}

				if idleLimit > 0 && idle >= idleLimit {
					checkError(process, fmt.Errorf("Idle time limit [%d] exceeded", cfg.IdleLimit.Seconds()))
				}
				memoryLimit := cfg.MemoryLimit.Value()
				if memoryLimit > 0 && processMemory > memoryLimit {
					checkError(process, fmt.Errorf("Memory limit [%s] exceeded", cfg.MemoryLimit.String()))
				}
				processTimeB = processTimeA
				cpuTimeB = cpuTimeA
			}
		}
	}
}

func checkError(process *os.Process, err error) {
	if err != nil {
		log.Printf("Process killed from subthread. Cause: %v\n", err)
		if process != nil {
			process.Kill() // Catch the error?
		}
		system.Exit(0)
	}
}

func fromPipe(r io.Reader, f *os.File, wg *sync.WaitGroup) {
	defer wg.Done()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		if len(text) == 0 {
			continue
		}
		if !cfg.Quiet {
			log.Printf(util.Bold("[%s]: %s"), cfg.BaseName, text)
		}
		if f != nil {
			f.WriteString(fmt.Sprintf("%s\n", text))
		}
	}
	if err := scanner.Err(); err != nil {
		checkError(nil, fmt.Errorf("Pipe handling error: %v", err))
	}
}
