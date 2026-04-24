package auth

import (
	"errors"
	"fmt"
)

// Plugin names recognized by the mysql provider. These are the
// DCL-facing values for the `auth_plugin` attribute on mysql_user.
// PluginAWSIAM maps to the server-side AWSAuthenticationPlugin and has
// no local authentication_string.
const (
	PluginCachingSHA2    = "caching_sha2_password"
	PluginNativePassword = "mysql_native_password"
	PluginAWSIAM         = "aws_iam"
)

// unsupportedPlugins names the plugins we deliberately reject at
// validate time so users hit a clear diagnostic rather than a late
// apply-time failure. Non-exhaustive; anything not in the supported
// set falls through to a generic rejection.
var unsupportedPlugins = map[string]bool{
	"authentication_ldap_simple": true,
	"authentication_ldap_sasl":   true,
	"authentication_pam":         true,
	"authentication_kerberos":    true,
	"sha256_password":            true, // predecessor to caching_sha2, not targeting
}

// Declared is the password-side state of a mysql_user as declared in
// DCL. Exactly one of Cleartext / Hash is set for password plugins;
// both are empty for aws_iam. ValidateDeclared enforces these
// invariants.
type Declared struct {
	Plugin    string
	Cleartext string // from `password = secret(...)` — rehashed against stored salt
	Hash      string // from `password_hash = ...` — byte-compared to stored
}

// ValidateDeclared checks the plugin-vs-password invariants. Call this
// at validate time; errors here surface before any connection attempt.
func ValidateDeclared(d Declared) error {
	if d.Plugin == "" {
		return errors.New("auth_plugin is required")
	}
	if unsupportedPlugins[d.Plugin] {
		return fmt.Errorf("auth_plugin %q is not supported in this release", d.Plugin)
	}

	switch d.Plugin {
	case PluginAWSIAM:
		if d.Cleartext != "" || d.Hash != "" {
			return fmt.Errorf("auth_plugin %q does not accept a local password; remove password / password_hash", d.Plugin)
		}
		return nil

	case PluginCachingSHA2, PluginNativePassword:
		if d.Cleartext != "" && d.Hash != "" {
			return fmt.Errorf("cannot set both password and password_hash for auth_plugin %q", d.Plugin)
		}
		if d.Cleartext == "" && d.Hash == "" {
			return fmt.Errorf("auth_plugin %q requires either password or password_hash", d.Plugin)
		}
		return nil

	default:
		return fmt.Errorf("auth_plugin %q is not supported in this release", d.Plugin)
	}
}

// Compare returns true when the declared password state matches the
// server's stored authentication_string. Dispatches by plugin:
//
//   - aws_iam: always matches — server delegates auth, no local hash
//     to diff.
//   - password_hash set: byte-compare declared hash to stored.
//   - Cleartext + caching_sha2_password: parse salt from stored,
//     recompute via Drepper SHA-256 crypt, byte-compare the result.
//   - Cleartext + mysql_native_password: recompute via double-SHA1,
//     byte-compare.
//
// Malformed stored input produces an error (not a silent mismatch) so
// operators see something has gone wrong in server state.
func Compare(d Declared, stored string) (bool, error) {
	switch d.Plugin {
	case PluginAWSIAM:
		return true, nil

	case PluginCachingSHA2:
		if d.Hash != "" {
			return d.Hash == stored, nil
		}
		salt, iterations, err := ExtractCachingSHA2Params(stored)
		if err != nil {
			return false, err
		}
		recomputed, err := ComputeCachingSHA2Hash(d.Cleartext, salt, iterations)
		if err != nil {
			return false, err
		}
		return recomputed == stored, nil

	case PluginNativePassword:
		if d.Hash != "" {
			return d.Hash == stored, nil
		}
		return NativePasswordHash(d.Cleartext) == stored, nil

	default:
		return false, fmt.Errorf("auth_plugin %q is not supported for comparison", d.Plugin)
	}
}
