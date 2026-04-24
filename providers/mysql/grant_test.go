package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// TestGrantHandler_Validate covers per-resource rules.
func TestGrantHandler_Validate(t *testing.T) {
	h := &grantHandler{}
	cases := []struct {
		name     string
		resource provider.Resource
		wantErr  string
	}{
		{
			name:    "missing user",
			resource: grantResource(map[string]provider.Value{
				"database":   provider.StringVal("appdb"),
				"table":      provider.StringVal("*"),
				"privileges": stringList("SELECT"),
			}),
			wantErr: "user",
		},
		{
			name: "missing database",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("app"),
				"host":       provider.StringVal("%"),
				"table":      provider.StringVal("*"),
				"privileges": stringList("SELECT"),
			}),
			wantErr: "database",
		},
		{
			name: "missing privileges",
			resource: grantResource(map[string]provider.Value{
				"user":     provider.StringVal("app"),
				"host":     provider.StringVal("%"),
				"database": provider.StringVal("appdb"),
				"table":    provider.StringVal("*"),
			}),
			wantErr: "privileges",
		},
		{
			name: "empty privilege string",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("app"),
				"host":       provider.StringVal("%"),
				"database":   provider.StringVal("appdb"),
				"table":      provider.StringVal("*"),
				"privileges": stringList("SELECT", ""),
			}),
			wantErr: "empty privilege",
		},
		{
			name: "valid minimal",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("app"),
				"host":       provider.StringVal("%"),
				"database":   provider.StringVal("appdb"),
				"table":      provider.StringVal("*"),
				"privileges": stringList("SELECT"),
			}),
		},
		{
			name: "valid global scope",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("app"),
				"host":       provider.StringVal("%"),
				"database":   provider.StringVal("*"),
				"table":      provider.StringVal("*"),
				"privileges": stringList("PROCESS"),
			}),
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

// TestGrantHandler_Normalize confirms ID.Name rewrites to the
// (user, host, database, table) canonical form and privileges sort.
func TestGrantHandler_Normalize(t *testing.T) {
	h := &grantHandler{}
	r := grantResource(map[string]provider.Value{
		"user":       provider.StringVal("app"),
		"host":       provider.StringVal("10.0.%"),
		"database":   provider.StringVal("appdb"),
		"table":      provider.StringVal("*"),
		"privileges": stringList("update", "select", "insert"),
	})
	out, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	want := "app@10.0.%:appdb.*"
	if out.ID.Name != want {
		t.Errorf("ID.Name = %q, want %q", out.ID.Name, want)
	}
	privs := getStringListField(out.Body, "privileges")
	wantPrivs := []string{"INSERT", "SELECT", "UPDATE"}
	if len(privs) != 3 || privs[0] != wantPrivs[0] || privs[1] != wantPrivs[1] || privs[2] != wantPrivs[2] {
		t.Errorf("privileges = %v, want %v", privs, wantPrivs)
	}
}

