// Deterministic masking engine (masking.py).
//
// Keyed HMAC-SHA256 backend used by identifier types to derive stable
// pseudo-values from real ones. Operates on raw bytes only - the identifier
// types own the conversion to and from their own representations.
//
// This engine exists to avoid leaking sensitive information in logs and
// during debugging. It does not guarantee that masked values are unique or
// non-reversible and provides no security guarantees.

package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync/atomic"
)

// Environment variable name for the mask key
const envKeyName = "ASUSROUTER_MASK_KEY"

// readKeyFromString mirrors `_read_key_from_string`.
func readKeyFromString(key string) []byte {
	// Try reading as hex
	if b, ok := pyFromHex(key); ok {
		return b
	}
	// Fall back to encoding if hex conversion fails
	return []byte(key)
}

// pyFromHex mirrors Python `bytes.fromhex`: ASCII whitespace is allowed
// between (not inside) byte pairs.
func pyFromHex(s string) ([]byte, bool) {
	out := []byte{}
	isSpace := func(c byte) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
	}
	for i := 0; i < len(s); {
		if isSpace(s[i]) {
			i++
			continue
		}
		if i+1 >= len(s) {
			return nil, false
		}
		b, err := hex.DecodeString(s[i : i+2])
		if err != nil {
			return nil, false
		}
		out = append(out, b[0])
		i += 2
	}
	return out, true
}

func randomKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("security: cannot generate a mask key: %v", err))
	}
	return key
}

// The key is swapped atomically, so readers always see a whole key
var key atomic.Pointer[[]byte]

func init() {
	// Try loading key from the environment variable
	if env, ok := os.LookupEnv(envKeyName); ok {
		k := readKeyFromString(env)
		key.Store(&k)
		return
	}
	// If there was no key to load, generate a new one for this runtime
	k := randomKey()
	key.Store(&k)
}

// ConfigureKey mirrors `configure_key`: set the key used for
// deterministic masking. A string is read as hex (or its UTF-8 bytes);
// nil (or any other type) generates a new random key.
func ConfigureKey(value any) {
	var k []byte
	switch v := value.(type) {
	case string:
		k = readKeyFromString(v)
	case []byte:
		k = append([]byte{}, v...)
	default:
		// Generate a new key
		k = randomKey()
	}
	key.Store(&k)
}

// HmacDigest mirrors `hmac_digest`: compute an HMAC-SHA256 digest of the
// data, with the configured key when k is nil.
func HmacDigest(data []byte, k []byte) []byte {
	if k == nil {
		k = *key.Load()
	}
	mac := hmac.New(sha256.New, k)
	mac.Write(data)
	return mac.Sum(nil)
}

// GetKeyHex mirrors `get_key_hex`: return current key as hex.
func GetKeyHex() string {
	return hex.EncodeToString(*key.Load())
}
