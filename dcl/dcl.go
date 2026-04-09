// Package dcl implements parsing and loading of DCL (Datastore Configuration Language) files.
//
// DCL is an HCL-inspired language for declaring datastore resource configurations:
// ISM policies, security roles, role mappings, and other post-provisioning settings.
//
// Entry points:
//
//   - [Parse] parses DCL source bytes into an AST.
//   - [LoadFile] reads and parses a single .dcl file from disk.
//   - [LoadDirectory] discovers, parses, and merges all .dcl files under a directory.
//
// The AST consists of [File], [Block], and [Attribute] structural nodes, with
// expression values represented by types implementing the [Expression] interface:
// [LiteralString], [LiteralInt], [LiteralFloat], [LiteralBool], [ListExpr],
// [MapExpr], [Identifier], [Reference], and [FunctionCall].
//
// Errors and warnings are reported as [Diagnostics] with source [Range] locations.
package dcl