// TestGrantHandler_CreateDiscoverDelete exercises the schema-scope
// path. Creates a user, grants, discovers, revokes.
func TestGrantHandler_CreateDiscoverDelete(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	h := &grantHandler{}

	user := "dsctl_grant_user"
	dbName := "dsctl_grant_db"
	host := "%"

	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP DATABASE IF EXISTS `"+dbName+"`")
		_, _ = client.DB().ExecContext(ctx, "DROP USER IF EXISTS `"+user+"`@`"+host+"`")
	})

	// Prep: create user + database on the server directly.
	if _, err := client.DB().ExecContext(ctx, "CREATE USER `"+user+"`@`"+host+"` IDENTIFIED BY 'pw'"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := client.DB().ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"); err != nil {
		t.Fatalf("create db: %v", err)
	}

	// Apply a schema-level grant.
	r := grantResource(map[string]provider.Value{
		"user":       provider.StringVal(user),
		"host":       provider.StringVal(host),
		"database":   provider.StringVal(dbName),
		"table":      provider.StringVal("*"),
		"privileges": stringList("SELECT", "INSERT"),
	})
	r, _ = h.Normalize(ctx, r)
	if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
		t.Fatalf("create grant: %v", err)
	}

	// Discover — should find our grant among others (e.g. the default
	// USAGE grant every user has).
	resources, err := h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	found := findGrant(resources, user, host, dbName, "*")
	if found == nil {
		t.Fatalf("discover did not return the schema-level grant")
	}
	privs := getStringListField(found.Body, "privileges")
	if !containsAll(privs, []string{"INSERT", "SELECT"}) {
		t.Errorf("discovered privileges = %v, want at least [INSERT SELECT]", privs)
	}

	// Update: add UPDATE.
	updated := grantResource(map[string]provider.Value{
		"user":       provider.StringVal(user),
		"host":       provider.StringVal(host),
		"database":   provider.StringVal(dbName),
		"table":      provider.StringVal("*"),
		"privileges": stringList("SELECT", "INSERT", "UPDATE"),
	})
	updated, _ = h.Normalize(ctx, updated)
	if err := h.Apply(ctx, client, provider.OpUpdate, updated); err != nil {
		t.Fatalf("update grant: %v", err)
	}
	resources, _ = h.Discover(ctx, client)
	found = findGrant(resources, user, host, dbName, "*")
	if found == nil {
		t.Fatal("grant disappeared after update")
	}
	privs = getStringListField(found.Body, "privileges")
	if !containsAll(privs, []string{"INSERT", "SELECT", "UPDATE"}) {
		t.Errorf("after update privileges = %v, want [INSERT SELECT UPDATE]", privs)
	}

	// Delete.
	if err := h.Apply(ctx, client, provider.OpDelete, updated); err != nil {
		t.Fatalf("delete grant: %v", err)
	}
	resources, _ = h.Discover(ctx, client)
	if findGrant(resources, user, host, dbName, "*") != nil {
		t.Errorf("grant still present after delete")
	}
}

// TestGrantHandler_GrantOption covers WITH GRANT OPTION round-trip.
func TestGrantHandler_GrantOption(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	h := &grantHandler{}

	user := "dsctl_grant_option_user"
	dbName := "dsctl_grant_option_db"
	host := "%"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP DATABASE IF EXISTS `"+dbName+"`")
		_, _ = client.DB().ExecContext(ctx, "DROP USER IF EXISTS `"+user+"`@`"+host+"`")
	})
	if _, err := client.DB().ExecContext(ctx, "CREATE USER `"+user+"`@`"+host+"` IDENTIFIED BY 'pw'"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := client.DB().ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"); err != nil {
		t.Fatalf("create db: %v", err)
	}

	r := grantResource(map[string]provider.Value{
		"user":         provider.StringVal(user),
		"host":         provider.StringVal(host),
		"database":     provider.StringVal(dbName),
		"table":        provider.StringVal("*"),
		"privileges":   stringList("SELECT"),
		"grant_option": provider.BoolVal(true),
	})
	r, _ = h.Normalize(ctx, r)
	if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
		t.Fatalf("create: %v", err)
	}
	resources, _ := h.Discover(ctx, client)
	found := findGrant(resources, user, host, dbName, "*")
	if found == nil {
		t.Fatal("grant not found")
	}
	if !getBodyBool(found.Body, "grant_option") {
		t.Error("grant_option = false, want true")
	}
}

// grantResource builds a test mysql_grant resource.
func grantResource(body map[string]provider.Value) provider.Resource {
	om := provider.NewOrderedMap()
	for k, v := range body {
		om.Set(k, v)
	}
	return provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_grant", Name: "test_grant"},
		Body: om,
	}
}

// stringList builds a ListVal of StringVals.
func stringList(ss ...string) provider.Value {
	elems := make([]provider.Value, len(ss))
	for i, s := range ss {
		elems[i] = provider.StringVal(s)
	}
	return provider.ListVal(elems)
}

// findGrant locates a discovered grant by identity tuple.
func findGrant(rs []provider.Resource, user, host, database, table string) *provider.Resource {
	for i := range rs {
		r := &rs[i]
		if getBodyString(r.Body, "user") == user &&
			getBodyString(r.Body, "host") == host &&
			getBodyString(r.Body, "database") == database &&
			getBodyString(r.Body, "table") == table {
			return r
		}
	}
	return nil
}

// containsAll reports whether haystack contains every item in needles.
func containsAll(haystack, needles []string) bool {
	set := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}
