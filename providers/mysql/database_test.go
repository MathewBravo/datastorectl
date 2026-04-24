package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// TestDatabaseHandler_Validate covers the rules that can be checked
// without a live cluster: system-schema rejection, empty-name rejection.
func TestDatabaseHandler_Validate(t *testing.T) {
	h := &databaseHandler{}
	cases := []struct {
		name      string
		resource  provider.Resource
		wantErr   string
	}{
		{
			name: "empty name",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_database", Name: ""},
			},
			wantErr: "name cannot be empty",
		},
		{
			name: "system schema mysql",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_database", Name: "mysql"},
			},
			wantErr: "is a reserved system schema",
		},
		{
			name: "system schema sys",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_database", Name: "sys"},
			},
			wantErr: "is a reserved system schema",
		},
		{
			name: "system schema performance_schema",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_database", Name: "performance_schema"},
			},
			wantErr: "is a reserved system schema",
		},
		{
			name: "system schema information_schema",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_database", Name: "information_schema"},
			},
			wantErr: "is a reserved system schema",
		},
		{
			name: "backticks in name rejected",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_database", Name: "bad`name"},
			},
			wantErr: "backtick",
		},
		{
			name: "valid name passes",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_database", Name: "appdb"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := h.Validate(context.Background(), c.resource)
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

// TestDatabaseHandler_DiscoverFiltersSystemSchemas confirms the four
// system schemas (mysql, sys, performance_schema, information_schema)
// never appear in Discover output, regardless of what the server has.
func TestDatabaseHandler_DiscoverFiltersSystemSchemas(t *testing.T) {
	client := newTestClient(t)
	h := &databaseHandler{}
	resources, err := h.Discover(context.Background(), client)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, r := range resources {
		switch r.ID.Name {
		case "mysql", "sys", "performance_schema", "information_schema":
			t.Errorf("system schema %q leaked into discover output", r.ID.Name)
		}
	}
}

// TestDatabaseHandler_CreateDiscoverDelete exercises the full lifecycle
// against a live cluster. Creates a schema, discovers it, modifies
// collation, re-discovers, deletes.
func TestDatabaseHandler_CreateDiscoverDelete(t *testing.T) {
	client := newTestClient(t)
	h := &databaseHandler{}
	ctx := context.Background()

	dbName := "dsctl_handler_test_db"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP DATABASE IF EXISTS `"+dbName+"`")
	})

	// --- Create
	body := provider.NewOrderedMap()
	body.Set("charset", provider.StringVal("utf8mb4"))
	body.Set("collation", provider.StringVal("utf8mb4_0900_ai_ci"))
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_database", Name: dbName},
		Body: body,
	}
	if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
		t.Fatalf("create: %v", err)
	}

	// --- Discover finds it
	resources, err := h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	found := findByName(resources, dbName)
	if found == nil {
		t.Fatalf("discover did not return %q; got %d other schemas", dbName, len(resources))
	}
	if charset := getStringField(found.Body, "charset"); charset != "utf8mb4" {
		t.Errorf("discovered charset = %q, want utf8mb4", charset)
	}

	// --- Update collation
	body2 := provider.NewOrderedMap()
	body2.Set("charset", provider.StringVal("utf8mb4"))
	body2.Set("collation", provider.StringVal("utf8mb4_unicode_ci"))
	r.Body = body2
	if err := h.Apply(ctx, client, provider.OpUpdate, r); err != nil {
		t.Fatalf("update: %v", err)
	}

	resources, err = h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("re-discover: %v", err)
	}
	found = findByName(resources, dbName)
	if found == nil {
		t.Fatalf("re-discover did not return %q", dbName)
	}
	if coll := getStringField(found.Body, "collation"); coll != "utf8mb4_unicode_ci" {
		t.Errorf("updated collation = %q, want utf8mb4_unicode_ci", coll)
	}

	// --- Delete
	if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
		t.Fatalf("delete: %v", err)
	}
	resources, err = h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("post-delete discover: %v", err)
	}
	if findByName(resources, dbName) != nil {
		t.Errorf("schema %q still present after delete", dbName)
	}
}

// findByName returns a pointer to the first resource whose ID.Name
// matches, or nil.
func findByName(rs []provider.Resource, name string) *provider.Resource {
	for i := range rs {
		if rs[i].ID.Name == name {
			return &rs[i]
		}
	}
	return nil
}

// getStringField fetches a string attribute from a resource body,
// returning "" when absent or of the wrong kind.
func getStringField(m *provider.OrderedMap, key string) string {
	v, ok := m.Get(key)
	if !ok || v.Kind != provider.KindString {
		return ""
	}
	return v.Str
}
