package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// callerIdentity captures the authenticated MySQL principal. Fetched
// once during Configure and cached on the provider so self-lockout
// guards can run against stable values. GrantedRoles is the full set
// of inbound role edges from mysql.role_edges; DefaultRoles is the
// subset set as session defaults in mysql.default_roles.
type callerIdentity struct {
	User         string
	Host         string
	DefaultRoles []roleRef
	GrantedRoles []roleRef
}

// roleRef identifies a role by the server's (user, host) tuple.
type roleRef struct {
	User string
	Host string
}

// fetchCallerIdentity queries CURRENT_USER() and mysql.default_roles
// so the self-lockout guard has the three classification inputs it
// needs (caller identity, default role set).
func fetchCallerIdentity(ctx context.Context, db *sql.DB) (callerIdentity, error) {
	var raw string
	if err := db.QueryRowContext(ctx, "SELECT CURRENT_USER()").Scan(&raw); err != nil {
		return callerIdentity{}, fmt.Errorf("mysql: caller identity: %w", err)
	}
	// CURRENT_USER() returns "user@host_pattern".
	at := strings.LastIndex(raw, "@")
	if at < 0 {
		return callerIdentity{}, fmt.Errorf("mysql: unexpected CURRENT_USER() shape %q", raw)
	}
	caller := callerIdentity{User: raw[:at], Host: raw[at+1:]}

	rows, err := db.QueryContext(ctx, `
		SELECT DEFAULT_ROLE_USER, DEFAULT_ROLE_HOST
		FROM mysql.default_roles
		WHERE USER = ? AND HOST = ?
	`, caller.User, caller.Host)
	if err == nil {
		for rows.Next() {
			var ru, rh string
			if err := rows.Scan(&ru, &rh); err != nil {
				rows.Close()
				return callerIdentity{}, err
			}
			caller.DefaultRoles = append(caller.DefaultRoles, roleRef{User: ru, Host: rh})
		}
		rows.Close()
	}
	// If mysql.default_roles is missing (e.g. 5.7, unlikely given the
	// version gate but defensive), leave DefaultRoles empty.

	// Load the full set of inbound role edges so we can guard
	// cascade-style lockouts — DROP ROLE of a non-default role
	// silently revokes the role's privileges from the caller.
	edgeRows, err := db.QueryContext(ctx, `
		SELECT FROM_USER, FROM_HOST FROM mysql.role_edges
		WHERE TO_USER = ? AND TO_HOST = ?
	`, caller.User, caller.Host)
	if err == nil {
		for edgeRows.Next() {
			var ru, rh string
			if err := edgeRows.Scan(&ru, &rh); err != nil {
				edgeRows.Close()
				return callerIdentity{}, err
			}
			caller.GrantedRoles = append(caller.GrantedRoles, roleRef{User: ru, Host: rh})
		}
		edgeRows.Close()
	}
	return caller, nil
}

// classifyUserLockout returns true when r is a mysql_user delete that
// would drop the caller's own authenticated account.
func classifyUserLockout(r provider.Resource, caller callerIdentity) bool {
	if r.ID.Type != "mysql_user" {
		return false
	}
	u := getBodyString(r.Body, "user")
	h := getBodyString(r.Body, "host")
	return u == caller.User && h == caller.Host
}

// requiredPrivileges lists the grants the provider itself needs to
// keep operating. A revoke of any of these against the caller is a
// self-lockout. ALL is a superset so it always counts.
var requiredPrivileges = map[string]bool{
	"ALL":         true,
	"SELECT":      true, // mysql.user, mysql.db, mysql.role_edges reads
	"CREATE USER": true,
	"DROP ROLE":   true,
	"GRANT OPTION": true,
	"SYSTEM_USER": true,
	"RELOAD":      true, // for FLUSH PRIVILEGES (future)
}

