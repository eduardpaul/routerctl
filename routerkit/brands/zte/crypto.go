package zte

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// hashPassword is the login digest the web UI sends:
// hex(sha256(password + loginToken)), see g_loginToken() in the login page.
func hashPassword(password, token string) string {
	sum := sha256.Sum256([]byte(password + token))
	return hex.EncodeToString(sum[:])
}

// tokenRE finds `_sessionTmpToken = "..."` assignments in UI pages. The
// value is written with \xNN escapes.
var tokenRE = regexp.MustCompile(`_sessionTmpToken\s*=\s*"([^"]*)"`)

// extractToken returns the last _sessionTmpToken assigned in a page.
func extractToken(page []byte) string {
	all := tokenRE.FindAllSubmatch(page, -1)
	for i := len(all) - 1; i >= 0; i-- {
		if tok := unescapeJS(string(all[i][1])); tok != "" {
			return tok
		}
	}
	return ""
}

// unescapeJS decodes the \xNN / \uNNNN escapes of a JS string literal.
func unescapeJS(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		switch s[i+1] {
		case 'x':
			if i+3 < len(s) {
				if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
					b.WriteByte(byte(v))
					i += 3
					continue
				}
			}
		case 'u':
			if i+5 < len(s) {
				if v, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
					b.WriteRune(rune(v))
					i += 5
					continue
				}
			}
		default:
			b.WriteByte(s[i+1])
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// aesKeyIV derives what getDecodeValue() uses: key = SHA256(token) (AES-256),
// IV = first 16 bytes of SHA256(reverse(token)).
func aesKeyIV(token string) (key, iv []byte) {
	k := sha256.Sum256([]byte(token))
	r := []rune(token)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	v := sha256.Sum256([]byte(string(r)))
	return k[:], v[:aes.BlockSize]
}

// errDecode is returned when a value cannot be decrypted.
var errDecode = errors.New("zte: cannot decode value")

// decodeParaValue reverses the UI's encodeParaValue(): base64 of
// AES-256-CBC with zero padding.
func decodeParaValue(token, src string) (string, error) {
	if src == "" {
		return "", nil
	}
	if token == "" {
		return "", fmt.Errorf("%w: no session token", errDecode)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src))
	if err != nil || len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return "", fmt.Errorf("%w: not an encrypted value", errDecode)
	}
	key, iv := aesKeyIV(token)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errDecode
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, raw)
	return string(bytes.TrimRight(out, "\x00")), nil
}

// encodeParaValue is the UI's encodeParaValue() (used by tests and the
// fake server; the library itself never posts configuration).
func encodeParaValue(token, src string) string {
	key, iv := aesKeyIV(token)
	block, _ := aes.NewCipher(key)
	pad := aes.BlockSize - len(src)%aes.BlockSize
	if pad == aes.BlockSize && len(src) > 0 {
		pad = 0
	}
	buf := append([]byte(src), make([]byte, pad)...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(buf, buf)
	return base64.StdEncoding.EncodeToString(buf)
}

// secretName reports whether a parameter holds a credential that must not
// leave the library unless Options.KeepSecrets is set.
func secretName(name string) bool {
	n := strings.ToLower(name)
	switch {
	case n == "username", n == "wpspin", n == "appin":
		return true
	case strings.HasPrefix(n, "wepkey"):
		return !strings.HasSuffix(n, "index")
	}
	for _, s := range []string{"password", "passwd", "passphrase", "pwd", "psk", "secret", "presharedkey"} {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}
