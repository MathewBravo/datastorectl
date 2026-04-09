package dcl

import (
	"strings"
	"testing"
)

// fullDCL is a realistic multi-block fixture that exercises every expression node type.
const fullDCL = `
ism_policy "rollover_policy" {
  description    = "Rollover and delete old indices"
  schema_version = 1

  state "hot" {
    priority = 100
    actions  = []
    min_size = 50.5

    transition "to_warm" {
      condition = min_index_age("7d")
    }
  }

  state "warm" {
    priority = 50
    readonly = true
    actions  = [
      { warm_migration = {} }
    ]

    transition "to_delete" {
      condition = min_index_age("30d")
    }
  }

  state "delete" {
    priority = 1
    readonly = false
  }

  default_state = hot
}

opensearch_role "log_reader" {
  cluster_permissions = ["cluster:monitor", "indices:data/read"]

  metadata = {
    team = "platform"
    tier = "core"
  }

  index_permissions "logs" {
    index_patterns = ["logs-*"]
    allowed_actions = ["read", "search"]
  }
}

role_mapping "log_reader_mapping" {
  role    = opensearch_role.log_reader
  backend = secret("vault", "opensearch/log_reader")
}
`

// findAttr searches a block's attributes by key, failing the test if not found.
func findAttr(t *testing.T, b Block, key string) Attribute {
	t.Helper()
	for _, a := range b.Attributes {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("attribute %q not found in block %s %q", key, b.Type, b.Label)
	return Attribute{}
}

func TestIntegration_FullDCL_StructuralShape(t *testing.T) {
	f, diags := Parse("full.dcl", []byte(fullDCL))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors:\n%s", diags.Error())
	}

	// 3 top-level blocks
	if got := len(f.Blocks); got != 3 {
		t.Fatalf("expected 3 top-level blocks, got %d", got)
	}

	ism := f.Blocks[0]
	role := f.Blocks[1]
	mapping := f.Blocks[2]

	// ISM policy block
	if ism.Type != "ism_policy" || ism.Label != "rollover_policy" {
		t.Errorf("block[0]: expected ism_policy/rollover_policy, got %s/%s", ism.Type, ism.Label)
	}
	if got := len(ism.Attributes); got != 3 {
		t.Errorf("ism_policy: expected 3 attributes (description, schema_version, default_state), got %d", got)
	}
	if got := len(ism.Blocks); got != 3 {
		t.Errorf("ism_policy: expected 3 state sub-blocks, got %d", got)
	}

	// Check state block labels
	stateLabels := []string{"hot", "warm", "delete"}
	for i, want := range stateLabels {
		if ism.Blocks[i].Label != want {
			t.Errorf("state[%d]: expected label %q, got %q", i, want, ism.Blocks[i].Label)
		}
	}

	// hot state: 3 attrs (priority, actions, min_size) + 1 transition sub-block
	hot := ism.Blocks[0]
	if got := len(hot.Attributes); got != 3 {
		t.Errorf("hot: expected 3 attributes, got %d", got)
	}
	if got := len(hot.Blocks); got != 1 {
		t.Errorf("hot: expected 1 transition block, got %d", got)
	}

	// warm state: 2 attrs (priority, readonly) + actions attr + 1 transition sub-block
	warm := ism.Blocks[1]
	if got := len(warm.Attributes); got != 3 {
		t.Errorf("warm: expected 3 attributes, got %d", got)
	}
	if got := len(warm.Blocks); got != 1 {
		t.Errorf("warm: expected 1 transition block, got %d", got)
	}

	// delete state: 2 attrs (priority, readonly), no sub-blocks
	del := ism.Blocks[2]
	if got := len(del.Attributes); got != 2 {
		t.Errorf("delete: expected 2 attributes, got %d", got)
	}
	if got := len(del.Blocks); got != 0 {
		t.Errorf("delete: expected 0 sub-blocks, got %d", got)
	}

	// opensearch_role block
	if role.Type != "opensearch_role" || role.Label != "log_reader" {
		t.Errorf("block[1]: expected opensearch_role/log_reader, got %s/%s", role.Type, role.Label)
	}
	if got := len(role.Blocks); got != 1 {
		t.Errorf("role: expected 1 index_permissions sub-block, got %d", got)
	}

	// role_mapping block — flat, no sub-blocks
	if mapping.Type != "role_mapping" || mapping.Label != "log_reader_mapping" {
		t.Errorf("block[2]: expected role_mapping/log_reader_mapping, got %s/%s", mapping.Type, mapping.Label)
	}
	if got := len(mapping.Blocks); got != 0 {
		t.Errorf("role_mapping: expected 0 sub-blocks, got %d", got)
	}
}

