package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// Provider defines how to map a method call to the actual implementation.
//
// If the given method is unknown to the plugin provider, then an error MUST be
// returned.
// Otherwise a RequestParameter and an InvokeFunc MUST be returned.
// The RequestParameter MUST be a non-nil pointer with default values that
// defines how to unmarshal the request body.
// The InvokeFunc is a callback that will be called after the unmarshal of the
// RequestParameter was successfull and should forward the call to the actual
// implementation. The InvokeFunc will be called with the same instance of the
// RequestParameter.
type Provider interface {
	Provide(method string) (RequestParameter, InvokeFunc, error)
}

// RequestParameter is an alias on an empty interface.
//
// Any non-nil pointer value can be used. See also Provider.
type RequestParameter interface{}

// InvokeFunc defines a callback that is invoked with the unmarshaled request
// parameter.
//
// It must either return a valid response or an error. The response must follow
// the same data structure that is expected from the Proxy. In order to return
// multiple values, they must be wrapped in a single response object.
type InvokeFunc func(ctx context.Context, params RequestParameter) (response interface{}, err error)

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

// listenAndServe starts a simple HTTP server on a unix socket and will wait
// for incoming requests.
//
// The path for the unix socket MUST be provided using the environment variable
// PLUGIN_SOCKET.
//
// listenAndServe is blocking until a message in the cancel channel has been
// received. After the server was cancelled, a single message is written into
// the cancelled channel. Both cancel and cancelled MUST be buffered channels
// with a capacity of at least 1.
//
// If one of the following system signals was received, the server will also
// stop listening and a cancelled message will be written:
// SIGINT, SIGTERM or SIGPIPE
//
// If the server could not be started an error is returned.
func listenAndServe(provider Provider, cancel <-chan struct{}, cancelled chan<- struct{}) error {
	socket := os.Getenv("PLUGIN_SOCKET")
	if socket == "" {
		return fmt.Errorf("no socket provided, please provide one using the PLUGIN_SOCKET environment variable")
	}

	var listener net.Listener
	var httpServer *http.Server

	// handle cancel signals
	sigCancel := make(chan os.Signal, 1)
	signal.Notify(sigCancel, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-cancel:
		case <-sigCancel:
		}
		if listener != nil {
			err := listener.Close()
			if err != nil {
				fmt.Println(err)
			}
		}
		if httpServer != nil {
			err := httpServer.Close()
			if err != nil {
				fmt.Println(err)
			}
		}
		cancelled <- struct{}{}
	}()

	// ensure that the unix socket can only be accessed by the current user
	syscall.Umask(0077)

	var err error
	listener, err = net.Listen("unix", socket)
	if err != nil {
		fmt.Println(pluginListenFailedMsg)
		return err
	}

	// signal the client that the server is ready
	fmt.Println(pluginIsReadyMsg)

	httpServer = &http.Server{
		Handler: &httpHandler{
			provider: provider,
		},
	}

	return httpServer.Serve(listener)
}

type httpHandler struct {
	provider Provider
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	responseIsSent := false

	defer func() {
		if responseIsSent {
			return
		}
		r := recover()
		if r != nil {
			h.sendErrorResponse(http.StatusInternalServerError, fmt.Errorf("%v", r), w)
		}
	}()
	method := req.URL.Path

	params, invoke, err := h.provider.Provide(method)
	if err != nil {
		h.sendErrorResponse(http.StatusNotFound, err, w)
		responseIsSent = true
		return
	}

	request := &requestWithContext{
		Payload: params, // assign params to ensure the decoder knows into which type to unmarshal
	}
	err = h.decode(req.Body, request)
	if err != nil {
		h.sendErrorResponse(http.StatusUnprocessableEntity, err, w)
		responseIsSent = true
		return
	}

	ctx, cancel := restoreContext(req.Context(), request.Context)
	defer cancel()
	response, err := invoke(ctx, request.Payload)
	if err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, err, w)
		responseIsSent = true
		return
	}

	h.sendResponse(http.StatusOK, response, w)
	responseIsSent = true
}

func (h *httpHandler) decode(requestBody io.ReadCloser, params interface{}) error {
	decoder := json.NewDecoder(requestBody)
	err := decoder.Decode(params)
	if err != nil {
		_ = requestBody.Close()
		return err
	}
	return requestBody.Close()
}

func (h *httpHandler) sendResponse(statusCode int, response interface{}, w http.ResponseWriter) {
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	err := encoder.Encode(response)
	if err != nil {
		// everything is too late, only log the error
		fmt.Println(err)
	}
}

func (h *httpHandler) sendErrorResponse(statusCode int, err error, w http.ResponseWriter) {
	h.sendResponse(statusCode, err.Error(), w)
}

func restoreContext(ctx context.Context, rCtx requestContext) (context.Context, context.CancelFunc) {
	if rCtx.Deadline != nil {
		return context.WithDeadline(ctx, *rCtx.Deadline)
	}
	return ctx, func() {}
}
