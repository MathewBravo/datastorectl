// Package auth implements the password hashing algorithms used by
// datastorectl's MySQL provider for stateless password diffing. The
// functions here are pure — no database, no network — so their
// correctness can be pinned by known-answer tests against reference
// vectors captured from real MySQL servers.
package auth

import (
	"crypto/sha1"
	"fmt"
	"strings"
)

// NativePasswordHash computes the mysql_native_password authentication
// string for the given cleartext. The algorithm is a double SHA-1
// with no salt:
//
//	"*" + UPPER(hex(SHA1(SHA1(password))))
//
// An empty input returns an empty string, matching MySQL's convention
// for passwordless accounts.
func NativePasswordHash(cleartext string) string {
	if cleartext == "" {
		return ""
	}
	first := sha1.Sum([]byte(cleartext))
	second := sha1.Sum(first[:])
	return "*" + strings.ToUpper(fmt.Sprintf("%x", second))
}