func TestIntegration_FullDCL_EveryExpressionType(t *testing.T) {
	f, diags := Parse("full.dcl", []byte(fullDCL))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors:\n%s", diags.Error())
	}

	ism := f.Blocks[0]
	role := f.Blocks[1]
	mapping := f.Blocks[2]

	t.Run("LiteralString", func(t *testing.T) {
		attr := findAttr(t, ism, "description")
		ls, ok := attr.Value.(*LiteralString)
		if !ok {
			t.Fatalf("expected *LiteralString, got %T", attr.Value)
		}
		if ls.Value != "Rollover and delete old indices" {
			t.Errorf("expected %q, got %q", "Rollover and delete old indices", ls.Value)
		}
	})

	t.Run("LiteralInt", func(t *testing.T) {
		attr := findAttr(t, ism, "schema_version")
		li, ok := attr.Value.(*LiteralInt)
		if !ok {
			t.Fatalf("expected *LiteralInt, got %T", attr.Value)
		}
		if li.Value != 1 {
			t.Errorf("expected 1, got %d", li.Value)
		}
	})

	t.Run("LiteralFloat", func(t *testing.T) {
		hot := ism.Blocks[0]
		attr := findAttr(t, hot, "min_size")
		lf, ok := attr.Value.(*LiteralFloat)
		if !ok {
			t.Fatalf("expected *LiteralFloat, got %T", attr.Value)
		}
		if lf.Value != 50.5 {
			t.Errorf("expected 50.5, got %f", lf.Value)
		}
	})

	t.Run("LiteralBool_true", func(t *testing.T) {
		warm := ism.Blocks[1]
		attr := findAttr(t, warm, "readonly")
		lb, ok := attr.Value.(*LiteralBool)
		if !ok {
			t.Fatalf("expected *LiteralBool, got %T", attr.Value)
		}
		if !lb.Value {
			t.Error("expected true, got false")
		}
	})

	t.Run("LiteralBool_false", func(t *testing.T) {
		del := ism.Blocks[2]
		attr := findAttr(t, del, "readonly")
		lb, ok := attr.Value.(*LiteralBool)
		if !ok {
			t.Fatalf("expected *LiteralBool, got %T", attr.Value)
		}
		if lb.Value {
			t.Error("expected false, got true")
		}
	})

	t.Run("ListExpr_empty", func(t *testing.T) {
		hot := ism.Blocks[0]
		attr := findAttr(t, hot, "actions")
		le, ok := attr.Value.(*ListExpr)
		if !ok {
			t.Fatalf("expected *ListExpr, got %T", attr.Value)
		}
		if len(le.Elements) != 0 {
			t.Errorf("expected 0 elements, got %d", len(le.Elements))
		}
	})

	t.Run("ListExpr_nonEmpty", func(t *testing.T) {
		attr := findAttr(t, role, "cluster_permissions")
		le, ok := attr.Value.(*ListExpr)
		if !ok {
			t.Fatalf("expected *ListExpr, got %T", attr.Value)
		}
		if len(le.Elements) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(le.Elements))
		}
		first, ok := le.Elements[0].(*LiteralString)
		if !ok {
			t.Fatalf("expected *LiteralString element, got %T", le.Elements[0])
		}
		if first.Value != "cluster:monitor" {
			t.Errorf("expected %q, got %q", "cluster:monitor", first.Value)
		}
	})

	t.Run("MapExpr", func(t *testing.T) {
		attr := findAttr(t, role, "metadata")
		me, ok := attr.Value.(*MapExpr)
		if !ok {
			t.Fatalf("expected *MapExpr, got %T", attr.Value)
		}
		if len(me.Keys) != 2 {
			t.Fatalf("expected 2 keys, got %d", len(me.Keys))
		}
		if me.Keys[0] != "team" || me.Keys[1] != "tier" {
			t.Errorf("expected keys [team, tier], got %v", me.Keys)
		}
		v0, ok := me.Values[0].(*LiteralString)
		if !ok {
			t.Fatalf("expected *LiteralString value, got %T", me.Values[0])
		}
		if v0.Value != "platform" {
			t.Errorf("expected %q, got %q", "platform", v0.Value)
		}
	})

	t.Run("Identifier", func(t *testing.T) {
		// default_state = hot is an attribute on the ism_policy block
		// It should be parsed as an Identifier (single bare name)
		attr := findAttr(t, ism, "default_state")
		id, ok := attr.Value.(*Identifier)
		if !ok {
			t.Fatalf("expected *Identifier, got %T", attr.Value)
		}
		if id.Name != "hot" {
			t.Errorf("expected %q, got %q", "hot", id.Name)
		}
	})

	t.Run("Reference", func(t *testing.T) {
		attr := findAttr(t, mapping, "role")
		ref, ok := attr.Value.(*Reference)
		if !ok {
			t.Fatalf("expected *Reference, got %T", attr.Value)
		}
		if len(ref.Parts) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(ref.Parts))
		}
		if ref.Parts[0] != "opensearch_role" || ref.Parts[1] != "log_reader" {
			t.Errorf("expected [opensearch_role log_reader], got %v", ref.Parts)
		}
	})

	t.Run("FunctionCall", func(t *testing.T) {
		// transition "to_warm" { condition = min_index_age("7d") }
		hot := ism.Blocks[0]
		transition := hot.Blocks[0]
		attr := findAttr(t, transition, "condition")
		fc, ok := attr.Value.(*FunctionCall)
		if !ok {
			t.Fatalf("expected *FunctionCall, got %T", attr.Value)
		}
		if fc.Name != "min_index_age" {
			t.Errorf("expected function name %q, got %q", "min_index_age", fc.Name)
		}
		if len(fc.Args) != 1 {
			t.Fatalf("expected 1 arg, got %d", len(fc.Args))
		}
		arg, ok := fc.Args[0].(*LiteralString)
		if !ok {
			t.Fatalf("expected *LiteralString arg, got %T", fc.Args[0])
		}
		if arg.Value != "7d" {
			t.Errorf("expected %q, got %q", "7d", arg.Value)
		}
	})

	t.Run("FunctionCall_multiArg", func(t *testing.T) {
		attr := findAttr(t, mapping, "backend")
		fc, ok := attr.Value.(*FunctionCall)
		if !ok {
			t.Fatalf("expected *FunctionCall, got %T", attr.Value)
		}
		if fc.Name != "secret" {
			t.Errorf("expected function name %q, got %q", "secret", fc.Name)
		}
		if len(fc.Args) != 2 {
			t.Fatalf("expected 2 args, got %d", len(fc.Args))
		}
	})
}

