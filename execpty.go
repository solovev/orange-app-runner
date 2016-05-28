package main

import (
	"bufio"
	"fmt"
	"orange-app-runner/system"
	"orange-app-runner/util"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

func runProcessViaPTY() (int, error) {
	util.Debug("Moving to pseudo terminal...")
	stringCommand := ""
	for i, arg := range os.Args {
		if arg == "-l" || arg == "-p" || (i > 0 && os.Args[i-1] == "-l") || (i > 0 && os.Args[i-1] == "-p") {
			continue
		}
		if i > 0 {
			stringCommand += " "
		}
		stringCommand += arg
	}

	cmd := exec.Command("/bin/su", cfg.User, "-c", stringCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	pty, err := system.StartPTY(cmd)
	if err != nil {
		return -1, err
	}

	finish := false
	stopKey := util.GetHash(stringCommand)
	util.Debug("Stop key for scanner: %s", stopKey)

	enterPassword(pty)

	messageChan := make(chan string, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		scanner := bufio.NewScanner(pty)
		for scanner.Scan() {
			select {
			case <-messageChan:
				/* Text comes from Stdin, ignore it */
			default:
				text := scanner.Text()
				if finish && scanner.Text() == stopKey {
					util.Debug("Stop key was received")
					return
				}
				if len(text) > 0 && !cfg.Quiet {
					fmt.Println(text)
				}
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(os.Stdin)

		for scanner.Scan() {
			text := scanner.Text()
			messageChan <- text
			write(text, pty)
		}
	}()

	exit := 0
	err = cmd.Wait()
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exit = status.ExitStatus()
			}
		} else {
			exit = -1
		}
	}

	finish = true
	write(stopKey, pty)
	wg.Wait()

	return exit, nil
}

func enterPassword(pty *os.File) {
	scanner := bufio.NewScanner(pty)
	scanner.Split(bufio.ScanRunes)
	/* Waiting for password request */
	for scanner.Scan() {
		break
	}
	write(cfg.Password, pty)
}

func write(value string, pty *os.File) {
	pty.WriteString(fmt.Sprintf("%s\n", value))
}
