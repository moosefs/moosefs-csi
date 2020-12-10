package driver

import (
	"os"

	"github.com/sirupsen/logrus"
)

type MfsLog struct {
	active  bool
	logfile string
}

func MakeMfsLog(logfile string, active bool) *MfsLog {
	if active {
		logrus.Infof("MfsLog Active!")
	}
	return &MfsLog{logfile: logfile, active: active}
}

func (log *MfsLog) Log(msg string) {
	if !log.active {
		return
	}
	f, err := os.OpenFile(log.logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logrus.Error(err.Error())
		return
	}
	defer f.Close()
	if _, err := f.WriteString(msg + "\n"); err != nil {
		logrus.Error(err.Error())
	}
}
