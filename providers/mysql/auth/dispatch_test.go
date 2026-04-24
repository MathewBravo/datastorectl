package auth

import (
	"strings"
	"testing"
)

// TestValidateDeclared covers the validate-time rejections for every
// shape the DCL can produce. These run before any connection attempt,
// so they don't depend on Compare being implemented correctly.
func TestValidateDeclared(t *testing.T) {
	cases := []struct {
		name    string
		decl    Declared
		wantErr string // "" means no error expected
	}{
		{
			name: "password + caching_sha2 ok",
			decl: Declared{Plugin: PluginCachingSHA2, Cleartext: "pw"},
		},
		{
			name: "password + native ok",
			decl: Declared{Plugin: PluginNativePassword, Cleartext: "pw"},
		},
		{
			name: "password_hash + caching_sha2 ok",
			decl: Declared{Plugin: PluginCachingSHA2, Hash: "$A$005$aaaaaaaaaaaaaaaaaaaaBBBB"},
		},
		{
			name: "password_hash + native ok",
			decl: Declared{Plugin: PluginNativePassword, Hash: "*1234"},
		},
		{
			name: "aws_iam alone ok",
			decl: Declared{Plugin: PluginAWSIAM},
		},
		{
			name:    "both password and hash",
			decl:    Declared{Plugin: PluginCachingSHA2, Cleartext: "pw", Hash: "stored"},
			wantErr: "cannot set both",
		},
		{
			name:    "neither set with caching_sha2",
			decl:    Declared{Plugin: PluginCachingSHA2},
			wantErr: "requires either password or password_hash",
		},
		{
			name:    "neither set with native",
			decl:    Declared{Plugin: PluginNativePassword},
			wantErr: "requires either password or password_hash",
		},
		{
			name:    "password set with aws_iam",
			decl:    Declared{Plugin: PluginAWSIAM, Cleartext: "pw"},
			wantErr: "does not accept a local password",
		},
		{
			name:    "hash set with aws_iam",
			decl:    Declared{Plugin: PluginAWSIAM, Hash: "stored"},
			wantErr: "does not accept a local password",
		},
		{
			name:    "ldap plugin rejected",
			decl:    Declared{Plugin: "authentication_ldap_simple"},
			wantErr: "not supported in this release",
		},
		{
			name:    "pam plugin rejected",
			decl:    Declared{Plugin: "authentication_pam"},
			wantErr: "not supported in this release",
		},
		{
			name:    "empty plugin rejected",
			decl:    Declared{Plugin: ""},
			wantErr: "auth_plugin is required",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateDeclared(c.decl)
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("expected nil error, got: %v", err)
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

// TestCompare_AWSIAMAlwaysMatches asserts the aws_iam shape skips
// authentication_string comparison entirely — per ADR 0010, IAM users
// have server-delegated auth with no local password to diff.
func TestCompare_AWSIAMAlwaysMatches(t *testing.T) {
	cases := []string{"", "anything", "garbage-not-even-a-hash"}
	for _, stored := range cases {
		match, err := Compare(Declared{Plugin: PluginAWSIAM}, stored)
		if err != nil {
			t.Errorf("aws_iam should not error on stored=%q: %v", stored, err)
		}
		if !match {
			t.Errorf("aws_iam should always match; stored=%q", stored)
		}
	}
}

// TestCompare_PasswordHashByteCompare asserts the password_hash path
// does a pure byte comparison with no hashing.
func TestCompare_PasswordHashByteCompare(t *testing.T) {
	stored := "$A$005$abcdefghijklmnopqrstBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	if ok, err := Compare(Declared{Plugin: PluginCachingSHA2, Hash: stored}, stored); err != nil || !ok {
		t.Errorf("expected match for identical hashes, got ok=%v err=%v", ok, err)
	}
	if ok, _ := Compare(Declared{Plugin: PluginCachingSHA2, Hash: stored}, "different"); ok {
		t.Error("expected non-match for different stored")
	}
}

// TestCompare_NativePasswordCleartextRehash uses a known-good vector:
// the hash MySQL stores for "password" under mysql_native_password is
// *2470C0C06DEE42FD1618BB99005ADCA2EC9D1E19.
func TestCompare_NativePasswordCleartextRehash(t *testing.T) {
	stored := "*2470C0C06DEE42FD1618BB99005ADCA2EC9D1E19"
	decl := Declared{Plugin: PluginNativePassword, Cleartext: "password"}
	if ok, err := Compare(decl, stored); err != nil || !ok {
		t.Errorf("expected match for correct cleartext, got ok=%v err=%v", ok, err)
	}
	declWrong := Declared{Plugin: PluginNativePassword, Cleartext: "notthepassword"}
	if ok, _ := Compare(declWrong, stored); ok {
		t.Error("expected non-match for wrong cleartext")
	}
}

// TestCompare_CachingSHA2CleartextRehash uses the same fixture bytes
// captured from the live cluster in caching_sha2_test.go. If this test
// and the round-trip test agree, the dispatch is correctly wiring into
// ComputeCachingSHA2Hash.
func TestCompare_CachingSHA2CleartextRehash(t *testing.T) {
	// Reuse the "password" fixture from caching_sha2_test.go.
	stored := string(mustHex("2441243030352478500E6D7B4B075664056A54121946312511440554575349376D2E666F416875797948574F4E6452305952654C564D316B7876342F4374694A717349336C35"))
	if ok, err := Compare(Declared{Plugin: PluginCachingSHA2, Cleartext: "password"}, stored); err != nil || !ok {
		t.Errorf("expected match; ok=%v err=%v", ok, err)
	}
	if ok, _ := Compare(Declared{Plugin: PluginCachingSHA2, Cleartext: "wrong"}, stored); ok {
		t.Error("expected non-match for wrong cleartext")
	}
}

// TestCompare_MalformedStoredHashErrors asserts unparseable stored
// strings produce an error (not a silent mismatch). Gives operators a
// clear signal when the server state is corrupt.
func TestCompare_MalformedStoredHashErrors(t *testing.T) {
	decl := Declared{Plugin: PluginCachingSHA2, Cleartext: "pw"}
	if _, err := Compare(decl, "not a valid hash"); err == nil {
		t.Error("expected error on malformed stored hash")
	}
}
