package auth

import "testing"

// TestNativePasswordHash_KnownVectors verifies the mysql_native_password
// algorithm against reference outputs. The first two are the canonical
// vectors cited in the MySQL Community Server documentation:
//
//	password  →  *2470C0C06DEE42FD1618BB99005ADCA2EC9D1E19
//	test      →  *94BDCEBE19083CE2A1F959FD02F964C7AF4CFC29
//
// The remaining three were independently computed with:
//
//	printf '%s' "<pw>" | openssl dgst -sha1 -binary | openssl dgst -sha1 -hex
//
// giving a second reference implementation. If the algorithm here
// drifts, one of these will fail.
func TestNativePasswordHash_KnownVectors(t *testing.T) {
	cases := []struct {
		cleartext string
		expected  string
	}{
		{"password", "*2470C0C06DEE42FD1618BB99005ADCA2EC9D1E19"},
		{"test", "*94BDCEBE19083CE2A1F959FD02F964C7AF4CFC29"},
		{"datastorectl", "*4C2E8A11AE4B1FE9A01A5F752B8E8D354E288B55"},
		{"hunter2", "*58815970BE77B3720276F63DB198B1FA42E5CC02"},
		{"Let_me_in!2026", "*A502358028AC1F16C4FFDBF560B8F834F361C0B8"},
	}
	for _, c := range cases {
		got := NativePasswordHash(c.cleartext)
		if got != c.expected {
			t.Errorf("NativePasswordHash(%q) = %q, want %q", c.cleartext, got, c.expected)
		}
	}
}

// TestNativePasswordHash_EmptyInput asserts the MySQL convention that
// an empty password produces an empty authentication_string, matching
// the behaviour of CREATE USER ... IDENTIFIED BY ''.
func TestNativePasswordHash_EmptyInput(t *testing.T) {
	if got := NativePasswordHash(""); got != "" {
		t.Errorf("NativePasswordHash(\"\") = %q, want \"\"", got)
	}
}

// TestNativePasswordHash_Deterministic asserts the function is pure —
// same input always yields identical output. Drift here would quietly
// break the diff logic.
func TestNativePasswordHash_Deterministic(t *testing.T) {
	const pw = "some-password-123"
	first := NativePasswordHash(pw)
	for i := 0; i < 5; i++ {
		if got := NativePasswordHash(pw); got != first {
			t.Fatalf("non-deterministic: run %d got %q, want %q", i, got, first)
		}
	}
}

// TestNativePasswordHash_FormatShape asserts the output format is
// "*" + 40 uppercase hex characters (20 bytes from a single SHA-1
// round).
func TestNativePasswordHash_FormatShape(t *testing.T) {
	hash := NativePasswordHash("anything")
	if len(hash) != 41 {
		t.Fatalf("len = %d, want 41", len(hash))
	}
	if hash[0] != '*' {
		t.Errorf("prefix = %q, want '*'", hash[0])
	}
	for i := 1; i < len(hash); i++ {
		r := hash[i]
		isDigit := r >= '0' && r <= '9'
		isUpperHex := r >= 'A' && r <= 'F'
		if !isDigit && !isUpperHex {
			t.Errorf("char %d = %q, want uppercase hex digit", i, r)
		}
	}
}
