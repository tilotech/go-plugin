package plugin_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tilotech/go-plugin"
)

func TestIntegration(t *testing.T) {
	fixture, term, err := Connect(
		plugin.StartWithProvider(Provide(&fakeImpl{})),
		&plugin.Config{
			Timeout:        2 * time.Second,
			ConnectTimeout: 2 * time.Second,
			KeepAlive:      2 * time.Second,
		},
	)
	require.NoError(t, err)
	defer term()

	actual, err := fixture.Simple("valid value")
	assert.NoError(t, err)
	assert.Equal(t, "valid value from impl", actual)
	actual, err = fixture.Simple("force error")
	assert.Error(t, err)
	assert.Equal(t, "", actual)

	actual, err = fixture.Simple("force invalid request")
	assert.Error(t, err)
	assert.Equal(t, "", actual)

	actual, err = fixture.Simple("force request marshal error")
	assert.Error(t, err)
	assert.Equal(t, "", actual)

	actual, err = fixture.Simple("force wrong method")
	assert.Error(t, err)
	assert.Equal(t, "", actual)

	actual, err = fixture.Simple("force 1 second delay")
	assert.NoError(t, err)
	assert.Equal(t, "forced 1 second delay in impl", actual)

	startTime := time.Now()
	actual, err = fixture.Simple("force 3 second delay")
	assert.Error(t, err)
	assert.Equal(t, "", actual)
	duration := time.Since(startTime)
	assert.Less(t, duration, 3*time.Second)

	actual, err = fixture.Simple("force provider panic")
	assert.Error(t, err)
	assert.Equal(t, "", actual)

	actual, err = fixture.Simple("valid value")
	assert.NoError(t, err)
	assert.Equal(t, "valid value from impl", actual)

	ab, c, hasDeadline, err := fixture.Complex(context.Background(), 1, 2, []int{3, 4, 5})
	assert.NoError(t, err)
	assert.Equal(t, 3, ab)
	assert.Equal(t, 12, c)
	assert.False(t, hasDeadline)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ab, c, hasDeadline, err = fixture.Complex(ctx, 1, 2, []int{3, 4, 5})
	assert.NoError(t, err)
	assert.Equal(t, 3, ab)
	assert.Equal(t, 12, c)
	assert.True(t, hasDeadline)
}

func TestIntegrationWithBrokenPlugin(t *testing.T) {
	fixture, term, err := Connect(
		plugin.StartWithProvider(Provide(&fakeImpl{})),
		&plugin.Config{
			Timeout:        2 * time.Second,
			ConnectTimeout: 2 * time.Second,
			KeepAlive:      2 * time.Second,
		},
	)
	require.NoError(t, err)
	defer term()

	actual, err := fixture.Simple("valid value")
	assert.NoError(t, err)
	assert.Equal(t, "valid value from impl", actual)

	term()
	term()

	actual, err = fixture.Simple("valid value")
	assert.NoError(t, err)
	assert.Equal(t, "valid value from impl", actual)
}

type Fake interface {
	Simple(s string) (string, error)
	Complex(ctx context.Context, a, b int, c []int) (ab int, cSum int, hasDeadline bool, err error)
}

type fakeImpl struct{}

func (d *fakeImpl) Simple(s string) (string, error) {
	if s == "force error" {
		return "", fmt.Errorf("returned error due to %v request", s)
	}
	if s == "force 1 second delay" {
		time.Sleep(1 * time.Second)
		return "forced 1 second delay in impl", nil
	}
	if s == "force 3 second delay" {
		time.Sleep(3 * time.Second)
		return "forced 3 second delay in impl", nil
	}
	if s == "force provider panic" {
		panic("forced panic")
	}
	return fmt.Sprintf("%v from impl", s), nil
}

func (d *fakeImpl) Complex(ctx context.Context, a, b int, c []int) (int, int, bool, error) {
	cSum := 0
	for _, v := range c {
		cSum += v
	}
	_, hasDeadline := ctx.Deadline()
	return a + b, cSum, hasDeadline, nil
}

func Connect(starter plugin.Starter, config *plugin.Config) (Fake, plugin.TermFunc, error) {
	rand.Seed(time.Now().UnixNano())
	client, term, err := plugin.Start(starter, fmt.Sprintf("%v/fake%v", os.TempDir(), rand.Intn(100000)), config)
	if err != nil {
		return nil, nil, err
	}
	return &proxy{
		client: client,
	}, term, nil
}

type fakeComplexRequest struct {
	A, B int
	C    []int
}

type fakeComplexResponse struct {
	AB          int
	CSum        int
	HasDeadline bool
}

type proxy struct {
	client *plugin.Client
}

func (p *proxy) Simple(s string) (string, error) {
	var request any
	request = s
	response := ""
	method := "/simple"

	if s == "force invalid request" {
		request = []string{"this", "is", "not", "valid"}
	}
	if s == "force request marshal error" {
		request = make(chan string, 1) // channels cannot be marshaled
	}
	if s == "force wrong method" {
		method = "/this-is-not-valid"
	}

	err := p.client.Call(context.Background(), method, request, &response)
	return response, err
}

func (p *proxy) Complex(ctx context.Context, a, b int, c []int) (int, int, bool, error) {
	request := &fakeComplexRequest{
		A: a,
		B: b,
		C: c,
	}
	response := &fakeComplexResponse{}
	err := p.client.Call(ctx, "/complex", request, &response)
	if err != nil {
		return 0, 0, false, err
	}
	return response.AB, response.CSum, response.HasDeadline, nil
}

func Provide(impl Fake) plugin.Provider {
	return &provider{
		impl: impl,
	}
}

type provider struct {
	impl Fake
}

func (p *provider) Provide(method string) (plugin.RequestParameter, plugin.InvokeFunc, error) {
	switch method {
	case "/simple":
		request := ""
		return &request, p.Simple, nil
	case "/complex":
		return &fakeComplexRequest{}, p.Complex, nil
	}

	return nil, nil, fmt.Errorf("invalid method %v", method)
}

func (p *provider) Simple(_ context.Context, params plugin.RequestParameter) (any, error) {
	s := params.(*string)
	return p.impl.Simple(*s)
}

func (p *provider) Complex(ctx context.Context, params plugin.RequestParameter) (any, error) {
	typedParams := params.(*fakeComplexRequest)
	ab, cSum, hasDeadline, err := p.impl.Complex(ctx, typedParams.A, typedParams.B, typedParams.C)
	if err != nil {
		return nil, err
	}
	return &fakeComplexResponse{
		AB:          ab,
		CSum:        cSum,
		HasDeadline: hasDeadline,
	}, nil
}
