package parse

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateFixtures, when set, regenerates the .json expected-parsed
// output files from the current parser behavior. Use with care — the
// point of fixtures is to lock parser output against regressions.
// Regenerate only after intentional changes to parser or struct
// shapes.
//
//	go test ./providers/mysql/parse/... -update-fixtures
var updateFixtures = flag.Bool("update-fixtures", false,
	"regenerate fixture .json files from current parser output")

// TestFixtures_CreateUser iterates every .sql file under
// testdata/<version>/users/ and asserts the parser produces output
// byte-identical to the sibling .json file. In update mode, writes
// the .json instead of comparing.
func TestFixtures_CreateUser(t *testing.T) {
	matches, err := filepath.Glob("testdata/*/users/*.sql")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no user fixtures found; run providers/mysql/parse/testdata/capture.sh")
	}
	for _, path := range matches {
		path := path
		t.Run(strings.TrimPrefix(path, "testdata/"), func(t *testing.T) {
			version := extractVersion(path)
			ddl := mustReadFile(t, path)
			stmt, err := ParseCreateUser(strings.TrimSpace(ddl), version)
			if err != nil {
				t.Fatalf("ParseCreateUser: %v", err)
			}
			assertJSONFixture(t, path, stmt)
		})
	}
}

// TestFixtures_Grant iterates single-statement grant fixtures. Each
// file holds exactly one GRANT statement (one line). multi_statement.sql
// is handled separately.
func TestFixtures_Grant(t *testing.T) {
	matches, err := filepath.Glob("testdata/*/grants/*.sql")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no grant fixtures found; run providers/mysql/parse/testdata/capture.sh")
	}
	for _, path := range matches {
		path := path
		t.Run(strings.TrimPrefix(path, "testdata/"), func(t *testing.T) {
			version := extractVersion(path)
			raw := mustReadFile(t, path)
			base := filepath.Base(path)
			if base == "multi_statement.sql" {
				stmts, err := ParseGrantLines(raw, version)
				if err != nil {
					t.Fatalf("ParseGrantLines: %v", err)
				}
				assertJSONFixture(t, path, stmts)
				return
			}
			stmt, err := ParseGrant(strings.TrimSpace(raw), version)
			if err != nil {
				t.Fatalf("ParseGrant: %v", err)
			}
			assertJSONFixture(t, path, stmt)
		})
	}
}

// extractVersion pulls "8.0" or "8.4" out of a fixture path like
// "testdata/8.4/users/basic_user.sql".
func extractVersion(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if p == "testdata" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// mustReadFile reads a file or fails the test.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// assertJSONFixture compares the JSON-marshaled parser output against
// the expected .json sibling file. In -update-fixtures mode, writes
// the current output instead.
func assertJSONFixture(t *testing.T, sqlPath string, got interface{}) {
	t.Helper()
	jsonPath := strings.TrimSuffix(sqlPath, ".sql") + ".json"
	encoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	encoded = append(encoded, '\n')
	if *updateFixtures {
		if err := os.WriteFile(jsonPath, encoded, 0644); err != nil {
			t.Fatalf("write %s: %v", jsonPath, err)
		}
		t.Logf("wrote %s", jsonPath)
		return
	}
	expected, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read %s (run with -update-fixtures to generate): %v", jsonPath, err)
	}
	if string(encoded) != string(expected) {
		t.Errorf("fixture mismatch for %s\nexpected:\n%s\ngot:\n%s", sqlPath, expected, encoded)
	}
}
