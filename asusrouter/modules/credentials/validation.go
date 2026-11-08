package credentials

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Fallback max length when the device does not report MaxLen_http_*
const DefaultMaxLength = 32

// Allowed login name: letters/digits, then also hyphen/underscore, but the
// first character may not be a hyphen or underscore. Anchored on both ends
// (Python fullmatch) so a trailing newline cannot slip through
var usernameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Strict (secure_default) password policy
const minStrictPasswordLength = 10

// Python string.punctuation
const specialChars = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// CredentialsPolicy mirrors `CredentialsPolicy`: device rules a
// credential change must satisfy.
type CredentialsPolicy struct {
	UsernameMax int  `json:"username_max"`
	PasswordMax int  `json:"password_max"`
	Strict      bool `json:"strict"`
}

// NewCredentialsPolicy mirrors `CredentialsPolicy()` with its defaults.
func NewCredentialsPolicy() CredentialsPolicy {
	return CredentialsPolicy{
		UsernameMax: DefaultMaxLength,
		PasswordMax: DefaultMaxLength,
		Strict:      false,
	}
}

// hasConsecutiveIdentical mirrors `_has_consecutive_identical`.
func hasConsecutiveIdentical(value string) bool {
	runes := []rune(value)
	for i := 1; i < len(runes); i++ {
		if runes[i-1] == runes[i] {
			return true
		}
	}
	return false
}

// validateLength mirrors `_validate_length`: check a value is non-empty
// and within the max length.
func validateLength(value string, maximum int, label string) *string {
	if value == "" {
		return errorf("%s must not be empty", label)
	}
	if utf8.RuneCountInString(value) > maximum {
		return errorf("%s must be at most %d characters", label, maximum)
	}
	return nil
}

// validateStrictPassword mirrors `_validate_strict_password`: apply the
// secure-default password rules; return an error or nil.
func validateStrictPassword(password string) *string {
	if utf8.RuneCountInString(password) < minStrictPasswordLength {
		return errorf("password must be at least %d characters", minStrictPasswordLength)
	}
	if !strings.ContainsFunc(password, unicode.IsLetter) {
		return errorf("password must contain at least one letter")
	}
	if !strings.ContainsFunc(password, pyIsDigit) {
		return errorf("password must contain at least one number")
	}
	if !strings.ContainsAny(password, specialChars) {
		return errorf("password must contain at least one special character")
	}
	if hasConsecutiveIdentical(password) {
		return errorf("password must not contain consecutive identical characters")
	}
	return nil
}

// validateUsername mirrors `_validate_username`: check a new login
// name's length and character set.
func validateUsername(username string, policy CredentialsPolicy) *string {
	if err := validateLength(username, policy.UsernameMax, "username"); err != nil {
		return err
	}
	if !usernameRE.MatchString(username) {
		return errorf("username may contain only letters, digits, hyphens and " +
			"underscores, and may not start with a hyphen or underscore")
	}
	return nil
}

// validatePassword mirrors `_validate_password`: check a new password's
// length and, when strict, its complexity.
func validatePassword(password string, policy CredentialsPolicy) *string {
	if err := validateLength(password, policy.PasswordMax, "password"); err != nil {
		return err
	}
	// The login payload is ASCII-encoded
	if !pyIsASCII(password) {
		return errorf("password must contain only ASCII characters")
	}
	if policy.Strict {
		return validateStrictPassword(password)
	}
	return nil
}

// ValidateCredentials mirrors `validate_credentials`: check a credential
// change against known device rules.
//
// Returns a human-readable error string for the first failed rule, or nil
// when the change should be accepted. Length limits and the username/password
// match are universal; the strict complexity rules apply only when the device
// reports the secure-default policy. A nil policy is `CredentialsPolicy()`.
func ValidateCredentials(
	curUsername string,
	curPassword string,
	newUsername *string,
	newPassword *string,
	policy *CredentialsPolicy,
) *string {
	p := NewCredentialsPolicy()
	if policy != nil {
		p = *policy
	}

	if newUsername != nil {
		if err := validateUsername(*newUsername, p); err != nil {
			return err
		}
	}

	if newPassword != nil {
		if err := validatePassword(*newPassword, p); err != nil {
			return err
		}
	}

	// The resulting login name and password must differ
	finalUsername := curUsername
	if newUsername != nil {
		finalUsername = *newUsername
	}
	finalPassword := curPassword
	if newPassword != nil {
		finalPassword = *newPassword
	}
	if finalUsername == finalPassword {
		return errorf("username and password must not be the same")
	}

	return nil
}

func errorf(format string, args ...any) *string {
	s := fmt.Sprintf(format, args...)
	return &s
}

// pyIsASCII mirrors Python `str.isascii`.
func pyIsASCII(value string) bool {
	for _, r := range value {
		if r >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// pyIsDigit mirrors Python `str.isdigit` for one character. The strict
// rules only run on ASCII passwords, where that is exactly 0-9.
func pyIsDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
