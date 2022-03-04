package plugin

import (
	"fmt"
	"io"
	"os"
)

// StartWithProvider returns a Starter that will directly start the plugin using
// the Provider.
//
// This is especially helpful for, but not limitted to, testing plugins.
func StartWithProvider(provider Provider) Starter {
	return &providerStarter{
		provider: provider,
	}
}

type providerStarter struct {
	provider Provider
}

func (p *providerStarter) Start(socket string, exited chan<- struct{}, ready chan<- struct{}) (TermFunc, error) {
	cancel := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	term := func() error {
		cancel <- struct{}{}
		// wait until it is actually cancelled
		<-cancelled
		return nil
	}

	// capture and mirror everything written to stdout
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	originalStdOut := os.Stdout
	os.Stdout = w
	teeReader := io.TeeReader(r, originalStdOut)

	go func() {
		_ = os.Setenv("PLUGIN_SOCKET", socket)
		err := listenAndServe(p.provider, cancel, cancelled)
		if err != nil {
			fmt.Println(err)
		}
	}()

	go waitForServer(teeReader, ready)

	return term, nil
}
