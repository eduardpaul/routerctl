package credentials

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
)

// Fake credentials only - never a real login
const (
	curUser = "curuser"
	curPass = "curpass"
)

func strp(s string) *string { return &s }

func refMD5(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func refB64(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

// fields mirrors `_fields`: parse_qs with keep_blank_values.
func fields(t *testing.T, body string) map[string]string {
	t.Helper()
	values, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", body, err)
	}
	out := map[string]string{}
	for k, v := range values {
		out[k] = v[0]
	}
	return out
}

// ---- test_models.py ----

func mustChpass(t *testing.T, newUsername, newPassword *string) string {
	t.Helper()
	body, err := BuildChpassRequest(curUser, curPass, newUsername, newPassword)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestBuildUsernameChange(t *testing.T) {
	f := fields(t, mustChpass(t, strp("brandnew"), nil))
	if f["cur_username"] != refMD5(curUser) || f["cur_passwd"] != refMD5(curPass) {
		t.Fatalf("current fields: %v", f)
	}
	if f["new_username"] != refB64("brandnew") || f["restart_httpd"] != "1" {
		t.Fatalf("fields: %v", f)
	}
	if _, ok := f["new_passwd"]; ok {
		t.Fatal("new_passwd present")
	}
}

func TestBuildPasswordChange(t *testing.T) {
	// Padding '=' in the base64 must survive the url-encoding round trip
	f := fields(t, mustChpass(t, nil, strp("s3cr3t/p+w")))
	if f["cur_username"] != refMD5(curUser) || f["cur_passwd"] != refMD5(curPass) {
		t.Fatalf("current fields: %v", f)
	}
	if f["new_passwd"] != refB64("s3cr3t/p+w") || f["restart_httpd"] != "1" {
		t.Fatalf("fields: %v", f)
	}
	if _, ok := f["new_username"]; ok {
		t.Fatal("new_username present")
	}
}

func TestBuildSpecialCharsAreQuoted(t *testing.T) {
	body := mustChpass(t, nil, strp("ûÿ"))
	expected := refB64("ûÿ")
	// The raw base64 (with + or /) must not appear verbatim in the body
	if strings.Contains(body, expected) {
		t.Fatalf("raw base64 %q in body %q", expected, body)
	}
	if fields(t, body)["new_passwd"] != expected {
		t.Fatal("new_passwd mismatch")
	}
}

func TestBuildBothChange(t *testing.T) {
	f := fields(t, mustChpass(t, strp("brandnew"), strp("s3cret")))
	if f["new_username"] != refB64("brandnew") || f["new_passwd"] != refB64("s3cret") ||
		f["cur_username"] != refMD5(curUser) || f["cur_passwd"] != refMD5(curPass) {
		t.Fatalf("fields: %v", f)
	}
}

// TestBuildChpassExactBody pins the byte-exact body (Python dict order).
func TestBuildChpassExactBody(t *testing.T) {
	body := mustChpass(t, strp("brandnew"), strp("s3cr3t/p+w"))
	want := "cur_username=" + refMD5(curUser) +
		"&cur_passwd=" + refMD5(curPass) +
		"&restart_httpd=1" +
		"&new_username=" + url.QueryEscape(refB64("brandnew")) +
		"&new_passwd=" + url.QueryEscape(refB64("s3cr3t/p+w"))
	if body != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestBuildRequiresAtLeastOneNewValue(t *testing.T) {
	_, err := BuildChpassRequest(curUser, curPass, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadChpassResult(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want ARCredentialsStatus
	}{
		{"200", `{"statusCode":"200"}`, ARCredentialsStatusSuccess},
		{"401", `{"statusCode":"401"}`, ARCredentialsStatusWrongPassword},
		{"402_dict", map[string]any{"statusCode": "402"}, ARCredentialsStatusLockedOut},
		{"999", `{"statusCode":"999"}`, ARCredentialsStatusUnknown},
		{"empty", "{}", ARCredentialsStatusUnknown},
		{"not_json", "not json", ARCredentialsStatusUnknown},
		{"list", "[1, 2, 3]", ARCredentialsStatusUnknown},
		{"none", nil, ARCredentialsStatusUnknown},
		{"nil_raw", (*string)(nil), ARCredentialsStatusUnknown},
		{"int", 42, ARCredentialsStatusUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReadChpassResult(tc.raw); got != tc.want {
				t.Fatalf("ReadChpassResult = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- test_legacy.py ----

func TestBuildLegacyRequest(t *testing.T) {
	body := BuildLegacyRequest("someuser", "s3cret/p+w")
	f := fields(t, body)
	want := map[string]string{
		"action_mode":   "apply",
		"action_script": "restart_time;restart_upnp;",
		"http_username": "someuser",
		// Plaintext (url-encoding round-trips), not hashed or base64
		"http_passwd":  "s3cret/p+w",
		"http_passwd2": "s3cret/p+w",
		"v_password2":  "s3cret/p+w",
	}
	for k, v := range want {
		if f[k] != v {
			t.Fatalf("%s = %q, want %q", k, f[k], v)
		}
	}
	// Byte-exact body (Python dict order, quote_plus)
	exact := "action_mode=apply&action_script=restart_time%3Brestart_upnp%3B" +
		"&http_username=someuser&http_passwd=s3cret%2Fp%2Bw" +
		"&http_passwd2=s3cret%2Fp%2Bw&v_password2=s3cret%2Fp%2Bw"
	if body != exact {
		t.Fatalf("body = %s, want %s", body, exact)
	}
}

// ---- test_validation.py ----

var strict = CredentialsPolicy{UsernameMax: DefaultMaxLength, PasswordMax: DefaultMaxLength, Strict: true}

// A password that satisfies every strict rule
const strictOK = "Str0ng!pwd"

func validate(newUsername, newPassword *string, policy *CredentialsPolicy) *string {
	if policy == nil {
		p := NewCredentialsPolicy()
		policy = &p
	}
	return ValidateCredentials(curUser, curPass, newUsername, newPassword, policy)
}

func TestValidation(t *testing.T) {
	short4u := NewCredentialsPolicy()
	short4u.UsernameMax = 4
	short4p := NewCredentialsPolicy()
	short4p.PasswordMax = 4

	cases := []struct {
		name        string
		newUsername *string
		newPassword *string
		policy      *CredentialsPolicy
		// nil: accepted; exact: whole error; else a fragment
		want     *string
		fragment bool
	}{
		{"accepts_valid_change", nil, strp("brandnewpass"), nil, nil, false},
		{"empty_username", strp(""), nil, nil, strp("must not be empty"), true},
		{"empty_password", nil, strp(""), nil, strp("must not be empty"), true},
		{"too_long_username", strp("toolong"), nil, &short4u, strp("username must be at most 4 characters"), false},
		{"too_long_password", nil, strp("toolong"), &short4p, strp("password must be at most 4 characters"), false},
		{"non_ascii_accent", nil, strp("café"), nil, strp("password must contain only ASCII characters"), false},
		{"non_ascii_cyrillic", nil, strp("пароль"), nil, strp("password must contain only ASCII characters"), false},
		{"non_ascii_nbsp", nil, strp("pass word"), nil, strp("password must contain only ASCII characters"), false},
		{"non_ascii_emoji", nil, strp("emoji\U0001f512"), nil, strp("password must contain only ASCII characters"), false},
		{"valid_username_good_name", strp("good_name"), nil, nil, nil, false},
		{"valid_username_a1-b_2", strp("a1-b_2"), nil, nil, nil, false},
		{"valid_username_Router", strp("Router"), nil, nil, nil, false},
		{"valid_username_x", strp("x"), nil, nil, nil, false},
		{"hyphen_first", strp("-lead"), nil, nil, strp("username may contain only"), true},
		{"underscore_first", strp("_lead"), nil, nil, strp("username may contain only"), true},
		{"space", strp("has space"), nil, nil, strp("username may contain only"), true},
		{"special", strp("with!bang"), nil, nil, strp("username may contain only"), true},
		{"non_ascii", strp("café"), nil, nil, strp("username may contain only"), true},
		{"trailing_newline", strp("trail\n"), nil, nil, strp("username may contain only"), true},
		{"username_equal_password", nil, strp(curUser), nil, strp("username and password must not be the same"), false},
		{"username_conflicts_with_current_password", strp(curPass), nil, nil, strp("username and password must not be the same"), false},
		{"non_strict_allows_weak_password", nil, strp("weak"), nil, nil, false},
		{"strict_short", nil, strp("Sh0rt!"), &strict, strp("at least 10"), true},
		{"strict_no_letter", nil, strp("1234567890!"), &strict, strp("at least one letter"), true},
		{"strict_no_number", nil, strp("abcdefghi!"), &strict, strp("at least one number"), true},
		{"strict_no_special", nil, strp("abcdefghi1"), &strict, strp("at least one special character"), true},
		{"strict_consecutive", nil, strp("aabcdef1!x"), &strict, strp("consecutive identical"), true},
		{"strict_accepts_compliant_password", nil, strp(strictOK), &strict, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validate(tc.newUsername, tc.newPassword, tc.policy)
			switch {
			case tc.want == nil:
				if got != nil {
					t.Fatalf("error = %q, want none", *got)
				}
			case got == nil:
				t.Fatalf("no error, want %q", *tc.want)
			case tc.fragment:
				if !strings.Contains(*got, *tc.want) {
					t.Fatalf("error = %q, want fragment %q", *got, *tc.want)
				}
			default:
				if *got != *tc.want {
					t.Fatalf("error = %q, want %q", *got, *tc.want)
				}
			}
		})
	}
}

// ---- test_enums.py ----

func TestEnumsRoundTrip(t *testing.T) {
	for _, m := range ARCredentialsStatusMembers {
		if got := ARCredentialsStatusFromValue(string(m)); got != m {
			t.Fatalf("%s: FromValue(%q) = %v", m.Name(), string(m), got)
		}
	}
	if got := ARCredentialsStatusFromValue("zzz-not-a-real-enum-value"); got != ARCredentialsStatusUnknown {
		t.Fatalf("unknown fallback = %v", got)
	}
	for code, want := range map[string]ARCredentialsStatus{
		"200": ARCredentialsStatusSuccess,
		"401": ARCredentialsStatusWrongPassword,
		"402": ARCredentialsStatusLockedOut,
	} {
		if got := ARCredentialsStatusFromValue(code); got != want {
			t.Fatalf("FromValue(%q) = %v, want %v", code, got, want)
		}
	}
}
