# go-plugin

A simple and fast plugin library for golang based on http over unix socket.

The biggest advantage of the library is, that you don't need to worry about
reconnecting to the plugin if a connection is lost. It will automatically handle
that for you, which makes it ideal for environments where you might not have so
many control over, e.g. serverless functions like AWS Lambda.

## How to use it?

Create your interface:

```go
package api

type Modifier interface{
  Modify(s string) (string, error)
}
```

We just defined a simple interface accepting a string and returning a string and
an error. Maybe an implementation for `Modify` could be a simple string
decorator. We'll figure that out later.

Also don't worry, the example works with just a single parameter, but multiple
ones are also possible.

Next we need a plugin proxy. The proxy will implement the `Modifier` interface,
but will forward all calls to the plugin implementation via http over unix
sockets.

```go
package api

import (
  "context"
  "fmt"
  "os"

  "github.com/tilotech/go-plugin"
)

// The proxy is not exported in this example to show that this is indeed just
// implementing the Modifier interface. See the Connect function further down
// to see how it is used.
type proxy struct {
  client *plugin.Client
}

const modifyMethod = "/modify"

func (p *proxy) Modify(s string) (string, error) {
  // define response structure to automatically unmarshal the response
  response := ""
  // you can also use an existing context if your interface accepts it
  ctx := context.Background()
  // make sure to provide a pointer to the response, otherwise it will not be
  // modified
  err := p.client.Call(ctx, modifyMethod, s, &response)
  return *response, err
}

// Connect will be used by the plugin consumer for initializing the plugin.
func Connect(starter plugin.Starter, config *pluginConfig) (Modifier, plugin.TermFunc, error) {
  // we have chosen a static path to the socket here (/tmp/modify), but you can also use a random one
  client, term, err := plugin.Start(starter, fmt.Sprintf("%v/modify", os.TempDir()), config)
  if err != nil {
    return nil, nil, err
  }
  return &proxy{
    client: client,
  }, term, nil
}
```

The `Connect` function will later be the entry point for using any plugin. The
starter that needs to be provided comes currently in two flavours: a starter
that will run an executable (intended for the actual plugin) and one that works
directly with a plugin provider (intended for usage from within tests).

Now we need to create the plugin provider. The plugin provider helps to create
the actual plugin later. However, while we recommend the usage of a plugin
provider, you don't need to create one if the plugins will not be written in go.
It's slightly more difficult in that case though.

```go
package api

import (
  "context"
  "fmt"

  "github.com/tilotech/go-plugin"
)

// The provider also is not exported to show which interface it actually implements.
type provider struct {
  impl Modifier
}

func (p *provider) Provide(method string) (plugin.RequestParameter, plugin.InvokeFunc, error) {
  switch method {
  case modifyMethod:
    request := ""
    return &request, p.Modify, nil
  // add further methods here
  }
  return nil, nil, fmt.Errorf("invalid method %v", method)
}

// Modify will be invoked once the request has been unmarshaled.
func (p *provider) Modify(_ context.Context, params plugin.RequestParameter) (interface{}, error) {
  // It is guaranteed, that params is of the same type as the first return value
  // from the Provide method. Asserting the type can safely be done without
  // worrying about a panic.
  s := params.(*string)
  return p.impl.Modify(*s)
}

// Provide can be used by the plugin author to create the server.
func Provide(impl Modifier) plugin.Provider {
  return &provider{
    impl: impl,
  }
}
```

Now we have everything ready to create the actual plugin. This is the only part
the plugin authors have to take care of.

Note, that this should be in a different package if you want to provide this as
a binary.

```go
package main

import (
  "fmt"

  "github.com/tilotech/go-plugin"
  "myproject/api"
)

func main() {
  err := plugin.ListenAndServe(api.Provide(&decorateModifierPlugin{}))
  if err != nil {
    fmt.Println(err)
  }
}

type decorateModifierPlugin struct {}

func (d *decorateModifierPlugin) Modify(s string) (string, error) {
  return s + " decorated"
}
```

You can then compile the plugin using `go build main.go`.

Using the plugin is also simple.

```go
modifier, term, err := api.Connect(plugin.StartWithCmd(
  func() *exec.Cmd {
    return exec.Command("path/to/plugin")
  },
  plugin.DefaultConfig
))
if err != nil {
  panic(err)
}
defer term()

modified, err := modifier.Modify("some value")
fmt.Println(modified, err)
```