package plugin

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// StartWithCmd returns a Starter that will start the plugin using the given
// executable command.
//
// Its TermFunc will terminate the plugin by sending a SIGTERM signal to the
// started process.
func StartWithCmd(cmd *exec.Cmd) Starter {
	return &cmdStarter{
		cmd: cmd,
	}
}

type cmdStarter struct {
	cmd *exec.Cmd
}

func (c *cmdStarter) Start(socket string, exited chan<- struct{}, ready chan<- struct{}) (TermFunc, error) {
	r, w := io.Pipe()
	c.cmd.Stdout = io.MultiWriter(os.Stdout, w)
	c.cmd.Stderr = os.Stderr
	c.cmd.Env = append(c.cmd.Env, os.Environ()...)
	c.cmd.Env = append(c.cmd.Env, fmt.Sprintf("PLUGIN_SOCKET=%v", socket))

	err := c.cmd.Start()
	if err != nil {
		return nil, err
	}

	term := func() error {
		return c.cmd.Process.Signal(syscall.SIGTERM)
	}

	go func() {
		err := c.cmd.Wait()
		if err != nil {
			fmt.Println(err)
		}
		exited <- struct{}{}
	}()

	go waitForServer(r, ready)

	return term, nil
}
