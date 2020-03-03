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

package prober

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type CommandRunner interface {
	RunWithTimeout(cmd []string, timeout time.Duration) ([]byte, error)
}

type ExecRunner struct {
}

func (r *ExecRunner) RunWithTimeout(cmd []string, timeout time.Duration) ([]byte, error) {
	var cancel context.CancelFunc
	ctx := context.Background()
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}

	// Todo: we might want to limit the number of bytes we read here
	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Env = os.Environ()
	out, err := command.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return []byte("Command timed out"), fmt.Errorf("Command timed out")
	}
	return out, err
}
