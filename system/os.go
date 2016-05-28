package system

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

func SetCPUAffinity(pid int) error {
	cpuIndex, err := getReliableCPU()
	if err != nil {
		return err
	}
	var mask [1024 / 64]uintptr
	mask[cpuIndex/64] |= 1 << (cpuIndex % 64)

	_, _, err1 := syscall.RawSyscall(syscall.SYS_SCHED_SETAFFINITY, uintptr(pid),
		uintptr(len(mask)*8),
		uintptr(unsafe.Pointer(&mask[0])))

	if err1 != 0 {
		return err
	}
	return nil
}

func getReliableCPU() (uint, error) {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	reader := bufio.NewReader(bytes.NewBuffer(data))
	result, max := 0, 0
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("Unable to read \"/proc/stat\" file: %v", err)
		}

		str := string(line)
		if strings.Index(str, "cpu") != 0 {
			break
		}

		sign := ""
		user, nice, system, idle := 0, 0, 0, 0
		_, err = fmt.Sscanf(str, "%s %d %d %d %d", &sign, &user, &nice, &system, &idle)
		total := user + nice + system + idle

		if len(sign) == 4 {
			if total < max {
				max = total
				result, err = strconv.Atoi(sign[3:])
				if err != nil {
					return 0, fmt.Errorf("Unable to parse CPU's index (%s): %v", sign[3:], err)
				}
			}
		} else {
			max = total
		}
	}
	return uint(result), nil
}

func GetTotalCPUTime() (uint64, error) {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	reader := bufio.NewReader(bytes.NewBuffer(data))
	line, _, err := reader.ReadLine()
	if err != nil {
		return 0, fmt.Errorf("Unable to read \"/proc/stat\" file: %v", err)
	}

	sign := ""
	var user, nice, system, idle uint64
	_, err = fmt.Sscanf(string(line), "%s %d %d %d %d", &sign, &user, &nice, &system, &idle)
	if sign != "cpu" {
		return 0, errors.New("Unable to get total cpu time from \"/proc/stat\" file")
	}

	return user + nice + system + idle, nil
}

func GetProcessStats(pid int) (uint64, uint64, error) {
	path := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	stats := strings.Fields(string(data))

	utime, err := strconv.ParseUint(stats[13], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("Unable to parse \"utime\" from \"%s\" file: %v", path, err)
	}
	stime, err := strconv.ParseUint(stats[14], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("Unable to parse \"stime\" from \"%s\" file: %v", path, err)
	}
	vsize, err := strconv.ParseUint(stats[22], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("Unable to parse \"vtime\" from \"%s\" file: %v", path, err)
	}
	return stime + utime, vsize, nil
}

func GetProcessCommand(pid int) string {
	path := "/proc/" + strconv.Itoa(pid) + "/cmdline"
	cmdline, err := ioutil.ReadFile(path)
	for b := range cmdline {
		if b <= (len(cmdline) - 1) {
			if cmdline[b] == 0x00 {
				cmdline[b] = 0x20
			}
		}
	}
	if err != nil {
		return "-"
	}
	return string(cmdline)
}

func Exit(code int) {
	runtime.UnlockOSThread()
	os.Exit(code)
}

func GetCPUCount() (int, error) {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	reader := bufio.NewReader(bytes.NewBuffer(data))
	number := -1
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		str := string(line)
		attr := strings.SplitN(str, " ", 1)
		if strings.Index(attr[0], "cpu") == 0 {
			number++
		} else {
			break
		}
	}
	return number, nil
}