// classifyGrantLockout returns true when r is a mysql_grant delete
// targeted at the caller that would revoke a privilege the provider
// needs to keep working. Strict: any intersection with the required
// set triggers the guard; operators use --allow-self-lockout when
// they really mean it.
func classifyGrantLockout(r provider.Resource, caller callerIdentity) bool {
	if r.ID.Type != "mysql_grant" {
		return false
	}
	u := getBodyString(r.Body, "user")
	h := getBodyString(r.Body, "host")
	if u != caller.User || h != caller.Host {
		return false
	}
	// Any scope touching mysql.* or global is definitionally risky.
	// We already filtered to caller-targeted grants above.
	db := getBodyString(r.Body, "database")
	scopeRelevant := db == "*" || strings.EqualFold(db, "mysql")
	if !scopeRelevant {
		return false
	}
	privs := getStringListField(r.Body, "privileges")
	for _, p := range privs {
		if requiredPrivileges[strings.ToUpper(p)] {
			return true
		}
	}
	return false
}

// classifyDefaultRoleLockout returns true when r is a mysql_role
// delete whose name matches one of the caller's default roles.
func classifyDefaultRoleLockout(r provider.Resource, caller callerIdentity) bool {
	if r.ID.Type != "mysql_role" {
		return false
	}
	name := roleNameFromID(r.ID.Name)
	for _, dr := range caller.DefaultRoles {
		if dr.User == name {
			return true
		}
	}
	return false
}

// classifyGrantedRoleCascadeLockout returns true when r is a
// mysql_role delete whose name matches a role granted to the caller
// but not set as a default role. Such a delete cascades through
// mysql.role_edges, silently revoking the role's privileges from the
// caller. Excludes default roles to avoid double-classification — the
// default-role case already covers them with a more specific reason.
func classifyGrantedRoleCascadeLockout(r provider.Resource, caller callerIdentity) bool {
	if r.ID.Type != "mysql_role" {
		return false
	}
	name := roleNameFromID(r.ID.Name)
	// Exclude default roles to keep classifications distinct.
	for _, dr := range caller.DefaultRoles {
		if dr.User == name {
			return false
		}
	}
	for _, gr := range caller.GrantedRoles {
		if gr.User == name {
			return true
		}
	}
	return false
}

// roleNameFromID strips any @host suffix Normalize may have applied.
func roleNameFromID(id string) string {
	if at := strings.Index(id, "@"); at >= 0 {
		return id[:at]
	}
	return id
}

// GuardDeletes implements provider.DeleteGuarder. It runs the three
// classification functions against every planned delete and returns a
// DeleteGuard for each match with a diagnostic-quality Reason.
func (p *Provider) GuardDeletes(_ context.Context, deletes []provider.Resource) ([]provider.DeleteGuard, dcl.Diagnostics) {
	var guards []provider.DeleteGuard
	for _, r := range deletes {
		switch {
		case classifyUserLockout(r, p.caller):
			guards = append(guards, provider.DeleteGuard{
				Resource: r.ID,
				Reason: fmt.Sprintf("would delete caller %s@%s's own user account",
					p.caller.User, p.caller.Host),
			})
		case classifyGrantLockout(r, p.caller):
			privs := getStringListField(r.Body, "privileges")
			guards = append(guards, provider.DeleteGuard{
				Resource: r.ID,
				Reason: fmt.Sprintf("would revoke caller %s@%s's %s grants, blocking future plan or apply runs",
					p.caller.User, p.caller.Host, strings.Join(privs, ", ")),
			})
		case classifyDefaultRoleLockout(r, p.caller):
			guards = append(guards, provider.DeleteGuard{
				Resource: r.ID,
				Reason: fmt.Sprintf("would delete caller %s@%s's default role, breaking session auth after apply",
					p.caller.User, p.caller.Host),
			})
		case classifyGrantedRoleCascadeLockout(r, p.caller):
			guards = append(guards, provider.DeleteGuard{
				Resource: r.ID,
				Reason: fmt.Sprintf("would cascade-revoke role %q from caller %s@%s via mysql.role_edges",
					roleNameFromID(r.ID.Name), p.caller.User, p.caller.Host),
			})
		}
	}
	return guards, nil
}