func TestIntegration_EmptyFile(t *testing.T) {
	f, diags := Parse("empty.dcl", []byte(""))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(f.Blocks))
	}
}

func TestIntegration_CommentsOnlyFile(t *testing.T) {
	src := `# This is a comment
# Another comment
# Hash comment
`
	f, diags := Parse("comments.dcl", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}
	if len(f.Blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(f.Blocks))
	}
}

func TestIntegration_InterpolationError(t *testing.T) {
	src := `resource "db" {
  greeting = "hello ${world}"
}`
	_, diags := Parse("interp.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors for string interpolation")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "string interpolation with ${} is not supported") {
			found = true
			if d.Suggestion == "" {
				t.Error("expected non-empty Suggestion on interpolation diagnostic")
			}
		}
	}
	if !found {
		t.Errorf("expected interpolation diagnostic, got:\n%s", diags.Error())
	}
}

func TestIntegration_LoadFile(t *testing.T) {
	dir := t.TempDir()
	path := writeDCLFile(t, dir, "full.dcl", fullDCL)

	f, diags := LoadFile(path)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors:\n%s", diags.Error())
	}
	if got := len(f.Blocks); got != 3 {
		t.Fatalf("expected 3 blocks, got %d", got)
	}
	if f.Blocks[0].Type != "ism_policy" {
		t.Errorf("block[0]: expected type ism_policy, got %s", f.Blocks[0].Type)
	}
	if f.Blocks[1].Type != "opensearch_role" {
		t.Errorf("block[1]: expected type opensearch_role, got %s", f.Blocks[1].Type)
	}
	if f.Blocks[2].Type != "role_mapping" {
		t.Errorf("block[2]: expected type role_mapping, got %s", f.Blocks[2].Type)
	}
}

func TestIntegration_LoadDirectory(t *testing.T) {
	dir := t.TempDir()

	// Split the fixture across 3 files, one block each.
	writeDCLFile(t, dir, "ism.dcl", `
ism_policy "rollover_policy" {
  description    = "Rollover and delete old indices"
  schema_version = 1
  default_state  = hot
}
`)
	writeDCLFile(t, dir, "role.dcl", `
opensearch_role "log_reader" {
  cluster_permissions = ["cluster:monitor"]
}
`)
	writeDCLFile(t, dir, "mapping.dcl", `
role_mapping "log_reader_mapping" {
  role = opensearch_role.log_reader
}
`)

	f, diags := LoadDirectory(dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors:\n%s", diags.Error())
	}
	if got := len(f.Blocks); got != 3 {
		t.Fatalf("expected 3 blocks, got %d", got)
	}

	// Files are sorted lexicographically: ism.dcl, mapping.dcl, role.dcl
	types := []string{f.Blocks[0].Type, f.Blocks[1].Type, f.Blocks[2].Type}
	expectedTypes := []string{"ism_policy", "role_mapping", "opensearch_role"}
	for i, want := range expectedTypes {
		if types[i] != want {
			t.Errorf("block[%d]: expected type %q, got %q", i, want, types[i])
		}
	}
}
