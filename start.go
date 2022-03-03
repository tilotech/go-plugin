package plugin

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Starter defines how to start a plugin.
//
// The socket defines the unix socket (usually /tmp/something) where to listen
// to incoming requests.
// The exited channel can be used to indicate that the plugin as been shutdown
// already.
// The ready channel can be used to indicate that the plugin is ready to receive
// requests.
// Either the exited or the ready channel MUST send an empty struct, but not
// both.
//
// If the plugin was started (or the start was initialized) it MUST return a
// TermFunc which can be used to shutdown the plugin. Otherwise an error MUST be
// returned.
type Starter interface {
	Start(socket string, exited chan<- struct{}, ready chan<- struct{}) (TermFunc, error)
}

// TermFunc can be used to terminate a previously started plugin.
//
// Typically the TermFunc will be deferred right after the plugin was started.
type TermFunc func() error

// Start starts the plugin using the given Starter on the provided socket and
// provides you with a client that can be used during proxying requests to the
// plugin.
//
// You can provide additional configuration using the config.
//
// Start is blocking until the plugin is ready to receive requests, has been
// terminated or the connect timeout has been reached.
func Start(starter Starter, socket string, config *Config) (client *Client, terminate TermFunc, err error) {
	exited := make(chan struct{}, 1)
	ready := make(chan struct{}, 1)
	term, err := starter.Start(socket, exited, ready)

	defer func() {
		r := recover()
		if err != nil || r != nil {
			_ = term()
		}
		if r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	timeout := time.After(10 * time.Second)

	select {
	case <-timeout:
		return nil, nil, fmt.Errorf("could not start plugin within 10 seconds")
	case <-exited:
		return nil, nil, fmt.Errorf("plugin failed to start")
	case <-ready:
	}

	httpClient := http.Client{}
	httpClient.Timeout = config.Timeout
	httpClient.Transport = &http.Transport{
		Dial: (&unixDialer{
			Dialer: net.Dialer{
				Timeout:   config.ConnectTimeout,
				KeepAlive: config.KeepAlive,
			},
			socket: socket,
		}).Dial,
	}

	return &Client{
		url:        "http://plugin", // plugin could by anything since the custom dialer ignores the value
		httpClient: httpClient,
	}, term, nil
}

type unixDialer struct {
	net.Dialer
	socket string
}

func (d *unixDialer) Dial(_, _ string) (net.Conn, error) {
	return d.Dialer.Dial("unix", d.socket)
}

func waitForServer(r io.Reader, ready chan<- struct{}) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == pluginIsReadyMsg {
			ready <- struct{}{}
			break
		}
	}

	go func() {
		for scanner.Scan() {
			// read everything to prevent blocking the stdout
		}
	}()
}
