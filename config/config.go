// Package config extracts and validates connection contexts from parsed DCL files.
// Contexts define how to connect to a provider (endpoint, auth, credentials).
// This package bridges DCL AST blocks and the provider.OrderedMap configs
// that engine.ConfigureProviders expects.
package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// SecretResolver resolves a secret reference to its plaintext value.
// This mirrors engine.SecretResolver — defined locally to avoid an import cycle.
type SecretResolver interface {
	Resolve(ctx context.Context, backend, path string) (string, error)
}

// Context is a validated, structured representation of a DCL context block.
type Context struct {
	Name     string              // block label (e.g., "prod-opensearch")
	Provider string              // value of the "provider" attribute
	Attrs    *provider.OrderedMap // remaining attributes (endpoint, auth, etc.)
}

// SplitFile separates context blocks from resource blocks in a parsed DCL file.
// Context blocks have Type == "context". Everything else is treated as a resource block.
func SplitFile(file *dcl.File) (contexts []dcl.Block, resources []dcl.Block) {
	for _, block := range file.Blocks {
		if block.Type == "context" {
			contexts = append(contexts, block)
		} else {
			resources = append(resources, block)
		}
	}
	return contexts, resources
}

// ParseContexts validates and converts raw context blocks into structured Contexts.
//
// Each context block must have:
//   - A label (the context name)
//   - A "provider" attribute that is a string or identifier
//
// The provider attribute is extracted and stored separately; it does not appear in Attrs.
// Duplicate context names produce an error.
func ParseContexts(blocks []dcl.Block) ([]Context, error) {
	seen := map[string]struct{}{}
	contexts := make([]Context, 0, len(blocks))

	for _, block := range blocks {
		if block.Label == "" {
			return nil, fmt.Errorf("context block is missing a name — use: context \"my-name\" { ... }")
		}

		if _, dup := seen[block.Label]; dup {
			return nil, fmt.Errorf("duplicate context name %q — each context must have a unique name", block.Label)
		}
		seen[block.Label] = struct{}{}

		providerName, attrs, err := extractContextAttrs(block)
		if err != nil {
			return nil, fmt.Errorf("context %q: %s", block.Label, err)
		}

		contexts = append(contexts, Context{
			Name:     block.Label,
			Provider: providerName,
			Attrs:    attrs,
		})
	}

	return contexts, nil
}

// BuildConfigs converts parsed contexts into the map[string]*provider.OrderedMap
// format that engine.ConfigureProviders expects. The map key is the provider name
// (e.g., "opensearch"). For v0.1.0, only one context per provider is supported.
func BuildConfigs(contexts []Context) (map[string]*provider.OrderedMap, error) {
	configs := make(map[string]*provider.OrderedMap, len(contexts))
	for _, ctx := range contexts {
		if _, dup := configs[ctx.Provider]; dup {
			return nil, fmt.Errorf("multiple contexts configure provider %q — v0.1.0 supports one context per provider", ctx.Provider)
		}
		configs[ctx.Provider] = ctx.Attrs
	}
	return configs, nil
}

// ResolveResourceContexts strips the "context" attribute from each resource body
// and validates that every resource references a known context whose provider
// matches the resource type prefix.
func ResolveResourceContexts(resources []provider.Resource, contexts []Context) ([]provider.Resource, error) {
	ctxByName := make(map[string]Context, len(contexts))
	for _, ctx := range contexts {
		ctxByName[ctx.Name] = ctx
	}

	resolved := make([]provider.Resource, len(resources))
	for i, r := range resources {
		ctxVal, ok := r.Body.Get("context")
		if !ok {
			return nil, fmt.Errorf("%s: \"context\" attribute is required — every resource must declare which context it belongs to", r.ID)
		}
		if ctxVal.Kind != provider.KindString {
			return nil, fmt.Errorf("%s: \"context\" must be a name, got %s", r.ID, ctxVal.Kind)
		}

		ctxName := ctxVal.Str
		ctx, ok := ctxByName[ctxName]
		if !ok {
			return nil, fmt.Errorf("%s: references unknown context %q", r.ID, ctxName)
		}

		prefix, ok := provider.ProviderForResourceType(r.ID.Type)
		if !ok {
			return nil, fmt.Errorf("%s: cannot determine provider from resource type", r.ID)
		}
		if prefix != ctx.Provider {
			return nil, fmt.Errorf("%s: resource type prefix %q does not match context %q provider %q", r.ID, prefix, ctxName, ctx.Provider)
		}

		body := r.Body.Clone()
		body.Delete("context")
		resolved[i] = provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}
	}

	return resolved, nil
}

