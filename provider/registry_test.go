package provider

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
)

func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = make(map[string]ProviderFunc)
}

// stubProvider satisfies the Provider interface with no-op methods.
type stubProvider struct{}

var _ Provider = stubProvider{}

func (stubProvider) Configure(context.Context, *OrderedMap) dcl.Diagnostics { return nil }
func (stubProvider) Discover(context.Context) ([]Resource, dcl.Diagnostics) { return nil, nil }
func (stubProvider) Normalize(_ context.Context, r Resource) (Resource, dcl.Diagnostics) {
	return r, nil
}
func (stubProvider) Validate(context.Context, Resource) dcl.Diagnostics { return nil }
func (stubProvider) Apply(context.Context, Operation, Resource) dcl.Diagnostics {
	return nil
}

func TestRegisterAndLookup(t *testing.T) {
	t.Cleanup(resetRegistry)

	Register("test", func() Provider { return stubProvider{} })

	f, ok := Lookup("test")
	if !ok {
		t.Fatal("Lookup returned false for registered provider")
	}
	p := f()
	if _, ok := p.(Provider); !ok {
		t.Fatal("factory did not return a Provider")
	}
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	t.Cleanup(resetRegistry)

	Register("dup", func() Provider { return stubProvider{} })

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate Register")
		}
		msg := fmt.Sprint(r)
		if got := msg; got != "provider: Register called twice for provider dup" {
			t.Errorf("panic message = %q, want it to contain provider name", got)
		}
	}()
	Register("dup", func() Provider { return stubProvider{} })
}

func TestLookupMiss(t *testing.T) {
	t.Cleanup(resetRegistry)

	f, ok := Lookup("nonexistent")
	if ok {
		t.Error("Lookup returned true for unregistered provider")
	}
	if f != nil {
		t.Error("Lookup returned non-nil factory for unregistered provider")
	}
}

func TestRegisteredNames(t *testing.T) {
	t.Cleanup(resetRegistry)

	// Empty registry returns empty slice.
	names := RegisteredNames()
	if len(names) != 0 {
		t.Errorf("empty registry: got %v, want []", names)
	}

	Register("zeta", func() Provider { return stubProvider{} })
	Register("alpha", func() Provider { return stubProvider{} })
	Register("mu", func() Provider { return stubProvider{} })

	names = RegisteredNames()
	want := []string{"alpha", "mu", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestProviderForResourceType(t *testing.T) {
	tests := []struct {
		input      string
		wantPrefix string
		wantOK     bool
	}{
		{"opensearch_ism_policy", "opensearch", true},
		{"s3_bucket", "s3", true},
		{"aws_iam_role_policy", "aws", true},
		{"foo_", "foo", true},
		{"standalone", "", false},
		{"", "", false},
		{"_orphan", "", true},
		{"_", "", true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			prefix, ok := ProviderForResourceType(tt.input)
			if prefix != tt.wantPrefix || ok != tt.wantOK {
				t.Errorf("ProviderForResourceType(%q) = (%q, %v), want (%q, %v)",
					tt.input, prefix, ok, tt.wantPrefix, tt.wantOK)
			}
		})
	}
}

func TestRegistryConcurrency(t *testing.T) {
	t.Cleanup(resetRegistry)

	const n = 100
	var wg sync.WaitGroup

	// Concurrent registrations with unique names.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("provider-%03d", i)
			Register(name, func() Provider { return stubProvider{} })
		}(i)
	}

	// Concurrent reads while registrations are happening.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("provider-%03d", i)
			Lookup(name)
			RegisteredNames()
		}(i)
	}

	wg.Wait()

	names := RegisteredNames()
	if len(names) != n {
		t.Errorf("got %d registered providers, want %d", len(names), n)
	}
}
