package provider

import (
	"github.com/MathewBravo/datastorectl/dcl"
)

// ResourceID uniquely identifies a resource by its provider type and name.
type ResourceID struct {
	Type string // provider resource type, e.g. "opensearch_ism_policy"
	Name string // resource name, e.g. "hot_warm_delete"
}

// String returns the resource identifier as "Type.Name".
func (id ResourceID) String() string {
	return id.Type + "." + id.Name
}

// Resource represents a DCL resource flowing through the engine pipeline
// (parse → plan → apply). Live-state resources fetched from providers
// will have a zero-value SourceRange.
type Resource struct {
	ID          ResourceID
	Body        *OrderedMap
	SourceRange dcl.Range
}
