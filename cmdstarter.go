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
func StartWithCmd(cmdProvider func() *exec.Cmd) Starter {
	return &cmdStarter{
		cmdProvider: cmdProvider,
	}
}

type cmdStarter struct {
	cmdProvider func() *exec.Cmd
}

func (c *cmdStarter) Start(socket string, exited chan<- struct{}, ready chan<- struct{}) (TermFunc, error) {
	r, w := io.Pipe()
	cmd := c.cmdProvider()
	cmd.Stdout = io.MultiWriter(os.Stdout, w)
	cmd.Stderr = os.Stderr
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PLUGIN_SOCKET=%v", socket))

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	term := func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			fmt.Println(err)
		}
		exited <- struct{}{}
	}()

	go waitForServer(r, ready)

	return term, nil
}
