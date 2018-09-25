package util

import "io/ioutil"

func Minint64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func DisableOOMKiller() error {
	val := []byte("-1000\n")
	return ioutil.WriteFile("/proc/self/oom_score_adj", val, 0644)
}
