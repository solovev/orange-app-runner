package util

import (
	"errors"
	"flag"
	"fmt"
	"orange-app-runner/system"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	debug = false
	quiet = false
)

type Config struct {
	TimeLimit     duration
	MemoryLimit   memory
	RequiredLoad  rload
	IdleLimit     duration
	HomeDirectory string
	User          string
	Password      string
	ExitCode      bool
	Quiet         bool
	DisplayWindow bool
	SingleCore    bool
	Environment   env

	InputFile, OutputFile, ErrorFile, StoreFile string

	ProcessPath string
	ProcessArgs []string
	BaseName    string

	// Extended options
	AllowCreateProcesses   bool
	MultiThreadedProcess   bool
	TerminateOnFCException bool
}

type duration time.Duration

func (d *duration) String() string {
	return d.Value().String()
}

func (d *duration) Seconds() int {
	value := d.Value().Seconds()
	if value < 1 && value != 0 {
		value = 1
	}
	return int(value)
}

func (d *duration) Value() time.Duration {
	return time.Duration(*d)
}

func (d *duration) Set(value string) error {
	if _, err := strconv.Atoi(value); err == nil {
		value += "s"
	}
	v, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	*d = duration(v)

	return nil
}

type memory uint64

func (m *memory) String() string {
	return StringifyMemory(uint64(*m))
}

func (m *memory) Value() uint64 {
	return uint64(*m)
}

func (m *memory) Set(value string) error {
	if v, err := strconv.ParseUint(value, 10, 64); err == nil {
		*m = memory(v)
	} else {
		l := len(value) - 1
		v, err := strconv.ParseUint(value[:l], 10, 64)
		if err != nil {
			return err
		}
		switch value[l:] {
		case "K":
			v *= 1024
		case "M":
			v *= 1024 * 1024
		default:
			return errors.New("Unknown dimension for memory value. Type '-h' for show help message.")
		}
		*m = memory(v)
	}

	return nil
}

type rload float64

func (r *rload) String() string {
	return StringifyLoad(r.Value())
}

func (r *rload) Value() float64 {
	return float64(*r)
}

func (r *rload) Set(value string) error {
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		if v > 0 && v <= 1.0 {
			*r = rload(v)
		} else {
			return errors.New("Wrong required load value, out of range [0.0 - 1.0]? Type '-h' for show help message.")
		}
	} else {
		length := len(value) - 1
		if value[length:] == "%" {
			v, err := strconv.Atoi(value[:length])
			if err != nil {
				return err
			}
			if v > 0 && v <= 100 {
				*r = rload(float64(v) / 100.0)
			} else {
				return errors.New("Wrong required load value, out of range [0 - 100]? Type '-h' for show help message.")
			}
		} else {
			return errors.New("Wrong required load value. Type '-h' for show help message.")
		}
	}
	return nil
}

type env []string

func (e *env) String() string {
	return "Environment variables"
}

func (e *env) Set(value string) error {
	if value == "~DEBUG" {
		debug = true
		return nil
	}
	if strings.Count(value, "=") != 1 {
		return errors.New("Wrong syntax of '-D' option. Type '-h' for show help message.")
	}

	k := strings.Split(value, "=")[0]

	index := -1
	for i, v := range *e {
		if strings.HasPrefix(v, k) {
			index = i
			break
		}
	}
	if index != -1 {
		(*e)[index] = value
	} else {
		*e = append(*e, value)
	}
	return nil
}

func NewConfig() *Config {
	cfg := new(Config)
	cfg.RequiredLoad.Set("0.05")
	cfg.Environment = os.Environ()

	flag.Var(&cfg.TimeLimit, "t", "Time limit, terminate after <value> seconds,\n\tyou can add 'ms', 'm', 'h' (w/o quotes) after the number to specify.")
	flag.Var(&cfg.MemoryLimit, "m", "Memory limit, terminate if working set of the process\n\texceeds <value> bytes, you can add 'K' or 'M' to specify\n\tmemory limit in kilo- or megabytes.")
	flag.Var(&cfg.RequiredLoad, "r", "Required load of the processor for this process\n\tnot to be considered idle. You can add '%' sign to specify\n\trequired load in percent, default is 0.05 = 5%.")
	flag.Var(&cfg.IdleLimit, "y", "Idleness limit, terminate process if it did not load processor\n\tfor at least <-r option> for duration of <value>.")

	flag.StringVar(&cfg.HomeDirectory, "d", "", "Make <string> home directory for process.")
	flag.StringVar(&cfg.User, "l", system.GetCurrentUserName(), "Create process under <string> user.")
	flag.StringVar(&cfg.Password, "p", "", "Logins user using <string> password.")

	flag.StringVar(&cfg.InputFile, "i", "", "Redirects standart input stream to the <string> file.")
	flag.StringVar(&cfg.OutputFile, "o", "", "Redirects standart output stream to the <string> file.")
	flag.StringVar(&cfg.ErrorFile, "e", "", "Redirects standart error stream to the <string> file.")

	flag.BoolVar(&cfg.ExitCode, "x", false, "Return exit code of the application.")
	flag.BoolVar(&cfg.Quiet, "q", false, "Do not display any information on the screen.")
	flag.BoolVar(&cfg.DisplayWindow, "w", false, "Display program window on the screen.")
	flag.BoolVar(&cfg.SingleCore, "1", false, "Use single CPU/CPU core.")

	flag.StringVar(&cfg.StoreFile, "s", "", "Store statistics in <string> file.")
	flag.Var(&cfg.Environment, "D", "Sets value of the environment variable,\n\tcurrent environment is completely ignored in this case.")

	flag.BoolVar(&cfg.AllowCreateProcesses, "Xacp", false, "Allow the spawned process to create new processes.")
	flag.BoolVar(&cfg.MultiThreadedProcess, "Xamt", false, "Allow the spawned process to clone himself for new thread creation.\n\tRelevanted only if -Xacp is not stated.")
	flag.BoolVar(&cfg.TerminateOnFCException, "Xtfce", false, "Do not ignore exceptions if they are marked as first-chance,\n\trequired for some old compilers as Borland Delphi.")

	flag.Parse()

	idleLimit := cfg.IdleLimit.Seconds()
	if idleLimit != 0 && idleLimit < 1 {
		cfg.IdleLimit.Set("1s")
	}

	args := flag.Args()
	if len(args) > 0 {
		cfg.ProcessPath = args[0]
	}
	if len(args) > 1 {
		cfg.ProcessArgs = args[1:]
	}

	cfg.BaseName = GetProcessBaseName(cfg.ProcessPath)

	/*
	*	Adding "./" before process path, if its not a system command
	 */
	if cfg.ProcessPath == cfg.BaseName {
		_, err := exec.LookPath(cfg.ProcessPath)
		if err != nil {
			cfg.ProcessPath = fmt.Sprintf("%s%s", "./", cfg.ProcessPath)
			Debug("Prefix \"./\" was added to %s", cfg.BaseName)
		}
	}

	if cfg.Quiet {
		cfg.DisplayWindow = false
	}

	// debug = true // TODO'0
	quiet = cfg.Quiet

	return cfg
}
