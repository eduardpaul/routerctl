package credentials

import (
	"context"
	"net/url"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// getter mirrors `_getter`: the current fake credentials.
type getter struct{ calls int }

func (g *getter) get() (string, string) {
	g.calls++
	return curUser, curPass
}

// rawPoster mirrors `_poster`: a raw poster returning a reply.
type rawPoster struct {
	reply *string
	err   error
	calls []struct {
		ep      endpoint.AREndpoint
		request string
	}
}

func (p *rawPoster) fetch(_ context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
	p.calls = append(p.calls, struct {
		ep      endpoint.AREndpoint
		request string
	}{ep, request})
	return p.reply, p.err
}

func (p *rawPoster) read(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
	return p.fetch(ctx, ep, request)
}

func chpassPoster(status string) *rawPoster {
	body := `{"statusCode":"` + status + `"}`
	return &rawPoster{reply: &body}
}

// sentFields mirrors `_sent_fields`.
func (p *rawPoster) sentFields(t *testing.T, ep endpoint.AREndpoint) map[string]string {
	t.Helper()
	if len(p.calls) != 1 {
		t.Fatalf("want one post, got %d", len(p.calls))
	}
	if p.calls[0].ep != ep {
		t.Fatalf("endpoint = %s, want %s", p.calls[0].ep, ep)
	}
	values, err := url.ParseQuery(p.calls[0].request)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k, v := range values {
		out[k] = v[0]
	}
	return out
}

// setter records credentials_set_callback calls.
type setter struct{ calls [][2]string }

func (s *setter) set(_ context.Context, u, p string) (bool, error) {
	s.calls = append(s.calls, [2]string{u, p})
	return true, nil
}

type identityOpts struct {
	usernameMax, passwordMax int
	strict, noChpass         bool
}

// identity mirrors `_identity`: advertising the given login capabilities.
func identity(o identityOpts) *device.ARDeviceIdentity {
	if o.usernameMax == 0 {
		o.usernameMax = 32
	}
	if o.passwordMax == 0 {
		o.passwordMax = 32
	}
	capabilities := map[ARCredentialsCapability]any{
		ARCredentialsCapabilityUsernameMaxLength: o.usernameMax,
		ARCredentialsCapabilityPasswordMaxLength: o.passwordMax,
	}
	if o.strict {
		capabilities[ARCredentialsCapabilitySecureDefault] = true
	}
	if !o.noChpass {
		capabilities[ARCredentialsCapabilityChpass] = true
	}
	return identityWith(map[support.ARSupportType]any{support.ARSupportTypeCredentialsCapabilities: capabilities})
}

func identityWith(sup map[support.ARSupportType]any) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(support.ARSupportSourceUniversal): sup,
	})
}

type runOpts struct {
	poster   *rawPoster
	noRaw    bool
	getter   *getter
	noGetter bool
	setter   *setter
	identity *device.ARDeviceIdentity
}

func runCreds(t *testing.T, a *ARCredentialsAction, o runOpts) bool {
	t.Helper()
	c := &registry.Context{Read: o.poster.read, Identity: o.identity}
	if !o.noRaw {
		c.FetchRaw = o.poster.fetch
	}
	if !o.noGetter {
		if o.getter == nil {
			o.getter = &getter{}
		}
		c.CredentialsGet = o.getter.get
	}
	if o.setter != nil {
		c.CredentialsSet = o.setter.set
	}
	got, err := runAction(context.Background(), c, a)
	if err != nil {
		t.Fatal(err)
	}
	return got.(service.ARServiceResult).Success
}

func assertSet(t *testing.T, s *setter, u, p string) {
	t.Helper()
	if len(s.calls) != 1 || s.calls[0] != [2]string{u, p} {
		t.Fatalf("setter calls = %v, want [%s %s]", s.calls, u, p)
	}
}

func TestPasswordChangeSuccess(t *testing.T) {
	p, s := chpassPoster("200"), &setter{}
	if !runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{})}) {
		t.Fatal("failed")
	}
	f := p.sentFields(t, endpoint.AREndpointChpass)
	if f["cur_username"] != refMD5(curUser) || f["cur_passwd"] != refMD5(curPass) || f["new_passwd"] != refB64("freshpass") {
		t.Fatalf("fields = %v", f)
	}
	if _, ok := f["new_username"]; ok {
		t.Fatal("new_username present")
	}
	// Password changed, username kept
	assertSet(t, s, curUser, "freshpass")
}

