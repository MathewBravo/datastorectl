package opensearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchCallerIdentity_parses_response(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_plugins/_security/api/account" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"user_name": "admin",
			"backend_roles": ["admin", "ops"],
			"roles": ["all_access", "own_index"]
		}`))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "u", "p", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	id, err := fetchCallerIdentity(context.Background(), client)
	if err != nil {
		t.Fatalf("fetchCallerIdentity: %v", err)
	}
	if id.UserName != "admin" {
		t.Errorf("UserName = %q, want admin", id.UserName)
	}
	if len(id.BackendRoles) != 2 || id.BackendRoles[0] != "admin" || id.BackendRoles[1] != "ops" {
		t.Errorf("BackendRoles = %v, want [admin ops]", id.BackendRoles)
	}
}

func TestFetchCallerIdentity_error_on_non_2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	client, _ := NewClient(srv.URL, "u", "p", false)

	_, err := fetchCallerIdentity(context.Background(), client)
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status code: %v", err)
	}
}
