// loader.go implements single-file and recursive directory loading of .dcl files.
package dcl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LoadFile reads a single file from disk and parses it.
// On read failure, returns an empty *File with a single error diagnostic.
func LoadFile(path string) (*File, Diagnostics) {
	src, err := os.ReadFile(path)
	if err != nil {
		diag := Diagnostic{
			Severity: SeverityError,
			Message:  fmt.Sprintf("failed to read file: %s", err),
		}
		return &File{}, Diagnostics{diag}
	}
	return Parse(path, src)
}

// collectDCLFiles recursively collects all .dcl file paths under dir,
// returning them sorted by full path.
func collectDCLFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".dcl" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// mergeFiles concatenates blocks and diagnostics from multiple parsed files
// into a single *File with a zero-value Range.
func mergeFiles(files []*File) *File {
	merged := &File{}
	for _, f := range files {
		merged.Blocks = append(merged.Blocks, f.Blocks...)
		merged.Diagnostics.Append(f.Diagnostics)
	}
	return merged
}

// LoadDirectory discovers all .dcl files under dir (recursively),
// parses each one, and merges the results into a single *File.
func LoadDirectory(dir string) (*File, Diagnostics) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		msg := fmt.Sprintf("path is not a directory: %s", dir)
		if err != nil {
			msg = fmt.Sprintf("failed to access directory: %s", err)
		}
		diag := Diagnostic{
			Severity: SeverityError,
			Message:  msg,
		}
		return &File{}, Diagnostics{diag}
	}

	paths, err := collectDCLFiles(dir)
	if err != nil {
		diag := Diagnostic{
			Severity: SeverityError,
			Message:  fmt.Sprintf("failed to walk directory: %s", err),
		}
		return &File{}, Diagnostics{diag}
	}

	if len(paths) == 0 {
		diag := Diagnostic{
			Severity: SeverityError,
			Message:  fmt.Sprintf("no .dcl files found in %s", dir),
		}
		return &File{}, Diagnostics{diag}
	}

	files := make([]*File, 0, len(paths))
	for _, p := range paths {
		f, _ := LoadFile(p)
		files = append(files, f)
	}

	merged := mergeFiles(files)
	return merged, merged.Diagnostics
}
