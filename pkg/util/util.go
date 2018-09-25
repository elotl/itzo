package util

import (
	"fmt"
	"io/ioutil"
)

func Minint64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// We disable the OOM Killer for Itzo in the itzo startup script
// but we want to reenable it for user processes that itzo spawns
func SetOOMScore(pid, score int) error {
	writebytes := []byte(fmt.Sprintf("%d\n", score))
	filepath := fmt.Sprintf("/proc/%d/oom_score_adj", pid)
	return ioutil.WriteFile(filepath, writebytes, 0644)
}
