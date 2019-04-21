package runner

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/solovev/orange-app-runner/system"
	"github.com/solovev/orange-app-runner/util"
)

// RunProcess запускает основной процесс отслеживания
func RunProcess(cfg *util.Config) (int, error) {
	util.Debug("Starting process: %s %v", cfg.ProcessPath, cfg.ProcessArgs)

	if len(cfg.ProcessPath) == 0 {
		return -1, errors.New("ProcessPath isn't specified")
	}

	cmd := exec.Command(cfg.ProcessPath, cfg.ProcessArgs...)
	// Передаем в параметры переменные среды
	cmd.Env = cfg.Environment

	homeDir, err := util.GetProcessHomeDirectory(cfg.HomeDirectory)
	if err != nil {
		return -1, fmt.Errorf("Unable to create home directory \"%s\": %v", cfg.HomeDirectory, err)
	}
	cmd.Dir = homeDir

	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// Атрибутом "ptrace" сообщаем системе, что будем отслеживать действия процесса.
	cmd.SysProcAttr.Ptrace = true
	// "Убиваем" всех потомков процесса при его смерти.
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	// Если текущий пользователь имеет привелегии администратора, то эмулируем запуск
	// под другим пользователем, указанным в параметре "-l"
	if cfg.User != system.GetCurrentUserName() {
		uid, gid, err := system.FindUser(cfg.User)
		if err != nil {
			return -1, err
		}
		util.Debug("Set process credential to %s [UID: %d, GID: %d]", cfg.User, uid, gid)
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	// Если параметр "-o" был указан, то перенаправляем stdout запущенного процесса в файл
	// И, если не указан параметр "-q", еще в нашу консоль.
	if len(cfg.OutputFile) > 0 {
		var outputFile *os.File
		outputFile, err = util.CreateFile(cfg.OutputFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to create \"%s\": %v", cfg.OutputFile, err)
		}
		defer outputFile.Close()
		go fromPipe(cfg, stdout, outputFile, wg)
	} else {
		// Параметр "-o" не был указан, перенаправляем stdout только в консоль.
		go fromPipe(cfg, stdout, nil, wg)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, err
	}
	// Если параметр "-e" был указан, то перенаправляем stderr запущенного процесса в файл
	// И, если не указан параметр "-q", еще в нашу консоль.
	if len(cfg.ErrorFile) > 0 {
		var errorFile *os.File
		errorFile, err = util.CreateFile(cfg.ErrorFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to create \"%s\": %v", cfg.ErrorFile, err)
		}
		defer errorFile.Close()
		go fromPipe(cfg, stderr, errorFile, wg)
	} else {
		// Параметр "-e" не был указан, перенаправляем stderr только в консоль.
		go fromPipe(cfg, stderr, nil, wg)
	}

	// Если указан параметр "-i", то stdin'ом процесса является указанный файл.
	if len(cfg.InputFile) > 0 {
		var inputFile *os.File
		inputFile, err = util.OpenFile(cfg.InputFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to open \"%s\": %v", cfg.InputFile, err)
		}
		defer inputFile.Close()
		cmd.Stdin = inputFile
	} else {
		// Если не указан параметр "-i", stdin'ом процесса является консоль
		cmd.Stdin = os.Stdin
	}

	// Запускаем процесс
	err = cmd.Start()
	if err != nil {
		return -1, err
	}
	// Убеждаемся, что после выхода из функции (runProcess) запущенный процесс завершится.
	defer cmd.Process.Kill()

	pid := cmd.Process.Pid
	util.Debug("Process id: %d", pid)

	// Если указан параметр "-1", процесс будет выполнятся только на 1ом ядре процессора.
	if len(cfg.Affinity) > 0 {
		set, err := system.SetAffinity(cfg.Affinity, pid)
		if err != nil {
			return 0, err
		}
		util.Debug("Processor affinity was set to: %v", set)
	}

	// Если указан параметр "-s", создаем файл сбора статистики.
	var storeFile *os.File
	if len(cfg.StoreFile) > 0 {
		storeFile, err = util.OpenFile(cfg.StoreFile)
		if err != nil {
			return -1, fmt.Errorf("Unable to open storage file: %v", err)
		}
		defer storeFile.Close()
	}

	// Начинаем отслеживать потребление ресурсов в отдельном потоке.
	go measureUsage(cfg, storeFile, cmd.Process)

	// В отдельном потоке начинаем отслеживать время жизни процесса, если указан "-t".
	timeLimit := cfg.TimeLimit.Value()
	if timeLimit > 0 {
		go func() {
			select {
			case <-time.After(timeLimit):
				checkError(cmd.Process, fmt.Errorf("Time limit [%s] exceeded", cfg.TimeLimit.String()))
			}
		}()
	}

	// Т.к. атрибут "ptrace" включен, то, после запуска, начинаем ждать пока
	// процесс изменит свой статус (остановится, завершится, подаст сигнал и т.д.)
	var ws syscall.WaitStatus
	waitPid, err := syscall.Wait4(-1, &ws, syscall.WALL, nil)
	if err != nil {
		return -1, fmt.Errorf("Error [syscall.Wait4] for \"%s\": %v", cfg.BaseName, err)
	}
	if waitPid != pid {
		return -1, fmt.Errorf("Error [syscall.Wait4]: First waited PID (%d) not equal to \"%s\" PID (%d)", waitPid, cfg.BaseName, pid)
	}

	// Ptrace-параметры
	options := syscall.PTRACE_O_TRACEFORK
	options |= syscall.PTRACE_O_TRACEVFORK
	options |= syscall.PTRACE_O_TRACECLONE
	options |= syscall.PTRACE_O_TRACEEXIT
	parentPid := 0

	// Начинаем рекурсивно отслеживать поведение запущенного процесса и его потомков.
	// Пример. Если запущенный процесс (указанный в параметре "oar") создал потомка_1, то
	// начинаем отслеживать этого потомка_1, потомок_1 тоже может создать потомка (потомка_2),
	// в новой итерации начинаем отслеживать этого нового потомка (потомка_2).
	// Если потомок_2 больше не создает новых потомков, то следущая итерация цикла
	// будет принадлежать его родителю (потомку_1), если потомок_1 также не создает потомков,
	// переходим к главному процессу (запущенному через "oar").
	// Первая (и последняя) итерация будет отслеживать наш запущенный процесс, остальные - потомков.
	// 	Creation 0 | Main Process			   Exit	5 | - - - - - Main Process
	// 	         1 | - Child_1					4 | - - - Child_1
	// 	         2 | - - - Child_2				3 | Child_2
	for {
		// После того как процесс-потомок остановился (после syscall.Wait4)
		// Передаем ему ptrace-параметры, которые заставят его останавливаться
		// в тех случаях, когда он начинает создавать дочерний процесс.
		syscall.PtraceSetOptions(waitPid, options)
		syscall.PtraceCont(waitPid, 0)

		parentPid = waitPid

		// Снова ждем пока процесс-потомок изменит свой статус, теперь это может быть
		// не только остановка, завершение, сигналирование, но и создание дочернего процесса.
		waitPid, err = syscall.Wait4(-1, &ws, syscall.WALL, nil)
		if err != nil {
			return -1, fmt.Errorf("Error [syscall.Wait4] for [PID: %d, PPID %d]: %v", waitPid, parentPid, err)
		}
		command := system.GetProcessCommand(waitPid)
		util.Debug("Waited PID: %d, PPID: %d, CMD: %s", waitPid, parentPid, command)

		// Проверяем, завершился ли процесс-потомок
		if ws.Exited() {
			util.Debug(" - Process [PID: %d] finished", waitPid)
			// Если завершенный процесс-потомок является нашим запущенным процессом
			// (процессом, указанным в параметре "oar"), то ломаем цикл for,
			// и выходим из функции (runProcess) с кодом выхода ws.ExitStatus()
			if waitPid == pid {
				break
			}
			// Если нет, переходим к его родителю.
			continue
		}

		if ws.Signaled() {
			util.Debug(" - Process [PID: %d] signaled: %v", waitPid, ws)
			continue
		}

		sigtrap := uint32(syscall.SIGTRAP)
		sigsegv := uint32(syscall.SIGSEGV)
		// Если причиной изменения статуса является создание дочернего процесса:
		// Если параметер "-Xacp" не установлен, то после попытки создать дочерний процесс
		// функция (runProcess) завершится, сработает defer cmd.Process.Kill()
		// (процесс будет убит), а т.к. в атрибутах запуска процесса стоит
		// Pdeathsig: syscall.SIGKILL, то будут убиты все созданные потомки.
		if !cfg.AllowCreateProcesses {
			switch uint32(ws) >> 8 {
			case sigtrap | (syscall.PTRACE_EVENT_CLONE << 8):
				// Для создания отдельного потока, процесс создает потомка,
				// параметр "-Xamt" разрешает многопоточность.
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
				return -1, fmt.Errorf("Segmentation fault! [PID %d, PPID %d]", waitPid, parentPid)
			}
			// Если параметер "-Xacp" установлен, то просто выводим инфу о созданных потомках
		} else {
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
	// Ждем, пока функции перенаправления stdout и stderr в файл/консоль,
	// запущенные в отдельных потоках, закончат свою работу
	wg.Wait()

	return ws.ExitStatus(), nil
}

func measureUsage(cfg *util.Config, storage *os.File, process *os.Process) {
	// Проверяем, не завершился ли процесс до того, как мы начнем считать потребление ресурсов.
	if _, err := os.Stat(fmt.Sprintf("/proc/%d/stat", process.Pid)); err == nil {
		// Потребление CPU в % считается по такой формуле:
		// consumtion = (cores * (ptA - ptB) * 100) / (ttA - ttB)
		// Где	cores	- Количество используемых ядер процессорa
		//	ptA	- Потребляемое время cpu процессом в момент времени А
		//	ptB	- Потребляемое время cpu процессом в момент времени B
		//	ttA	- Нагруженность процессора (общее время) в момент A
		//	ttB	- Нагруженность процессора (общее время) в момент B
		// Замер А позже замера B (A > B)
		ptB, _, err := system.GetProcessStats(process.Pid)
		checkError(process, err)
		ttB, err := system.GetTotalCPUTime()
		checkError(process, err)

		cores, err := system.GetCPUCount(process.Pid)
		checkError(process, err)

		util.Debug("Process using %d cpu cores.", cores)

		idle := 0
		idleLimit := cfg.IdleLimit.Seconds()

		// Проводим замер каждую секунду? работы программы
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-ticker.C:
				ttA, err := system.GetTotalCPUTime()
				checkError(process, err)

				ptA, processMemory, err := system.GetProcessStats(process.Pid)
				checkError(process, err)

				// Расчитываем потребление CPU
				load := float64(uint64(cores)*(ptA-ptB)) / float64(ttA-ttB)
				if idleLimit > 0 {
					// Если потребление CPU меньше чем допустимая нагрузка
					// увеличиваем счетчик простоя (idle)
					if cfg.RequiredLoad.Value() > load {
						idle++
					} else {
						idle = 0
					}
				}
				stringMemory := util.StringifyMemory(processMemory)
				stringLoad := util.StringifyLoad(load)
				util.Debug(" - [Memory: %s/%s, Load: %s/%s]", stringMemory, cfg.MemoryLimit.String(), stringLoad, cfg.RequiredLoad.String())

				// Записываем полученные данные о потреблении ресурсов в файл, указанный в "-s".
				if storage != nil {
					storage.WriteString(fmt.Sprintf("%s,%f,%d\n", time.Now().Format("15:04:05"), load, processMemory))
					err = storage.Sync()
					checkError(process, err)
				}

				// Проверка на превышение указанных лимитов (если параметры были указаны)
				if idleLimit > 0 && idle >= idleLimit {
					checkError(process, fmt.Errorf("Idle time limit [%d] exceeded", cfg.IdleLimit.Seconds()))
				}
				memoryLimit := cfg.MemoryLimit.Value()
				if memoryLimit > 0 && processMemory > memoryLimit {
					checkError(process, fmt.Errorf("Memory limit [%s] exceeded", cfg.MemoryLimit.String()))
				}
				ptB = ptA
				ttB = ttA
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
		os.Exit(0)
	}
}

func fromPipe(cfg *util.Config, r io.Reader, f *os.File, wg *sync.WaitGroup) {
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
