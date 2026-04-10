package provider

import (
	"sort"
	"strings"
	"sync"
)

// ProviderFunc is a function that creates a new Provider instance.
type ProviderFunc func() Provider

var (
	registryMu sync.RWMutex
	registry   = make(map[string]ProviderFunc)
)

// Register adds a ProviderFunc to the registry under the given name.
// It panics if called twice with the same name (a programming error).
func Register(name string, f ProviderFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup {
		panic("provider: Register called twice for provider " + name)
	}
	registry[name] = f
}

// Lookup returns the ProviderFunc registered under the given name.
func Lookup(name string) (ProviderFunc, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[name]
	return f, ok
}

// RegisteredNames returns the names of all registered providers, sorted.
func RegisteredNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ProviderForResourceType extracts the provider prefix from a resource type
// string by splitting on the first underscore. It does not consult the
// registry; callers compose with Lookup.
func ProviderForResourceType(resourceType string) (string, bool) {
	prefix, _, ok := strings.Cut(resourceType, "_")
	if !ok {
		return "", false
	}
	return prefix, true
}
