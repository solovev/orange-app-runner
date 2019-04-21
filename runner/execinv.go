package runner

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/solovev/orange-app-runner/system"
	"github.com/solovev/orange-app-runner/util"
)

// https://stackoverflow.com/questions/29528756/how-can-i-read-from-exec-cmd-extrafiles-fd-in-child-process
// https://www.youtube.com/watch?time_continue=642&v=QDDwwePbDtw

type MonitoringConfig struct {
	Command          string
	Args             []string
	Environment      []string
	WorkingDirectory string
	User             string
	Affinity         []int
}

type monitorController struct {
	cmd *exec.Cmd
	cfg *MonitoringConfig

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	debugReader io.ReadCloser
	debugWriter io.WriteCloser

	stopping chan chan error
}

func (c *monitorController) killGroup(pid int) (int, error) {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return -1, err
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		return pgid, err
	}
	return pgid, nil
}

func (c *monitorController) kill() {
	process := c.cmd.Process
	if process == nil {
		return
	}

	pid := process.Pid
	pgid, err := c.killGroup(pid)
	if err == nil {
		return
	}

	c.log("kill", "Error while killing processes group (%d): %v", pgid, err)
	if err := process.Kill(); err != nil {
		c.log("kill", "Killing process (%d) error: %v", pid, err)
	}
}

func (c *monitorController) dispose() {
	close(c.stopping)

	c.kill()

	c.debugWriter.Close()
	c.debugReader.Close()
}

func (c *monitorController) log(ctx, msg string, a ...interface{}) {
	msg = fmt.Sprintf(msg, a...)
	msg = fmt.Sprintln("["+ctx+"]", msg)
	io.WriteString(c.debugWriter, msg)
}

func (c *monitorController) StdOut() io.ReadCloser {
	return c.stdout
}

func (c *monitorController) StdErr() io.ReadCloser {
	return c.stderr
}

func (c *monitorController) DebugMessages() io.ReadCloser {
	return c.debugReader
}

func PrepareMonitoring(cfg *MonitoringConfig) (*monitorController, error) {
	commandName := cfg.Command

	if len(commandName) == 0 {
		return nil, errors.New("Command (or path to executable file) isn't specified")
	}

	if !util.IsFileExists(commandName) {
		path, err := exec.LookPath(commandName)
		if err != nil {
			return nil, fmt.Errorf("Command (or executable file) \"%s\" is not found", commandName)
		}
		commandName = path
	}

	cmd := exec.Command(commandName, cfg.Args...)
	cmd.Env = cfg.Environment

	cwd := cfg.WorkingDirectory
	if len(cwd) > 0 {
		if !util.IsFileExists(cwd) {
			return nil, fmt.Errorf("Working directory \"%s\" is not found", cwd)
		}
		cmd.Dir = cwd
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Ptrace = true
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	user := cfg.User
	if len(user) > 0 && user != system.GetCurrentUserName() {
		uid, gid, err := system.FindUser(user)
		if err != nil {
			return nil, err
		}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	debugReader, debugWriter := io.Pipe()

	return &monitorController{
		stdout: stdout,
		stderr: stderr,
		stdin:  stdin,

		debugReader: debugReader,
		debugWriter: debugWriter,

		stopping: make(chan chan error),

		cmd: cmd,
		cfg: cfg,
	}, nil
}

func (c *monitorController) Start() error {
	defer c.dispose()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cmd := c.cmd
	cfg := c.cfg

	if err := cmd.Start(); err != nil {
		return err
	}

	process := cmd.Process
	pid := process.Pid

	c.log("Start", "Process (%d): %s %v", pid, cfg.Command, cfg.Args)

	if len(cfg.Affinity) > 0 {
		set, err := system.SetAffinity(cfg.Affinity, pid)
		if err != nil {
			return err
		}
		c.log("Start", "Processor affinity was set to: %v", set)
	}

	// state := cmd.ProcessState
	// s := state.SysUsage().(*syscall.Rusage)
	// s.

	return nil
}

func (c *monitorController) Stop() error {
	errc := make(chan error)
	defer close(errc)

	c.stopping <- errc

	return <-errc
}
