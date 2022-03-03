package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// Client represents a configured plugin client that knows how to communicate
// with the plugin server.
type Client struct {
	url        string
	httpClient http.Client
}

// Call will send an HTTP request to the plugin server with the provided method
// and request parameters.
//
// The request can be of any type as long as the server understands how to
// unmarshal the result.
//
// The response SHOULD be a non-nil pointer into which the servers response can
// be unmarshaled to. Exceptions occur when not expecting a response value.
//
// The error contains either a communication error or the error from the invoked
// method on the plugin.
func (c *Client) Call(ctx context.Context, method string, request, response interface{}) error {
	j, err := json.Marshal(requestWithContext{
		Context: createRequestContext(ctx),
		Payload: request,
	})
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.url+method, "application/json", bytes.NewBuffer(j))
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := ""
		err := c.decode(resp.Body, &errMsg)
		if err != nil {
			return err
		}
		return errors.New(errMsg)
	}

	// decode and assign the response or return an error if it cannot be decoded
	return c.decode(resp.Body, response)
}

func (c *Client) decode(responseBody io.ReadCloser, response interface{}) error {
	decoder := json.NewDecoder(responseBody)
	err := decoder.Decode(response)
	if err != nil {
		_ = responseBody.Close()
		return err
	}
	return responseBody.Close()
}

type requestContext struct {
	Deadline *time.Time
}

type requestWithContext struct {
	Context requestContext
	Payload interface{}
}

func createRequestContext(ctx context.Context) requestContext {
	rc := requestContext{}
	if deadline, ok := ctx.Deadline(); ok {
		rc.Deadline = &deadline
	}
	return rc
}
