package auth

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// mustHex decodes a hex string or panics. Test helper only.
func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Ground-truth hashes captured from a live mysql:8.4 container running:
//
//	CREATE USER 't'@'%' IDENTIFIED WITH caching_sha2_password BY '<pw>';
//	SELECT HEX(authentication_string) FROM mysql.user WHERE User = 't';
//
// If caching_sha2 ever changes its storage format or iteration count
// default, one of these will fail and we'll know immediately.
var cachingSHA2Fixtures = []struct {
	cleartext string
	storedHex string
}{
	{
		cleartext: "password",
		storedHex: "2441243030352478500E6D7B4B075664056A54121946312511440554575349376D2E666F416875797948574F4E6452305952654C564D316B7876342F4374694A717349336C35",
	},
	{
		cleartext: "datastorectl",
		storedHex: "244124303035243F19405A5A22133B3872383E355E227A3B0133506C5353414A4C4533576F4575514945724368506E2F685263482E4F38466878333964586A7469657271332F",
	},
	{
		cleartext: "hunter2",
		storedHex: "244124303035240127504852680F55434726202C5776345C491801583676482E56394D7935575557517A55705737686859302F74525A507246313178706C48426C2E76354733",
	},
}

func TestExtractCachingSHA2Params_ValidHash(t *testing.T) {
	stored := string(mustHex(cachingSHA2Fixtures[0].storedHex))
	salt, iterations, err := ExtractCachingSHA2Params(stored)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iterations != 5000 {
		t.Errorf("iterations = %d, want 5000", iterations)
	}
	if len(salt) != 20 {
		t.Errorf("salt length = %d, want 20", len(salt))
	}
}

func TestExtractCachingSHA2Params_MalformedHashes(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"missing magic", "005$aaaaaaaaaaaaaaaaaaaa<hash>"},
		{"wrong magic", "$B$005$saltsaltsaltsaltsalt<hash>"},
		{"short iteration", "$A$05$saltsaltsaltsaltsalt<hash>"},
		{"non-numeric iteration", "$A$abc$saltsaltsaltsaltsalt<hash>"},
		{"missing salt delimiter", "$A$005saltsaltsaltsaltsalt"},
		{"salt too short", "$A$005$shortsalt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := ExtractCachingSHA2Params(c.input); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestComputeCachingSHA2Hash_RoundTrip(t *testing.T) {
	for _, fx := range cachingSHA2Fixtures {
		t.Run(fx.cleartext, func(t *testing.T) {
			stored := string(mustHex(fx.storedHex))
			salt, iterations, err := ExtractCachingSHA2Params(stored)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			recomputed, err := ComputeCachingSHA2Hash(fx.cleartext, salt, iterations)
			if err != nil {
				t.Fatalf("compute: %v", err)
			}
			if !bytes.Equal([]byte(recomputed), []byte(stored)) {
				t.Errorf("mismatch\n  recomputed = %x\n  expected   = %x",
					recomputed, stored)
			}
		})
	}
}

func TestComputeCachingSHA2Hash_WrongPasswordProducesDifferentHash(t *testing.T) {
	fx := cachingSHA2Fixtures[0]
	stored := string(mustHex(fx.storedHex))
	salt, iterations, err := ExtractCachingSHA2Params(stored)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wrong, err := ComputeCachingSHA2Hash("notthepassword", salt, iterations)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if wrong == stored {
		t.Error("hashing a wrong password against the same salt must not match the stored hash")
	}
}

func TestComputeCachingSHA2Hash_FormatShape(t *testing.T) {
	salt := bytes.Repeat([]byte{0x41}, 20)
	out, err := ComputeCachingSHA2Hash("any", salt, 5000)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if !strings.HasPrefix(out, "$A$005$") {
		t.Errorf("missing $A$005$ prefix: %q", out[:7])
	}
	if len(out) != 7+20+43 {
		t.Errorf("len = %d, want %d", len(out), 7+20+43)
	}
}