// LoadConfigFile loads contexts from a DCL config file (e.g., ~/.datastorectl/config.dcl).
// Returns an empty slice and no error if the file does not exist (config file is optional).
// Returns an error if the file contains resource blocks (config files are for contexts only).
func LoadConfigFile(path string) ([]Context, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	file, diags := dcl.LoadFile(path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("loading config file %s: %s", path, diags.Error())
	}

	contexts, resources := SplitFile(file)
	if len(resources) > 0 {
		return nil, fmt.Errorf("config file %s contains resource blocks — config files should only contain context blocks", path)
	}

	return ParseContexts(contexts)
}

// MergeContexts combines contexts from resource files (inline) with contexts
// from a config file. Duplicate names across sources produce an error.
func MergeContexts(inline, fromFile []Context) ([]Context, error) {
	seen := make(map[string]struct{}, len(inline))
	for _, ctx := range inline {
		seen[ctx.Name] = struct{}{}
	}
	for _, ctx := range fromFile {
		if _, dup := seen[ctx.Name]; dup {
			return nil, fmt.Errorf("context %q defined in both resource files and config file — each context name must be unique", ctx.Name)
		}
		seen[ctx.Name] = struct{}{}
	}

	merged := make([]Context, 0, len(inline)+len(fromFile))
	merged = append(merged, inline...)
	merged = append(merged, fromFile...)
	return merged, nil
}

// DefaultConfigPath returns the default config file path: ~/.datastorectl/config.dcl.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".datastorectl", "config.dcl")
	}
	return filepath.Join(home, ".datastorectl", "config.dcl")
}

// ResolveConfigSecrets walks each config's OrderedMap and resolves secret()
// function calls via the given resolver. Configs are modified in place.
func ResolveConfigSecrets(ctx context.Context, configs map[string]*provider.OrderedMap, resolver SecretResolver) error {
	for providerName, cfg := range configs {
		for _, key := range cfg.Keys() {
			v, _ := cfg.Get(key)
			rv, err := resolveSecretValue(ctx, v, resolver)
			if err != nil {
				return fmt.Errorf("provider %q config attribute %q: %s", providerName, key, err)
			}
			cfg.Set(key, rv)
		}
	}
	return nil
}

