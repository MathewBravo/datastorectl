package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/MathewBravo/datastorectl/provider"
)

// databaseHandler manages mysql_database resources. A mysql_database
// corresponds to a server-side schema created via CREATE SCHEMA. The
// identifier comes from the DCL block label; the body carries
// charset and collation.
type databaseHandler struct{}

// systemSchemas lists schemas the server owns and the handler must
// never create, modify, or delete.
var systemSchemas = map[string]bool{
	"mysql":              true,
	"sys":                true,
	"performance_schema": true,
	"information_schema": true,
}

// Validate checks the rules that don't require a server round-trip:
// name non-empty, not a system schema, no backticks in the name.
func (h *databaseHandler) Validate(_ context.Context, r provider.Resource) error {
	name := r.ID.Name
	if name == "" {
		return fmt.Errorf("mysql_database name cannot be empty")
	}
	if systemSchemas[strings.ToLower(name)] {
		return fmt.Errorf("mysql_database %q is a reserved system schema and cannot be managed", name)
	}
	if strings.ContainsAny(name, "`") {
		return fmt.Errorf("mysql_database name %q contains a backtick, which is not a valid identifier character", name)
	}
	return nil
}

// Normalize is a no-op for mysql_database. Charset and collation come
// back from Discover in canonical form already; no further massaging
// is needed.
func (h *databaseHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	return r, nil
}

// Discover queries information_schema.SCHEMATA for user schemas. The
// four system schemas are filtered out.
func (h *databaseHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	rows, err := client.DB().QueryContext(ctx, `
		SELECT SCHEMA_NAME, DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME
		FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('mysql','sys','performance_schema','information_schema')
	`)
	if err != nil {
		return nil, fmt.Errorf("mysql_database discover: %w", err)
	}
	defer rows.Close()

	var out []provider.Resource
	for rows.Next() {
		var name, charset, collation string
		if err := rows.Scan(&name, &charset, &collation); err != nil {
			return nil, fmt.Errorf("mysql_database discover scan: %w", err)
		}
		body := provider.NewOrderedMap()
		body.Set("charset", provider.StringVal(charset))
		body.Set("collation", provider.StringVal(collation))
		out = append(out, provider.Resource{
			ID:   provider.ResourceID{Type: "mysql_database", Name: name},
			Body: body,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql_database discover iterate: %w", err)
	}
	return out, nil
}

// Apply executes the create/update/delete operation.
func (h *databaseHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate:
		return h.create(ctx, client.DB(), r)
	case provider.OpUpdate:
		return h.update(ctx, client.DB(), r)
	case provider.OpDelete:
		return h.delete(ctx, client.DB(), r)
	}
	return fmt.Errorf("mysql_database: unsupported operation %s", op)
}

func (h *databaseHandler) create(ctx context.Context, db *sql.DB, r provider.Resource) error {
	charset := getBodyString(r.Body, "charset")
	collation := getBodyString(r.Body, "collation")

	var stmt strings.Builder
	stmt.WriteString("CREATE SCHEMA `")
	stmt.WriteString(escapeBacktick(r.ID.Name))
	stmt.WriteString("`")
	if charset != "" {
		stmt.WriteString(" DEFAULT CHARACTER SET ")
		stmt.WriteString(charset)
	}
	if collation != "" {
		stmt.WriteString(" DEFAULT COLLATE ")
		stmt.WriteString(collation)
	}
	if _, err := db.ExecContext(ctx, stmt.String()); err != nil {
		return fmt.Errorf("mysql_database create %q: %w", r.ID.Name, err)
	}
	return nil
}

func (h *databaseHandler) update(ctx context.Context, db *sql.DB, r provider.Resource) error {
	charset := getBodyString(r.Body, "charset")
	collation := getBodyString(r.Body, "collation")

	var stmt strings.Builder
	stmt.WriteString("ALTER SCHEMA `")
	stmt.WriteString(escapeBacktick(r.ID.Name))
	stmt.WriteString("`")
	if charset != "" {
		stmt.WriteString(" DEFAULT CHARACTER SET ")
		stmt.WriteString(charset)
	}
	if collation != "" {
		stmt.WriteString(" DEFAULT COLLATE ")
		stmt.WriteString(collation)
	}
	if _, err := db.ExecContext(ctx, stmt.String()); err != nil {
		return fmt.Errorf("mysql_database update %q: %w", r.ID.Name, err)
	}
	return nil
}

func (h *databaseHandler) delete(ctx context.Context, db *sql.DB, r provider.Resource) error {
	stmt := "DROP SCHEMA `" + escapeBacktick(r.ID.Name) + "`"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_database delete %q: %w", r.ID.Name, err)
	}
	return nil
}

// escapeBacktick doubles any backtick in the identifier, the MySQL
// convention for escaping backticks inside backtick-quoted names.
// Validate rejects backticks outright, so this is defense-in-depth.
func escapeBacktick(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}

// getBodyString retrieves a string attribute from a resource body,
// returning "" when absent or of the wrong kind.
func getBodyString(m *provider.OrderedMap, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m.Get(key)
	if !ok || v.Kind != provider.KindString {
		return ""
	}
	return v.Str
}
