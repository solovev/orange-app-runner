package main

import (
	"log"
	"runtime"

	"github.com/solovev/orange-app-runner/runner"
	"github.com/solovev/orange-app-runner/system"
	"github.com/solovev/orange-app-runner/util"
)

func main() {
	// Говорим "oar" использовать текущий поток выполнения ОС только под основную горутину.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cfg := util.NewConfig()

	// Если указан параметр "-w" то перезапускаем введенную команду
	if cfg.DisplayWindow {
		util.RestartItself("gnome-terminal")
	}

	// Если указан параметр "-d", то убеждаемся что такая директория существует.
	// В противном случае создаем ее.
	homeDir, err := util.CreateHomeDirectory(cfg.HomeDirectory)
	if err != nil {
		util.Debug("Unable to create home directory \"%s\": %v.", cfg.HomeDirectory, err)
		system.Exit(1)
	}

	if cfg.HomeDirectory != homeDir {
		cfg.HomeDirectory = homeDir
		util.Debug("Home directory path changed to: \"%s\".", cfg.HomeDirectory)
	}

	exitCode := 0
	// а) Если указанное имя пользователя в параметре "-l" совпадает с текущим
	//	пользователем (под которым мы запустили "oar"), или текущий пользователь
	//	имеет привилегии администратора, то сразу переходим в функцию runProcess.
	// б) Если текущий пользователь не обладает привилегиями администратора, то
	//	необходимо перед этим залогиниться, использовов имя пользователя (-l) и пароль (-p),
	//	сделать это можно при помощи системной команды "/bin/su <user> -c <command>".
	//	Перезапускаем введенную команду (./oar [<options>] <program> [<parameters>])
	//	через "/bin/su <user> -c <command>" в псевдотерминале без параметров "-l" и "-p".
	//	Т.к. сис-ую команду "/bin/su" запустить через fork/exec не представляется возможным,
	//	выполнить ее можно только через псевдотерминал (ф-ия runProcessViaPTY).
	//	Пример:
	//		Введенная команда: ./oar -t 10s -l test -p qwerty ./command
	//		Пойдет в псевдотерминал: /bin/su test -c "./oar -t 10s ./command"
	defaultRunning := cfg.User == system.GetCurrentUserName() || system.IsCurrentUserRoot()
	if defaultRunning {
		exitCode, err = runner.RunProcess(cfg)
	} else {
		exitCode, err = runner.RunProcessViaPTY(cfg)
	}

	if err != nil {
		log.Printf("Process killed. Cause: %v\n", err)
	}

	if exitCode != -1 {
		if defaultRunning {
			util.Debug("Exit code of \"%s\": %d", cfg.ProcessPath, exitCode)
		}
		// Если указан параметр "-x", то "oar" вернет код выхода отслеживаемого процесса.
		if cfg.ExitCode {
			system.Exit(exitCode)
		}
	}
}