// resolveSecretValue recursively walks a Value tree, resolving any secret() calls.
func resolveSecretValue(ctx context.Context, v provider.Value, resolver SecretResolver) (provider.Value, error) {
	switch v.Kind {
	case provider.KindFunctionCall:
		if v.FuncName != "secret" {
			return provider.Value{}, fmt.Errorf("unsupported function %q — only secret() is supported", v.FuncName)
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
		resolved, err := resolver.Resolve(ctx, v.FuncArgs[0].Str, v.FuncArgs[1].Str)
		if err != nil {
			return provider.Value{}, fmt.Errorf("secret(%q, %q): %s", v.FuncArgs[0].Str, v.FuncArgs[1].Str, err)
		}
		return provider.StringVal(resolved), nil

	case provider.KindList:
		elems := make([]provider.Value, len(v.List))
		for i, elem := range v.List {
			rv, err := resolveSecretValue(ctx, elem, resolver)
			if err != nil {
				return provider.Value{}, fmt.Errorf("list element %d: %s", i, err)
			}
			elems[i] = rv
		}
		return provider.ListVal(elems), nil

	case provider.KindMap:
		m := provider.NewOrderedMap()
		for _, key := range v.Map.Keys() {
			val, _ := v.Map.Get(key)
			rv, err := resolveSecretValue(ctx, val, resolver)
			if err != nil {
				return provider.Value{}, fmt.Errorf("key %q: %s", key, err)
			}
			m.Set(key, rv)
		}
		return provider.MapVal(m), nil

	default:
		return v, nil
	}
}

// --- private helpers ---

// extractContextAttrs pulls the "provider" attribute out of a context block
// and converts the remaining attributes into a provider.OrderedMap.
func extractContextAttrs(block dcl.Block) (string, *provider.OrderedMap, error) {
	var providerName string
	attrs := provider.NewOrderedMap()

	for _, attr := range block.Attributes {
		if attr.Key == "provider" {
			name, err := identifierString(attr.Value)
			if err != nil {
				return "", nil, fmt.Errorf("\"provider\" must be a name (e.g. opensearch), got %T", attr.Value)
			}
			providerName = name
			continue
		}

		v, err := exprToValue(attr.Value)
		if err != nil {
			return "", nil, fmt.Errorf("attribute %q: %s", attr.Key, err)
		}
		attrs.Set(attr.Key, v)
	}

	if providerName == "" {
		return "", nil, fmt.Errorf("\"provider\" attribute is required — specify which provider this context configures (e.g. provider = opensearch)")
	}

	return providerName, attrs, nil
}

// identifierString extracts a string from an Identifier or LiteralString expression.
func identifierString(expr dcl.Expression) (string, error) {
	switch e := expr.(type) {
	case *dcl.Identifier:
		return e.Name, nil
	case *dcl.LiteralString:
		return e.Value, nil
	default:
		return "", fmt.Errorf("expected identifier or string, got %T", expr)
	}
}

// exprToValue converts a DCL AST expression into a provider.Value.
// Same logic as engine/convert.go's exprToValue — duplicated here to avoid
// an import cycle (engine will import config in a later ticket).
func exprToValue(expr dcl.Expression) (provider.Value, error) {
	if expr == nil {
		return provider.Value{}, fmt.Errorf("cannot convert nil expression")
	}

	switch e := expr.(type) {
	case *dcl.LiteralString:
		return provider.StringVal(e.Value), nil
	case *dcl.LiteralInt:
		return provider.IntVal(e.Value), nil
	case *dcl.LiteralFloat:
		return provider.FloatVal(e.Value), nil
	case *dcl.LiteralBool:
		return provider.BoolVal(e.Value), nil
	case *dcl.Identifier:
		return provider.StringVal(e.Name), nil
	case *dcl.Reference:
		return provider.RefVal(e.Parts), nil
	case *dcl.FunctionCall:
		args := make([]provider.Value, len(e.Args))
		for i, arg := range e.Args {
			v, err := exprToValue(arg)
			if err != nil {
				return provider.Value{}, fmt.Errorf("function %q arg %d: %w", e.Name, i, err)
			}
			args[i] = v
		}
		return provider.FuncCallVal(e.Name, args), nil
	case *dcl.ListExpr:
		elems := make([]provider.Value, len(e.Elements))
		for i, elem := range e.Elements {
			v, err := exprToValue(elem)
			if err != nil {
				return provider.Value{}, fmt.Errorf("list element %d: %w", i, err)
			}
			elems[i] = v
		}
		return provider.ListVal(elems), nil
	case *dcl.MapExpr:
		m := provider.NewOrderedMap()
		for i, val := range e.Values {
			v, err := exprToValue(val)
			if err != nil {
				return provider.Value{}, fmt.Errorf("map key %q: %w", e.Keys[i], err)
			}
			m.Set(e.Keys[i], v)
		}
		return provider.MapVal(m), nil
	default:
		return provider.Value{}, fmt.Errorf("unsupported expression type %T", expr)
	}
}
