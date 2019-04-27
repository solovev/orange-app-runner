package instance

import (
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/solovev/orange-app-runner/system"
)

func setAffinity(pid int, cfg *Config) error {
	if len(cfg.Affinity) > 0 {
		set, err := system.SetAffinity(cfg.Affinity, pid)
		if err != nil {
			return err
		}
		log.Debugf("Processor affinity was set to: %v\n", set)
	}
	return nil
}

func wait(pid int) (cpid int, status syscall.WaitStatus, err error) {
	cpid, err = syscall.Wait4(pid, &status, syscall.WALL, nil)
	if err != nil {
		return 0, 0, err
	}
	return cpid, status, nil
}
