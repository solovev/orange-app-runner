package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"orange-app-runner/system"
	"orange-app-runner/util"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Переменная cfg видна во всем пакете main (exec.go, execpty.go, main.go)
var (
	cfg *util.Config
)

func main() {
	// Говорим "oar" использовать текущий поток выполнения ОС только под основную горутину.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cfg = util.NewConfig()

	// Если указан параметр "-q", то перенаправляем вывод консольных логов в никуда.
	if cfg.Quiet {
		log.SetOutput(ioutil.Discard)
	}

	// Если указан параметр "-w" то перезапускаем введенную команду
	// (./oar [<options>] <program> [<parameters>]) в новом терминале без параметра "-w".
	// Пример:
	//	Оригинальная команда: "./oar -w -x -1 ./command"
	//	Перезапуск в новом терминале "./oar -x -1 ./command"
	if cfg.DisplayWindow {
		restartItself("gnome-terminal")
	}

	// Если указан параметр "-d", то убеждаемся что такая директория существует.
	// В противном случае создаем ее.
	if len(cfg.HomeDirectory) > 0 {
		path := cfg.HomeDirectory
		// Определяем, является ли указанный путь в "-d" абсолютным или относительным.
		// Если путь не начинается с ~ и /, то он относительный.
		if path[0] != '~' && path[0] != '/' {
			dir, err := os.Getwd()
			if err != nil {
				fmt.Printf("Unable to get working directory: %v.\n", err)
				system.Exit(1)
			}
			// Если путь относительный, конкатенируем его с текущий директорией.
			path = filepath.Join(dir, cfg.HomeDirectory)
		}
		// Создаем директорию по указанному пути.
		if _, err := os.Stat(path); os.IsNotExist(err) {
			err := os.MkdirAll(path, 0777)
			if err != nil {
				fmt.Printf("Error creating home directory \"%s\": %v.\n", path, err)
				system.Exit(1)
			}
			util.Debug("Home directory \"%s\" was just created", path)
		} else {
			util.Debug("Home directory \"%s\" is exists", path)
		}
	}

	var err error
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
		exitCode, err = runProcess()
	} else {
		exitCode, err = runProcessViaPTY()
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

func restartItself(from string) {
	if from == "gnome-terminal" {
		terminalArgs := []string{"-x"}
		for _, arg := range os.Args {
			if arg != "-w" {
				terminalArgs = append(terminalArgs, arg)
			}
		}
		cmd := exec.Command(from, terminalArgs...)
		err := cmd.Run()
		if err != nil {
			log.Printf("Unable to open new \"%s\" terminal: %v\n", from, err)
			system.Exit(1)
		}
		log.Println("Redirected to new terminal.")
	}
	system.Exit(0)
}
