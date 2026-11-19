package plugin

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
)

// Starter defines how to start a plugin.
//
// The socket defines the unix socket (usually /tmp/something) where to listen
// to incoming requests.
// The failed channel can be used to indicate that the plugin did not start
// properly.
// The ready channel can be used to indicate that the plugin is ready to receive
// requests.
// Either the exited or the ready channel MUST send an empty struct, but not
// both.
//
// If the plugin was started (or the start was initialized) it MUST return a
// TermFunc which can be used to shutdown the plugin. Otherwise an error MUST be
// returned.
type Starter interface {
	Start(socket string, failed chan<- struct{}, ready chan<- struct{}) (TermFunc, error)
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
	httpClient := http.Client{}
	httpClient.Timeout = config.Timeout
	httpClient.Transport = &http.Transport{
		DialContext: (&unixDialer{
			Dialer: net.Dialer{
				Timeout:   config.ConnectTimeout,
				KeepAlive: config.KeepAlive,
			},
			socket: socket,
		}).DialContext,
	}

	client = &Client{
		url:        "http://plugin", // plugin could by anything since the custom dialer ignores the value
		httpClient: httpClient,
		starter:    starter,
		socket:     socket,
	}

	err = client.startPlugin()
	if err != nil {
		return nil, nil, err
	}

	return client, client.terminatePlugin, nil
}

type unixDialer struct {
	net.Dialer
	socket string
}

func (d *unixDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	return d.Dialer.DialContext(ctx, "unix", d.socket)
}

func waitForServer(r io.Reader, failed chan<- struct{}, ready chan<- struct{}) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == pluginIsReadyMsg {
			ready <- struct{}{}
			break
		}
		if line == pluginListenFailedMsg {
			failed <- struct{}{}
		}
	}

	go func() {
		for scanner.Scan() { //nolint:revive
			// read everything to prevent blocking the stdout
		}
	}()
}
