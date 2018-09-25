package util

import (
	"fmt"
	"io/ioutil"
	"os"
)

func Minint64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func SetOOMScore(pid, score int) error {
	writebytes := []byte(fmt.Sprintf("%d\n", score))
	if pid == 0 {
		pid = os.Getpid()
	}
	filepath := fmt.Sprintf("/proc/%d/oom_score_adj", pid)
	return ioutil.WriteFile(filepath, writebytes, 0644)
}
