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
