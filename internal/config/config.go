package config

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Option represents a single configuration value with a default.
type Option struct {
	key         string
	description string
	deflt       string
}

var (
	mu      sync.RWMutex
	options = map[string]*Option{}
)

// RegisterOption registers an option for key, or returns the existing one.
func RegisterOption(key, description, deflt string) *Option {
	mu.Lock()
	defer mu.Unlock()

	if o, ok := options[key]; ok {
		return o
	}
	o := &Option{key: key, description: description, deflt: deflt}
	options[key] = o
	return o
}

// Key returns the full dotted key this option was registered under.
func (o *Option) Key() string { return o.key }

// Description returns the human-readable description.
func (o *Option) Description() string { return o.description }

// Default returns the value used when the environment variable is unset or empty.
func (o *Option) Default() string { return o.deflt }

// EnvName returns the environment variable this option reads from.
func (o *Option) EnvName() string {
	short := o.key
	if i := strings.LastIndex(short, "."); i >= 0 {
		short = short[i+1:]
	}
	name := strings.ToUpper(strings.ReplaceAll(short, "-", "_"))
	return "RATEGATE_" + name
}

// GetString returns the current value: env var if set and non-empty, else default.
func (o *Option) GetString() string {
	if v, ok := os.LookupEnv(o.EnvName()); ok && v != "" {
		return v
	}
	return o.deflt
}

// GetBool parses GetString() as a boolean.
func (o *Option) GetBool() bool {
	v, err := strconv.ParseBool(o.GetString())
	if err != nil {
		return false
	}
	return v
}

// GetInt parses GetString() as an integer.
func (o *Option) GetInt() int {
	n, err := strconv.Atoi(o.GetString())
	if err != nil {
		return 0
	}
	return n
}

// Keys returns every registered key, sorted.
func Keys() []string {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]string, 0, len(options))
	for k := range options {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}