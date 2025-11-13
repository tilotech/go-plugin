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
//
// The Starter maintains a per-socket mapping of *exec.Cmd instances to allow
// reuse of existing plugin processes. If Start is called multiple times with
// the same socket, it will attempt to reuse the already running plugin rather
// than starting a new process. If the original plugin process is no longer
// running, the socket is removed before starting a new process.
//
// Limitations and caveats:
//   - The reuse mechanism relies on the ProcessState of the stored *exec.Cmd,
//     which may not always accurately reflect the process status on Linux due
//     to timing and PID reuse.
//   - If multiple instances of Starter exist in the same process, or if multiple
//     processes attempt to start a plugin on the same socket concurrently, the
//     socket may be removed and recreated, potentially leaving the original
//     plugin in an inconsistent or faulty state.
//   - Plugin processes are terminated if the parent process exits, assuming
//     the TermFunc has been retained and invoked.
func StartWithCmd(cmdProvider func() *exec.Cmd) Starter {
	return &cmdStarter{
		cmdProvider: cmdProvider,
		socketCmds:  make(map[string]*exec.Cmd),
	}
}

type cmdStarter struct {
	cmdProvider func() *exec.Cmd
	socketCmds  map[string]*exec.Cmd
}

func (c *cmdStarter) Start(socket string, failed chan<- struct{}, ready chan<- struct{}) (TermFunc, error) {
	if term := c.tryReuse(socket, ready); term != nil {
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

	c.socketCmds[socket] = cmd

	term := func() error {
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err == os.ErrProcessDone {
			return nil
		}
		return err
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

func (c *cmdStarter) tryReuse(socket string, ready chan<- struct{}) TermFunc {
	cmd, ok := c.socketCmds[socket]
	if !ok || cmd.ProcessState.Exited() {
		// the process does not exist (never started, plugin terminated or parent and plugin terminated)
		// remove socket if it exists
		_ = os.Remove(socket)
		return nil
	}
	// the process probably still exists (not guaranteed due to false ProcessState.Exited reporting on linux)
	if c.isProcessRunning(cmd) {
		term := func() error {
			err := cmd.Process.Signal(syscall.SIGTERM)
			if err == os.ErrProcessDone {
				return nil
			}
			return err
		}

		// mark server as ready
		go func() {
			ready <- struct{}{}
		}()

		return term
	}

	// process actually no longer running
	// remove socket if it exists
	_ = os.Remove(socket)
	return nil
}

func (c *cmdStarter) isProcessRunning(cmd *exec.Cmd) bool {
	err := cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}
