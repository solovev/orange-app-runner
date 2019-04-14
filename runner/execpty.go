package runner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/solovev/orange-app-runner/system"
	"github.com/solovev/orange-app-runner/util"
)

func RunProcessViaPTY(cfg *util.Config) (int, error) {
	util.Debug("Moving to pseudoterminal...")
	stringCommand := ""

	// Уничтожаем параметры "-l" и "-p"
	for i, arg := range os.Args {
		if arg == "-l" || arg == "-p" || (i > 0 && os.Args[i-1] == "-l") || (i > 0 && os.Args[i-1] == "-p") {
			continue
		}
		if i > 0 {
			stringCommand += " "
		}
		stringCommand += arg
	}

	// Запускаем системную команду "/bin/su", через которую перезапускаем "oar".
	cmd := exec.Command("/bin/su", cfg.User, "-c", stringCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// "Убиваем" всех потомков при смерти родителя.
		Pdeathsig: syscall.SIGKILL,
	}

	// Запускаем "/bin/su ..." через псевдотерминал.
	pty, err := system.StartPTY(cmd)
	if err != nil {
		return -1, err
	}

	// Флаг завершения
	finish := false
	// Создаем стоп-слово для сканера псевдотерминала (подробнее о сканере - ниже)
	// После того как "oar" завершил отслеживать выполнение процесса, чтобы
	// выйти из текущей функции надо подождать пока сканер, запускаемый в отдельном потоке,
	// вышел из бесконечного цикла и завершился. Т.к. псевдотерминал представляет из себя
	// некий небуферизированный источник, то сканер никогда не прекратит его считывать,
	// пока не считает стоп-слово, которое мы запишем в псевдотерминал по завершении "oar"
	// одновременно с изменением флага завершения (finish) на true.
	stopKey := util.GetHash(stringCommand)
	util.Debug("Stop key for scanner: %s", stopKey)

	// Сразу (почти) после запуска "/bin/su" попросит ввести пароль, вводим его.
	enterPassword(cfg.Password, pty)

	// Т.к. псевдотерминал представляет из себя одновременно и приемник и передатчик
	// (stdin, stdout в 1 флаконе), то когда мы введем что-либо в него, сканер псевдотерминала
	// считает наше сообщение и выведет его на экран повторно, чтобы этого избежать,
	// будем записывать наше сообщение в канал "messageChan", и проверять в сканере,
	// что если считываемое сообщение такое же как то, что лежит в канале, сканер выводить его не будет.
	messageChan := make(chan string, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Начинаем в отдельном потоке считывать сканером псевдотерминал, чтобы
	// перенаправить вывод в стандартную консоль
	go func() {
		defer wg.Done()

		scanner := bufio.NewScanner(pty)
		for scanner.Scan() {
			select {
			// Проверяем, есть ли что-то в канале
			case <-messageChan:
				// Если да, выходит, что сообщение пришло из Stdin, игнорируем его.
				// Канал messageChan очищается.
			default:
				text := scanner.Text()
				// Завершаем работу сканера псевдотерминала, если он получил стоп-слово
				if finish && scanner.Text() == stopKey {
					util.Debug("Stop key was received")
					return
				}
				// Выводим сообщение из псевдотерминала в обычную консоль
				if len(text) > 0 && !cfg.Quiet {
					fmt.Println(text)
				}
			}
		}
	}()

	// Начинаем в отдельном потоке считывать сканером вводимые данные (stdin)
	// чтобы записать их в псевдотерминал
	go func() {
		scanner := bufio.NewScanner(os.Stdin)

		for scanner.Scan() {
			text := scanner.Text()
			// Записываем введенное сообщение в канал, для того, чтобы сканер
			// псевдотерминала не выводил его повторно в нашу консоль.
			messageChan <- text
			// Записываем сообщение в псевдотерминал.
			write(text, pty)
		}
	}()

	exit := 0

	// Останавливаемся здесь, чтобы подождать завершения созданного процесса
	err = cmd.Wait()
	if err != nil {
		// Проверяем, является ли "err" кодом ошибки
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exit = status.ExitStatus()
				err = nil
			}
		} else {
			exit = -1
		}
	}

	finish = true
	// Посылаем стоп-слово в сканер псевдотерминала
	write(stopKey, pty)
	// Ждем пока сканер псевдотерминала прочтет стоп-слово и завершится
	wg.Wait()

	return exit, err
}

func enterPassword(password string, pty *os.File) {
	scanner := bufio.NewScanner(pty)
	scanner.Split(bufio.ScanRunes)
	// Ждем запроса пароля от "/bin/su ..."
	for scanner.Scan() {
		break
	}
	// Вводим пароль
	write(password, pty)
}

func write(value string, pty *os.File) {
	pty.WriteString(fmt.Sprintf("%s\n", value))
}
