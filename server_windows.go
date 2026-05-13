//go:build windows

package plugin

import "fmt"

// listenAndServe is not implemented under Windows and will always return an
// error.
func listenAndServe(_ Provider, _ <-chan struct{}, _ chan<- struct{}) error {
	return fmt.Errorf("not implemented under Windows")
}