func TestUsernameChangeSuccess(t *testing.T) {
	p, s := chpassPoster("200"), &setter{}
	if !runCreds(t, &ARCredentialsAction{NewUsername: strp("freshuser")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{})}) {
		t.Fatal("failed")
	}
	f := p.sentFields(t, endpoint.AREndpointChpass)
	if f["new_username"] != refB64("freshuser") {
		t.Fatalf("fields = %v", f)
	}
	if _, ok := f["new_passwd"]; ok {
		t.Fatal("new_passwd present")
	}
	assertSet(t, s, "freshuser", curPass)
}

func TestBothChangeSuccess(t *testing.T) {
	p, s := chpassPoster("200"), &setter{}
	if !runCreds(t, &ARCredentialsAction{NewUsername: strp("freshuser"), NewPassword: strp("freshpass")},
		runOpts{poster: p, setter: s, identity: identity(identityOpts{})}) {
		t.Fatal("failed")
	}
	assertSet(t, s, "freshuser", "freshpass")
}

func TestFailureDoesNotResync(t *testing.T) {
	for _, status := range []string{"401", "402", "999"} {
		p, s := chpassPoster(status), &setter{}
		if runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{})}) {
			t.Fatalf("%s: success", status)
		}
		if len(s.calls) != 0 {
			t.Fatalf("%s: setter called", status)
		}
	}
}

func TestLostResponseDoesNotResync(t *testing.T) {
	p, s := &rawPoster{}, &setter{}
	if runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, setter: s}) {
		t.Fatal("success")
	}
	if len(s.calls) != 0 {
		t.Fatal("setter called")
	}
}

func TestConnectionDropReportsFailureWithoutSwap(t *testing.T) {
	for _, err := range []error{arerror.NewConnectionError("Server gone"), arerror.NewTimeoutError("Timeout")} {
		p, s := &rawPoster{err: err}, &setter{}
		if runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{})}) {
			t.Fatal("success")
		}
		if len(s.calls) != 0 {
			t.Fatal("setter called")
		}
	}
}

func TestSuccessWithoutSetterCallback(t *testing.T) {
	p := chpassPoster("200")
	if !runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, identity: identity(identityOpts{})}) {
		t.Fatal("failed")
	}
}

func TestNoGetterReturnsFailure(t *testing.T) {
	p := chpassPoster("200")
	if runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, noGetter: true}) {
		t.Fatal("success")
	}
	if len(p.calls) != 0 {
		t.Fatal("posted")
	}
}

func TestNoNewValueReturnsFailure(t *testing.T) {
	p, g := chpassPoster("200"), &getter{}
	if runCreds(t, &ARCredentialsAction{}, runOpts{poster: p, getter: g}) {
		t.Fatal("success")
	}
	if g.calls != 0 || len(p.calls) != 0 {
		t.Fatalf("getter = %d, posts = %d", g.calls, len(p.calls))
	}
}

func TestFallsBackToPrimaryCallback(t *testing.T) {
	p, s := chpassPoster("200"), &setter{}
	if !runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, noRaw: true, setter: s, identity: identity(identityOpts{})}) {
		t.Fatal("failed")
	}
	if len(p.calls) != 1 {
		t.Fatalf("posts = %d", len(p.calls))
	}
}

func TestRejectedBeforeSending(t *testing.T) {
	cases := []struct {
		name     string
		action   *ARCredentialsAction
		identity *device.ARDeviceIdentity
	}{
		{"over_max_length", &ARCredentialsAction{NewPassword: strp("waytoolongpassword")}, identity(identityOpts{passwordMax: 8})},
		{"username_equal_password", &ARCredentialsAction{NewPassword: strp(curUser)}, identity(identityOpts{})},
		{"strict_rejects_weak_password", &ARCredentialsAction{NewPassword: strp("weak")}, identity(identityOpts{strict: true})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, s := chpassPoster("200"), &setter{}
			if runCreds(t, tc.action, runOpts{poster: p, setter: s, identity: tc.identity}) {
				t.Fatal("success")
			}
			if len(p.calls) != 0 || len(s.calls) != 0 {
				t.Fatalf("posts = %d, sets = %d", len(p.calls), len(s.calls))
			}
		})
	}
}

