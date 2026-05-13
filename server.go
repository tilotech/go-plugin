package plugin

import (
	"context"
)

// Provider defines how to map a method call to the actual implementation.
//
// If the given method is unknown to the plugin provider, then an error MUST be
// returned.
// Otherwise a RequestParameter and an InvokeFunc MUST be returned.
// The RequestParameter MUST be a non-nil pointer with default values that
// defines how to unmarshal the request body.
// The InvokeFunc is a callback that will be called after the unmarshal of the
// RequestParameter was successful and should forward the call to the actual
// implementation. The InvokeFunc will be called with the same instance of the
// RequestParameter.
type Provider interface {
	Provide(method string) (RequestParameter, InvokeFunc, error)
}

// RequestParameter is an alias on an empty interface.
//
// Any non-nil pointer value can be used. See also Provider.
type RequestParameter any

// InvokeFunc defines a callback that is invoked with the unmarshaled request
// parameter.
//
// It must either return a valid response or an error. The response must follow
// the same data structure that is expected from the Proxy. In order to return
// multiple values, they must be wrapped in a single response object.
type InvokeFunc func(ctx context.Context, params RequestParameter) (response any, err error)

const pluginIsReadyMsg = "plugin is ready"
const pluginListenFailedMsg = "failed to listen on socket"

// ListenAndServe starts a simple HTTP server on a unix socket and will wait
// for incoming requests.
//
// The path for the unix socket MUST be provided using the environment variable
// PLUGIN_SOCKET.
//
// If one of the following system signals was received, the server will stop
// listening and return from the function:
// SIGINT, SIGTERM or SIGPIPE
//
// It is guaranteed that the server has been fully stopped in the moment the
// ListenAndServe method returns.
func ListenAndServe(provider Provider) error {
	cancel := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	defer func() {
		// block until the server has been fully stopped
		<-cancelled
	}()
	return listenAndServe(provider, cancel, cancelled)
}
