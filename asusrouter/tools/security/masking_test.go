package security

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

const testKey = "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f"

func TestReadKeyFromString(t *testing.T) {
	fromHex := func(s string) []byte { b, _ := hex.DecodeString(s); return b }
	cases := []struct {
		keyStr string
		key    []byte
	}{
		// even-length valid hex (lowercase)
		{testKey, fromHex(testKey)},
		// even-length valid hex (uppercase)
		{strings.ToUpper(testKey), fromHex(testKey)},
		// empty string -> fromhex('') -> b''
		{"", []byte{}},
		// plain non-hex string -> fallback to UTF-8 encoding
		{"my-secret-key", []byte("my-secret-key")},
		// invalid-hex characters (even length) -> fallback to encoding
		{strings.Repeat("zz", 16), []byte(strings.Repeat("zz", 16))},
		// odd-length string -> fromhex would raise -> fallback to encoding
		{"abc", []byte("abc")},
		// fromhex skips whitespace between byte pairs
		{"0f 0f", []byte{0x0f, 0x0f}},
		{"0 f", []byte("0 f")},
	}
	for _, c := range cases {
		if got := readKeyFromString(c.keyStr); !bytes.Equal(got, c.key) {
			t.Errorf("readKeyFromString(%q) = %x, want %x", c.keyStr, got, c.key)
		}
	}
}

func TestConfigureKey(t *testing.T) {
	old := GetKeyHex()
	defer ConfigureKey(old)

	// bytes set the exact key
	key := bytes.Repeat([]byte("13"), 32)
	ConfigureKey(key)
	if GetKeyHex() != hex.EncodeToString(key) {
		t.Fatal("bytes key")
	}
	// hex string
	ConfigureKey(strings.Repeat("13", 32))
	if GetKeyHex() != strings.Repeat("13", 32) {
		t.Fatal("hex key")
	}
	// plain string uses UTF-8 encoding
	ConfigureKey("my-secret-key")
	if GetKeyHex() != hex.EncodeToString([]byte("my-secret-key")) {
		t.Fatal("plain key")
	}
	// None generates a new key
	ConfigureKey(make([]byte, 32))
	k1 := GetKeyHex()
	ConfigureKey(nil)
	if GetKeyHex() == k1 {
		t.Fatal("new key not generated")
	}
}

func TestConfigureKeyAffectsDigest(t *testing.T) {
	old := GetKeyHex()
	defer ConfigureKey(old)

	data := []byte("payload")
	ConfigureKey(bytes.Repeat([]byte("A"), 32))
	d1 := HmacDigest(data, nil)
	ConfigureKey(bytes.Repeat([]byte("B"), 32))
	if bytes.Equal(HmacDigest(data, nil), d1) {
		t.Fatal("key change did not affect the digest")
	}
	// Same key -> same digest (deterministic)
	ConfigureKey(bytes.Repeat([]byte("A"), 32))
	if !bytes.Equal(HmacDigest(data, nil), d1) || !bytes.Equal(HmacDigest(data, nil), HmacDigest(data, nil)) {
		t.Fatal("digest not deterministic")
	}
}

func TestHmacDigestWithKey(t *testing.T) {
	key, _ := hex.DecodeString(testKey)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("payload"))
	if !bytes.Equal(HmacDigest([]byte("payload"), key), mac.Sum(nil)) {
		t.Fatal("digest mismatch")
	}
}

func TestMaskingThreadSafe(t *testing.T) {
	old := GetKeyHex()
	defer ConfigureKey(old)

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 50 {
				ConfigureKey(bytes.Repeat([]byte{byte(i)}, 32))
			}
		}()
		go func() {
			defer wg.Done()
			for range 50 {
				if len(HmacDigest([]byte("payload"), nil)) != 32 {
					t.Error("bad digest")
				}
			}
		}()
	}
	wg.Wait()
}
