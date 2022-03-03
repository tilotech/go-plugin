package plugin

import "time"

// Config defines the configuration for a plugin.
type Config struct {
	Timeout        time.Duration
	ConnectTimeout time.Duration
	KeepAlive      time.Duration
}

// DefaultConfig returns you a new Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Timeout:        30 * time.Second,
		ConnectTimeout: 5 * time.Second,
		KeepAlive:      30 * time.Second,
	}
}
