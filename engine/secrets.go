package engine

import (
	"context"
	"fmt"
	"os"

	"github.com/MathewBravo/datastorectl/provider"
)

// SecretResolver resolves a secret reference to its plaintext value.
type SecretResolver interface {
	Resolve(ctx context.Context, backend, path string) (string, error)
}

// EnvSecretResolver resolves secrets from environment variables.
// It only supports the "env" backend.
type EnvSecretResolver struct{}

// Resolve reads the environment variable named by path.
// It returns an error if backend is not "env" or if the variable is not set.
func (EnvSecretResolver) Resolve(ctx context.Context, backend, path string) (string, error) {
	if backend != "env" {
		return "", fmt.Errorf("unsupported secret backend %q", backend)
	}
	v, ok := os.LookupEnv(path)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", path)
	}
	return v, nil
}

// ResolveSecrets walks a resource body and replaces KindFunctionCall values
// (specifically secret() calls) with concrete string values obtained from the
// given resolver. The original resource is not mutated.
func ResolveSecrets(ctx context.Context, r provider.Resource, resolver SecretResolver) (provider.Resource, error) {
	if r.Body == nil {
		return provider.Resource{ID: r.ID, Body: nil, SourceRange: r.SourceRange}, nil
	}
	resolved := r.Body.Clone()
	for _, key := range resolved.Keys() {
		v, _ := resolved.Get(key)
		rv, err := resolveValue(ctx, v, resolver)
		if err != nil {
			return provider.Resource{}, fmt.Errorf("attribute %q: %w", key, err)
		}
		resolved.Set(key, rv)
	}
	return provider.Resource{ID: r.ID, Body: resolved, SourceRange: r.SourceRange}, nil
}

// resolveValue recursively walks a Value tree, resolving any function calls.
func resolveValue(ctx context.Context, v provider.Value, resolver SecretResolver) (provider.Value, error) {
	switch v.Kind {
	case provider.KindFunctionCall:
		return resolveFunction(ctx, v, resolver)
	case provider.KindList:
		elems := make([]provider.Value, len(v.List))
		for i, elem := range v.List {
			rv, err := resolveValue(ctx, elem, resolver)
			if err != nil {
				return provider.Value{}, fmt.Errorf("list element %d: %w", i, err)
			}
			elems[i] = rv
		}
		return provider.ListVal(elems), nil
	case provider.KindMap:
		m := provider.NewOrderedMap()
		for _, key := range v.Map.Keys() {
			val, _ := v.Map.Get(key)
			rv, err := resolveValue(ctx, val, resolver)
			if err != nil {
				return provider.Value{}, fmt.Errorf("key %q: %w", key, err)
			}
			m.Set(key, rv)
		}
		return provider.MapVal(m), nil
	default:
		return v, nil
	}
}

// resolveFunction dispatches a KindFunctionCall value to the appropriate
// handler. Only "secret" is supported in v0.1.0.
func resolveFunction(ctx context.Context, v provider.Value, resolver SecretResolver) (provider.Value, error) {
	if v.FuncName != "secret" {
		return provider.Value{}, fmt.Errorf("unsupported function %q", v.FuncName)
	}
	if len(v.FuncArgs) != 2 {
		return provider.Value{}, fmt.Errorf("secret() requires exactly 2 arguments, got %d", len(v.FuncArgs))
	}
	if v.FuncArgs[0].Kind != provider.KindString {
		return provider.Value{}, fmt.Errorf("secret() argument 0 must be a string, got %s", v.FuncArgs[0].Kind)
	}
	if v.FuncArgs[1].Kind != provider.KindString {
		return provider.Value{}, fmt.Errorf("secret() argument 1 must be a string, got %s", v.FuncArgs[1].Kind)
	}
	backend := v.FuncArgs[0].Str
	path := v.FuncArgs[1].Str
	resolved, err := resolver.Resolve(ctx, backend, path)
	if err != nil {
		return provider.Value{}, fmt.Errorf("secret(%q, %q): %w", backend, path, err)
	}
	return provider.StringVal(resolved), nil
}
