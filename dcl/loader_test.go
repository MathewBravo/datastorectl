package dcl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDCLFile writes content to dir/name and returns the full path.
func writeDCLFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- LoadFile tests ---

func TestLoadFile_Simple(t *testing.T) {
	dir := t.TempDir()
	path := writeDCLFile(t, dir, "main.dcl", `resource "db" { host = "localhost" }`)

	f, diags := LoadFile(path)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	if f.Blocks[0].Rng.Start.Filename != path {
		t.Errorf("expected filename %q, got %q", path, f.Blocks[0].Rng.Start.Filename)
	}
}

func TestLoadFile_NonexistentPath(t *testing.T) {
	f, diags := LoadFile("/nonexistent/path/file.dcl")
	if !diags.HasErrors() {
		t.Fatal("expected errors for nonexistent path")
	}
	if len(f.Blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(f.Blocks))
	}
	if got := diags[0].Message; !strings.Contains(got, "/nonexistent/path/file.dcl") {
		t.Errorf("expected message to contain path, got: %s", got)
	}
}

func TestLoadFile_ParseErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeDCLFile(t, dir, "bad.dcl", `resource "db" { host = }`)

	f, diags := LoadFile(path)
	if !diags.HasErrors() {
		t.Fatal("expected parse errors")
	}
	if f == nil {
		t.Fatal("expected non-nil File")
	}
	for _, d := range diags {
		if d.Range.Start.Filename != "" && d.Range.Start.Filename != path {
			t.Errorf("expected filename %q on diagnostic, got %q", path, d.Range.Start.Filename)
		}
	}
}

// --- LoadDirectory tests ---

func TestLoadDirectory_TwoFilesMerged(t *testing.T) {
	dir := t.TempDir()
	writeDCLFile(t, dir, "a.dcl", `resource "alpha" { name = "a" }`)
	writeDCLFile(t, dir, "b.dcl", `resource "beta" { name = "b" }`)

	f, diags := LoadDirectory(dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(f.Blocks))
	}
	if f.Blocks[0].Type != "resource" || f.Blocks[0].Label != "alpha" {
		t.Errorf("expected first block to be alpha, got %s %s", f.Blocks[0].Type, f.Blocks[0].Label)
	}
	if f.Blocks[1].Type != "resource" || f.Blocks[1].Label != "beta" {
		t.Errorf("expected second block to be beta, got %s %s", f.Blocks[1].Type, f.Blocks[1].Label)
	}
}

func TestLoadDirectory_SingleFile(t *testing.T) {
	dir := t.TempDir()
	writeDCLFile(t, dir, "only.dcl", `resource "solo" { id = 1 }`)

	f, diags := LoadDirectory(dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
}

func TestLoadDirectory_RecursiveSubdirs(t *testing.T) {
	dir := t.TempDir()
	writeDCLFile(t, dir, "a.dcl", `resource "top" { level = "root" }`)
	writeDCLFile(t, dir, "sub/b.dcl", `resource "nested" { level = "sub" }`)

	f, diags := LoadDirectory(dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(f.Blocks))
	}
	if f.Blocks[0].Label != "top" {
		t.Errorf("expected first block label 'top', got %q", f.Blocks[0].Label)
	}
	if f.Blocks[1].Label != "nested" {
		t.Errorf("expected second block label 'nested', got %q", f.Blocks[1].Label)
	}
}

func TestLoadDirectory_LexicographicOrder(t *testing.T) {
	dir := t.TempDir()
	writeDCLFile(t, dir, "z.dcl", `resource "zulu" { order = 3 }`)
	writeDCLFile(t, dir, "a.dcl", `resource "alpha" { order = 1 }`)
	writeDCLFile(t, dir, "m/b.dcl", `resource "mid" { order = 2 }`)

	f, diags := LoadDirectory(dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(f.Blocks))
	}
	// Sorted by full path: a.dcl, m/b.dcl, z.dcl
	expected := []string{"alpha", "mid", "zulu"}
	for i, want := range expected {
		if f.Blocks[i].Label != want {
			t.Errorf("block[%d]: expected label %q, got %q", i, want, f.Blocks[i].Label)
		}
	}
}

func TestLoadDirectory_NoDCLFiles(t *testing.T) {
	dir := t.TempDir()
	writeDCLFile(t, dir, "readme.txt", "not a dcl file")

	f, diags := LoadDirectory(dir)
	if !diags.HasErrors() {
		t.Fatal("expected error for no .dcl files")
	}
	if len(f.Blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(f.Blocks))
	}
	if got := diags[0].Message; !strings.Contains(got, "no .dcl files") {
		t.Errorf("expected 'no .dcl files' in message, got: %s", got)
	}
}

func TestLoadDirectory_NonexistentPath(t *testing.T) {
	f, diags := LoadDirectory("/nonexistent/dir/path")
	if !diags.HasErrors() {
		t.Fatal("expected errors for nonexistent path")
	}
	if len(f.Blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(f.Blocks))
	}
}

func TestLoadDirectory_MixedValidAndErrored(t *testing.T) {
	dir := t.TempDir()
	writeDCLFile(t, dir, "good.dcl", `resource "ok" { status = "fine" }`)
	writeDCLFile(t, dir, "bad.dcl", `resource "broken" { status = }`)

	f, diags := LoadDirectory(dir)
	if !diags.HasErrors() {
		t.Fatal("expected errors from broken file")
	}
	foundOk := false
	for _, b := range f.Blocks {
		if b.Label == "ok" {
			foundOk = true
		}
	}
	if !foundOk {
		t.Error("expected block from good.dcl to be present")
	}
}

func TestLoadDirectory_FilenameOnNodes(t *testing.T) {
	dir := t.TempDir()
	pathA := writeDCLFile(t, dir, "first.dcl", `resource "one" { x = 1 }`)
	pathB := writeDCLFile(t, dir, "second.dcl", `resource "two" { y = 2 }`)

	f, diags := LoadDirectory(dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(f.Blocks))
	}
	if f.Blocks[0].Rng.Start.Filename != pathA {
		t.Errorf("block[0]: expected filename %q, got %q", pathA, f.Blocks[0].Rng.Start.Filename)
	}
	if f.Blocks[1].Rng.Start.Filename != pathB {
		t.Errorf("block[1]: expected filename %q, got %q", pathB, f.Blocks[1].Rng.Start.Filename)
	}
}
