/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
	"io/ioutil"
	"syscall"

	"github.com/golang/glog"
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

func GetNumberOfFreeAndAvailableBlocks(path string) (uint64, uint64, error) {
	var st syscall.Statfs_t
	err := syscall.Statfs(path, &st)
	if err != nil {
		glog.Errorf("Error calling statfs() on %s: %v", path, err)
		return uint64(0), uint64(0), err
	}
	return st.Bfree, st.Bavail, nil
}
