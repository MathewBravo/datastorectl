package auth

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
)

// MySQL stores caching_sha2_password authentication strings in a
// SHA-256 crypt variant:
//
//	$A$<iter/1000>$<20-byte salt><43-byte base64-like hash>
//
// The iteration field is 3 decimal digits representing rounds / 1000.
// Server default is 5000 rounds (stored as "005"). The salt is 20
// random raw bytes (may contain non-printable data). The hash is 43
// bytes of output encoded with the Drepper custom alphabet.
const (
	cachingSHA2Magic  = "$A$"
	cachingSHA2SaltLen = 20
	cachingSHA2HashLen = 43
)

// ExtractCachingSHA2Params parses a stored caching_sha2_password
// string and returns the salt and iteration count. The iteration value
// returned is the actual round count (e.g. 5000), not the stored
// divided-by-1000 form.
func ExtractCachingSHA2Params(stored string) (salt []byte, iterations int, err error) {
	if len(stored) < len(cachingSHA2Magic) || stored[:len(cachingSHA2Magic)] != cachingSHA2Magic {
		return nil, 0, errors.New("caching_sha2: missing $A$ magic prefix")
	}
	rest := stored[len(cachingSHA2Magic):]
	if len(rest) < 4 || rest[3] != '$' {
		return nil, 0, errors.New("caching_sha2: iteration field must be 3 digits followed by '$'")
	}
	mult, err := strconv.Atoi(rest[:3])
	if err != nil {
		return nil, 0, fmt.Errorf("caching_sha2: iteration count not numeric: %w", err)
	}
	rest = rest[4:]
	if len(rest) < cachingSHA2SaltLen {
		return nil, 0, fmt.Errorf("caching_sha2: salt must be %d bytes, got %d", cachingSHA2SaltLen, len(rest))
	}
	salt = []byte(rest[:cachingSHA2SaltLen])
	return salt, mult * 1000, nil
}

// ComputeCachingSHA2Hash derives the full caching_sha2_password stored
// string for the given cleartext, salt, and iteration count. Output is
// byte-identical to what MySQL 8.0+ stores in mysql.user.authentication_string
// for a user created with `IDENTIFIED WITH caching_sha2_password BY '<pw>'`.
//
// Algorithm is Ulrich Drepper's SHA-256 crypt scheme from glibc's
// crypt(3), adapted for MySQL's salt length (20 bytes) and custom magic
// prefix ($A$ instead of $5$).
func ComputeCachingSHA2Hash(cleartext string, salt []byte, iterations int) (string, error) {
	if len(salt) != cachingSHA2SaltLen {
		return "", fmt.Errorf("caching_sha2: salt must be %d bytes", cachingSHA2SaltLen)
	}
	if iterations < 1000 || iterations%1000 != 0 {
		return "", fmt.Errorf("caching_sha2: iteration count must be a multiple of 1000, got %d", iterations)
	}

	key := []byte(cleartext)
	hash := sha256crypt(key, salt, iterations)

	var out []byte
	out = append(out, cachingSHA2Magic...)
	out = append(out, fmt.Sprintf("%03d", iterations/1000)...)
	out = append(out, '$')
	out = append(out, salt...)
	out = append(out, hash...)
	return string(out), nil
}

// sha256crypt implements Drepper's SHA-256 crypt scheme. Returns 43
// bytes encoded in the custom base64 alphabet.
func sha256crypt(key, salt []byte, rounds int) []byte {
	// Step 1-3: hash B = SHA256(key || salt || key)
	b := sha256.New()
	b.Write(key)
	b.Write(salt)
	b.Write(key)
	bSum := b.Sum(nil)

	// Step 4: start hash A
	a := sha256.New()
	// Step 5: add key to A
	a.Write(key)
	// Step 6: add salt to A
	a.Write(salt)

	// Step 7: for each full 32-byte block of key length, add bSum.
	//         Remainder: add first N bytes of bSum.
	for i := len(key); i > 0; i -= 32 {
		if i >= 32 {
			a.Write(bSum)
		} else {
			a.Write(bSum[:i])
		}
	}

	// Step 8: walk bits of key length. 1 → add bSum. 0 → add key.
	for i := len(key); i > 0; i >>= 1 {
		if i&1 == 1 {
			a.Write(bSum)
		} else {
			a.Write(key)
		}
	}

	// Step 9: A result
	aSum := a.Sum(nil)

	// Step 10: DP = SHA256 of key repeated len(key) times
	dp := sha256.New()
	for i := 0; i < len(key); i++ {
		dp.Write(key)
	}
	dpSum := dp.Sum(nil)

	// Step 11: P = sequence of len(key) bytes by repeating dpSum
	p := make([]byte, len(key))
	for i := 0; i < len(key); i++ {
		p[i] = dpSum[i%32]
	}

	// Step 12: DS = SHA256 of salt repeated (16 + aSum[0]) times
	ds := sha256.New()
	count := 16 + int(aSum[0])
	for i := 0; i < count; i++ {
		ds.Write(salt)
	}
	dsSum := ds.Sum(nil)

	// Step 13: S = sequence of len(salt) bytes by repeating dsSum
	s := make([]byte, len(salt))
	for i := 0; i < len(salt); i++ {
		s[i] = dsSum[i%32]
	}

	// Step 14: iterate `rounds` times alternating digest compositions
	result := aSum
	for r := 0; r < rounds; r++ {
		c := sha256.New()
		if r%2 == 1 {
			c.Write(p)
		} else {
			c.Write(result)
		}
		if r%3 != 0 {
			c.Write(s)
		}
		if r%7 != 0 {
			c.Write(p)
		}
		if r%2 == 1 {
			c.Write(result)
		} else {
			c.Write(p)
		}
		result = c.Sum(nil)
	}

	// Step 15: encode as custom base64
	return encodeSHA256Crypt(result)
}

// drepperAlphabet is the 64-character alphabet used by SHA-256 crypt.
// Note the ordering differs from standard base64: dot, slash, 0-9, A-Z, a-z.
const drepperAlphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// encodeSHA256Crypt encodes the 32-byte SHA-256 digest into 43 bytes
// using Drepper's custom 3-byte-group permutation. The last group
// (bytes 30, 31, and a carry-in from byte 0's tail) is only 2-byte,
// producing the final 43rd character.
func encodeSHA256Crypt(digest []byte) []byte {
	// The permutation pattern from the Drepper spec for SHA-256 (32-byte digest).
	// Each triple (a, b, c) is processed to produce 4 output chars; final
	// pair (a, b, 0) produces 3 output chars — truncated to 43 total.
	groups := [][3]int{
		{0, 10, 20}, {21, 1, 11}, {12, 22, 2}, {3, 13, 23}, {24, 4, 14},
		{15, 25, 5}, {6, 16, 26}, {27, 7, 17}, {18, 28, 8}, {9, 19, 29},
	}
	out := make([]byte, 0, 43)
	for _, g := range groups {
		out = append(out, b64From24(digest[g[0]], digest[g[1]], digest[g[2]], 4)...)
	}
	// Final 2-byte group: bytes 31 and 30, 0 as the third input. 3 output chars.
	out = append(out, b64From24(0, digest[31], digest[30], 3)...)
	return out
}

// b64From24 encodes 24 bits (3 bytes) into `count` characters of the
// Drepper alphabet, LSB-first.
func b64From24(b2, b1, b0 byte, count int) []byte {
	w := uint32(b2)<<16 | uint32(b1)<<8 | uint32(b0)
	out := make([]byte, count)
	for i := 0; i < count; i++ {
		out[i] = drepperAlphabet[w&0x3f]
		w >>= 6
	}
	return out
}
