package plugin

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
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

func (c *cmdStarter) Start(socket string, failed chan<- struct{}, ready chan<- struct{}) (TermFunc, error) {
	pidFile := fmt.Sprintf("%v.pid", socket)

	if term := c.tryReuse(socket, pidFile, ready); term != nil {
		return term, nil
	}

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
		// send terminate signal and remove pid file
		// only the signal error is relevant
		err := cmd.Process.Signal(syscall.SIGTERM)
		_ = os.Remove(pidFile)
		return err
	}

	err = os.WriteFile(pidFile, []byte(fmt.Sprintf("%v", cmd.Process.Pid)), 0600)
	if err != nil {
		_ = term()
		return nil, err
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			fmt.Println(err)
		}
		failed <- struct{}{}
	}()

	go waitForServer(r, failed, ready)

	return term, nil
}

func (c *cmdStarter) tryReuse(socket string, pidFile string, ready chan<- struct{}) TermFunc {
	pidBB, err := ioutil.ReadFile(pidFile) // nolint:gosec
	if err != nil {
		// the process probably does not exist
		return nil
	}
	// the process probably still exists
	pid, err := strconv.Atoi(string(pidBB))
	if err != nil {
		return nil
	}

	if c.isProcessRunning(pid) {
		term := func() error {
			err := syscall.Kill(pid, syscall.SIGTERM)
			_ = os.Remove(pidFile)
			return err
		}

		// mark server as ready
		go func() {
			ready <- struct{}{}
		}()

		return term
	}

	// process actually no longer running
	// remove socket and pid file if they exists
	_ = os.Remove(pidFile)
	_ = os.Remove(socket)
	return nil
}

func (c *cmdStarter) isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
