package system

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

func GetCurrentUserName() string {
	return os.Getenv("USER")
}

func IsCurrentUserRoot() bool {
	return os.Getuid() == 0 /* && os.Getgid() == 0 */
}

func FindUser(name string) (uint32, uint32, error) {
	data, err := ioutil.ReadFile("/etc/passwd")
	if err != nil {
		return 0, 0, err
	}
	reader := bufio.NewReader(bytes.NewBuffer(data))
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}

		split := strings.Split(string(line), ":")
		username := split[0]
		uid, err := strconv.Atoi(split[2])
		if err != nil {
			return 0, 0, err
		}
		gid, err := strconv.Atoi(split[3])
		if err != nil {
			return 0, 0, err
		}

		if name == username {
			return uint32(uid), uint32(gid), nil
		}
	}
	return 0, 0, fmt.Errorf("Unknown user: %s", name)
}
