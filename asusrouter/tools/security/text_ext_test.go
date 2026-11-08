package security_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const extText = "pppd 2.4.7 started by fakeadmin, uid 0"

func mac(t *testing.T, v string) identifiers.MacAddress {
	t.Helper()
	m, err := identifiers.MacAddressFromValue(v)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSensitiveTextValue(t *testing.T) {
	if security.NewSensitiveText(extText).Value != extText {
		t.Fatal("value")
	}
}

func TestSensitiveTextStringRedacts(t *testing.T) {
	text := security.NewSensitiveText(extText)
	if text.PyStr() != security.RedactedStr || security.PyStrOf(text) != security.RedactedStr {
		t.Fatal("str")
	}
	repr := fmt.Sprintf("%#v", text)
	if !strings.Contains(repr, security.RedactedStr) || strings.Contains(repr, "pppd") {
		t.Fatalf("repr = %q", repr)
	}
	if strings.Contains(text.LogValue().String(), "pppd") {
		t.Fatal("log leak")
	}
}

func TestSensitiveTextDefaults(t *testing.T) {
	text := security.NewSensitiveText(extText)
	if text.RevealLevel() != security.ARSecurityLevelUnsafe || text.Maskable() {
		t.Fatal("defaults")
	}
}

func TestSensitiveTextRender(t *testing.T) {
	cases := []struct {
		reveal, level security.ARSecurityLevel
		revealed      bool
	}{
		{security.ARSecurityLevelUnsafe, security.ARSecurityLevelReasonable, false},
		{security.ARSecurityLevelUnsafe, security.ARSecurityLevelUnsafe, true},
		{security.ARSecurityLevelSanitized, security.ARSecurityLevelReasonable, true},
		{security.ARSecurityLevelSanitized, security.ARSecurityLevelDefault, false},
	}
	for _, c := range cases {
		rendered := security.Render(security.SensitiveText{Value: extText, Level: c.reveal}, c.level)
		if (rendered != security.Redacted) != c.revealed {
			t.Errorf("%v at %v: %v", c.reveal, c.level, rendered)
		}
	}
}

func TestSensitiveTextEquality(t *testing.T) {
	strict := security.SensitiveText{Value: extText, Level: security.ARSecurityLevelStrict}
	if security.NewSensitiveText(extText) != security.NewSensitiveText(extText) || security.NewSensitiveText(extText) == strict {
		t.Fatal("equality")
	}
}

func secrets(s ...security.ARSensitive) []security.ARSensitive { return s }

func TestRemoveSecrets(t *testing.T) {
	user := identifiers.NewUsername("fakeadmin")

	// A known secret is taken out of the text below the level
	cleaned := security.RemoveSecrets(extText, secrets(user), security.ARSecurityLevelReasonable)
	if strings.Contains(cleaned, "fakeadmin") || !strings.Contains(cleaned, security.RedactedStr) || !strings.HasPrefix(cleaned, "pppd 2.4.7 started by ") {
		t.Fatalf("below: %q", cleaned)
	}

	// At its own level the secret stays as the device wrote it
	if got := security.RemoveSecrets(extText, secrets(user), security.ARSecurityLevelUnsafe); got != extText {
		t.Fatalf("at level: %q", got)
	}

	// A secret is removed wherever it appears
	if got := security.RemoveSecrets("fakeadmin: started by fakeadmin", secrets(user), security.ARSecurityLevelStrict); strings.Contains(got, "fakeadmin") {
		t.Fatalf("every occurrence: %q", got)
	}

	// Every given secret is considered, each at its own level
	text := "fakeadmin used AA:BB:CC:DD:EE:FF"
	both := secrets(user, mac(t, "AA:BB:CC:DD:EE:FF"))
	cleaned = security.RemoveSecrets(text, both, security.ARSecurityLevelSanitized)
	if strings.Contains(cleaned, "fakeadmin") || strings.Contains(cleaned, "AA:BB:CC:DD:EE:FF") || !strings.Contains(cleaned, security.RedactedStr) {
		t.Fatalf("several: %q", cleaned)
	}
	cleaned = security.RemoveSecrets(text, both, security.ARSecurityLevelReasonable)
	if !strings.Contains(strings.ToLower(cleaned), "aa:bb:cc:dd:ee:ff") || strings.Contains(cleaned, "fakeadmin") {
		t.Fatalf("own level: %q", cleaned)
	}

	// Text without any of the secrets is untouched
	if security.RemoveSecrets(extText, nil, security.ARSecurityLevelStrict) != extText {
		t.Fatal("nothing to remove")
	}

	// A secret carrying no value cannot be searched for
	if security.RemoveSecrets(extText, secrets(identifiers.NewUsername("")), security.ARSecurityLevelStrict) != extText {
		t.Fatal("valueless")
	}
}

func TestRemoveSecretsMaskable(t *testing.T) {
	text := "fwver: x (sn:R2FAKE000000ABC)"
	serial := identifiers.NewSerial("R2FAKE000000ABC")

	// A secret that can be masked is replaced by its stand-in
	cleaned := security.RemoveSecrets(text, secrets(serial), security.ARSecurityLevelSanitized)
	if strings.Contains(cleaned, "R2FAKE000000ABC") || !strings.Contains(cleaned, "sn:sn-") {
		t.Fatalf("masked: %q", cleaned)
	}

	// Below the masking band even a maskable secret is withheld
	if cleaned := security.RemoveSecrets(text, secrets(serial), security.ARSecurityLevelStrict); !strings.Contains(cleaned, security.RedactedStr) {
		t.Fatalf("strict: %q", cleaned)
	}
}

func TestRemoveSecretsCaseInsensitive(t *testing.T) {
	text := "up C8:1F:01:02:03:04 down c8:1f:01:02:03:04"
	cleaned := security.RemoveSecrets(text, secrets(mac(t, "C8:1F:01:02:03:04")), security.ARSecurityLevelStrict)
	if strings.Contains(strings.ToLower(cleaned), "01:02:03:04") {
		t.Fatalf("got %q", cleaned)
	}
}

func TestRemoveSecretsReplacementIsLiteral(t *testing.T) {
	cleaned := security.RemoveSecrets("id AA:BB:CC:DD:EE:FF", secrets(mac(t, "AA:BB:CC:DD:EE:FF")), security.ARSecurityLevelStrict)
	if cleaned != "id "+security.RedactedStr {
		t.Fatalf("got %q", cleaned)
	}
}

func TestSearchable(t *testing.T) {
	// A type redacting its string form hands the value over instead
	if got := security.Searchable(identifiers.NewUsername("fakeadmin")); got == nil || *got != "fakeadmin" {
		t.Fatal("credential")
	}
	// A type without `value` prints itself
	if got := security.Searchable(mac(t, "AA:BB:CC:DD:EE:FF")); got == nil || *got != "aa:bb:cc:dd:ee:ff" {
		t.Fatal("identifier")
	}
	// Nothing to search for
	if security.Searchable(identifiers.NewUsername("")) != nil {
		t.Fatal("empty")
	}
}