func TestStrictPolicyAcceptsCompliantPassword(t *testing.T) {
	p, s := chpassPoster("200"), &setter{}
	if !runCreds(t, &ARCredentialsAction{NewPassword: strp("Str0ng!pwd")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{strict: true})}) {
		t.Fatal("failed")
	}
	if len(p.calls) != 1 {
		t.Fatalf("posts = %d", len(p.calls))
	}
	assertSet(t, s, curUser, "Str0ng!pwd")
}

func htmlPoster() *rawPoster {
	body := "<html>ok</html>"
	return &rawPoster{reply: &body}
}

func TestLegacyPasswordChange(t *testing.T) {
	p, s := htmlPoster(), &setter{}
	if !runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{noChpass: true})}) {
		t.Fatal("failed")
	}
	f := p.sentFields(t, endpoint.AREndpointStartApply)
	// Username unchanged -> current; password plaintext, not hashed
	if f["action_mode"] != "apply" || f["http_username"] != curUser || f["http_passwd"] != "freshpass" ||
		f["http_passwd2"] != "freshpass" || f["v_password2"] != "freshpass" {
		t.Fatalf("fields = %v", f)
	}
	exact := "action_mode=apply&action_script=restart_time%3Brestart_upnp%3B&http_username=curuser" +
		"&http_passwd=freshpass&http_passwd2=freshpass&v_password2=freshpass"
	if p.calls[0].request != exact {
		t.Fatalf("request = %s, want %s", p.calls[0].request, exact)
	}
	assertSet(t, s, curUser, "freshpass")
}

func TestLegacyUsernameChange(t *testing.T) {
	p, s := htmlPoster(), &setter{}
	if !runCreds(t, &ARCredentialsAction{NewUsername: strp("freshuser")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{noChpass: true})}) {
		t.Fatal("failed")
	}
	f := p.sentFields(t, endpoint.AREndpointStartApply)
	if f["http_username"] != "freshuser" || f["http_passwd"] != curPass {
		t.Fatalf("fields = %v", f)
	}
	assertSet(t, s, "freshuser", curPass)
}

func TestLegacyEmptyResponseFails(t *testing.T) {
	empty := ""
	p, s := &rawPoster{reply: &empty}, &setter{}
	if runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, setter: s, identity: identity(identityOpts{noChpass: true})}) {
		t.Fatal("success")
	}
	if len(s.calls) != 0 {
		t.Fatal("setter called")
	}
}

func TestFallsBackToLegacy(t *testing.T) {
	cases := []struct {
		name     string
		identity *device.ARDeviceIdentity
	}{
		{"no_identity", nil},
		{"non_dict_capabilities", identityWith(map[support.ARSupportType]any{support.ARSupportTypeCredentialsCapabilities: nil})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := htmlPoster()
			if !runCreds(t, &ARCredentialsAction{NewPassword: strp("freshpass")}, runOpts{poster: p, setter: &setter{}, identity: tc.identity}) {
				t.Fatal("failed")
			}
			if p.calls[0].ep != endpoint.AREndpointStartApply {
				t.Fatalf("endpoint = %s", p.calls[0].ep)
			}
		})
	}
}

func TestActionIsRegistered(t *testing.T) {
	if e := registry.Action(&ARCredentialsAction{NewPassword: strp("x")}); e == nil || e.Run == nil {
		t.Fatal("ARCredentialsAction is not registered")
	}
}

func TestActionReprHidesSecrets(t *testing.T) {
	a := &ARCredentialsAction{NewUsername: strp("secretuser"), NewPassword: strp("pw")}
	for _, s := range []string{a.String(), a.GoString(), a.LogValue().String(), action.Repr(a)} {
		if s != "<ARCredentialsAction>" {
			t.Fatalf("repr = %q", s)
		}
	}
}
