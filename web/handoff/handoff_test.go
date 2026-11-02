package handoff_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/web/handoff"
)

// TestMain runs the tests on Ebitengine's main thread when the GPU half is
// built in; see gifttest.Main.
func TestMain(m *testing.M) { gifttest.Main(m) }

func mount(t *testing.T, root func(*gift.Context) gift.View, size geom.Size) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{Root: root, Size: size,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme()})
	h.Settle()
	return h
}

func view(v gift.View) func(*gift.Context) gift.View {
	return func(*gift.Context) gift.View { return v }
}

// await runs frames until cond holds, for the results of goroutines.
func await(t *testing.T, h *gifttest.Harness, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("no %s:\n%s", what, h.Dump())
		}
		time.Sleep(10 * time.Millisecond)
		h.Frame()
	}
	h.Settle()
}

func has(h *gifttest.Harness, s gifttest.Selector) func() bool {
	return func() bool { return len(h.FindAll(s)) > 0 }
}

var authorization = &oauth2.DeviceAuthResponse{
	DeviceCode:              "dev-1",
	UserCode:                "WDJB-MJHT",
	VerificationURI:         "https://idp.example/device",
	VerificationURIComplete: "https://idp.example/device?user_code=WDJB-MJHT",
}

func TestDeviceShowsTheCodeAndWhereToEnterIt(t *testing.T) {
	h := mount(t, view(handoff.Device(authorization)), geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("Sign in with your phone"))
	h.AssertExists(gifttest.ByText("idp.example/device"))
	h.AssertExists(gifttest.ByText("WDJB-MJHT"))
	h.AssertExists(gifttest.ByText("QR code to sign in"))
	h.AssertNone(gifttest.ByTextContains("Valid for"))
	h.AssertNoOverflow()

	// Beside each other where there is room, above each other where not.
	code := h.Find(gifttest.ByText("QR code to sign in")).LayoutBounds()
	title := h.Find(gifttest.ByText("Sign in with your phone")).LayoutBounds()
	if title.Min.X < code.Max.X {
		t.Fatalf("wide: code %v, title %v", code, title)
	}
	h.Resize(geom.Sz(360, 800))
	h.Settle()
	code = h.Find(gifttest.ByText("QR code to sign in")).LayoutBounds()
	title = h.Find(gifttest.ByText("Sign in with your phone")).LayoutBounds()
	if title.Min.Y < code.Max.Y || code.Max.X > 360 {
		t.Fatalf("narrow: code %v, title %v", code, title)
	}
	h.AssertNoOverflow()
}

func TestDeviceCountsDownAndExpires(t *testing.T) {
	da := *authorization
	da.Expiry = time.Now().Add(15 * time.Minute)
	h := mount(t, view(handoff.Device(&da)), geom.Sz(800, 500))
	text := h.Find(gifttest.ByTextContains("Valid for")).Text()
	if !regexp.MustCompile(`^Valid for 1[45]:\d\d\.$`).MatchString(text) {
		t.Fatalf("countdown %q", text)
	}

	da.Expiry = time.Now().Add(-time.Second)
	restarts := 0
	h = mount(t, view(handoff.Device(&da).OnRestart(func() { restarts++ })), geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("The code has expired."))
	h.AssertNone(gifttest.ByText("QR code to sign in"))
	h.Find(gifttest.ByText("New code")).Click()
	if restarts != 1 {
		t.Fatalf("%d restarts", restarts)
	}
	// Without OnRestart, no button.
	h = mount(t, view(handoff.Device(&da)), geom.Sz(800, 500))
	h.AssertNone(gifttest.ByText("New code"))
}

func TestDeviceTicksIntoExpired(t *testing.T) {
	da := *authorization
	da.Expiry = time.Now().Add(1200 * time.Millisecond)
	h := mount(t, view(handoff.Device(&da)), geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("Valid for 0:02."))
	await(t, h, "second", has(h, gifttest.ByText("Valid for 0:01.")))
	await(t, h, "expiry", has(h, gifttest.ByText("The code has expired.")))
}

func TestTextsReplaceTheDefaults(t *testing.T) {
	h := mount(t, view(handoff.Device(authorization).Texts(handoff.Texts{SignInTitle: "Mit dem Handy anmelden"})),
		geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("Mit dem Handy anmelden"))
	h.AssertExists(gifttest.ByText("and enter this code:"))
}

func TestPayStates(t *testing.T) {
	const link = "https://pay.example/s/4711"
	h := mount(t, view(handoff.Pay(link).Amount("12,50 €")), geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("Pay with your phone"))
	h.AssertExists(gifttest.ByText("12,50 €"))
	h.AssertExists(gifttest.ByText("Waiting for the payment …"))
	h.AssertExists(gifttest.ByText("QR code to pay"))

	h = mount(t, view(handoff.Pay(link).Amount("12,50 €").State(handoff.Done)), geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("Paid. Thank you!"))
	h.AssertNone(gifttest.ByText("QR code to pay"))

	tries := 0
	h = mount(t, view(handoff.Pay(link).State(handoff.Failed).Message("Card declined.").OnRestart(func() { tries++ })),
		geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("Card declined."))
	h.Find(gifttest.ByText("Try again")).Click()
	if tries != 1 {
		t.Fatal("no retry")
	}

	h = mount(t, view(handoff.Pay(link).Expiry(time.Now().Add(-time.Minute))), geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("The code has expired."))

	h = mount(t, view(handoff.Pay("")), geom.Sz(800, 500))
	h.AssertExists(gifttest.ByText("Preparing the payment …"))
	h.AssertNone(gifttest.ByText("QR code to pay"))
}

// idp is an identity provider with the two endpoints of the device flow.
// Each token request answers with the next of answers, the last one again
// once they run out.
type idp struct {
	srv     *httptest.Server
	codes   atomic.Int32
	polls   atomic.Int32
	mu      sync.Mutex
	answers []string // "pending", "denied", "expired", "token"
	authErr bool
}

func newIDP(t *testing.T, answers ...string) *idp {
	p := &idp{answers: answers}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			if p.authErr {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized_client"})
				return
			}
			n := p.codes.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "dev", "user_code": "CODE-" + string(rune('A'+n-1)),
				"verification_uri": p.srv.URL + "/verify", "expires_in": 600, "interval": 1,
			})
		case "/token":
			p.polls.Add(1)
			p.mu.Lock()
			a := p.answers[0]
			if len(p.answers) > 1 {
				p.answers = p.answers[1:]
			}
			p.mu.Unlock()
			if a == "token" {
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "token_type": "Bearer"})
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			code := map[string]string{"pending": "authorization_pending", "denied": "access_denied",
				"expired": "expired_token"}[a]
			_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *idp) config() *oauth2.Config {
	// AuthStyleInParams: the default probes the token endpoint twice on the
	// first error, which would take two answers at a time.
	return &oauth2.Config{ClientID: "kiosk", Endpoint: oauth2.Endpoint{
		DeviceAuthURL: p.srv.URL + "/device", TokenURL: p.srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
}

func TestLoginSignsIn(t *testing.T) {
	p := newIDP(t, "pending", "token")
	var got *oauth2.Token
	h := mount(t, view(handoff.Login(p.config(), func(tok *oauth2.Token) { got = tok })), geom.Sz(800, 500))
	await(t, h, "code", has(h, gifttest.ByText("CODE-A")))
	h.AssertExists(gifttest.ByText(p.srv.URL[len("http://"):] + "/verify"))
	await(t, h, "sign-in", has(h, gifttest.ByText("Signed in")))
	if got == nil || got.AccessToken != "at-1" {
		t.Fatalf("token %v", got)
	}
}

func TestLoginDeclinedOffersToTryAgain(t *testing.T) {
	p := newIDP(t, "denied", "token")
	h := mount(t, view(handoff.Login(p.config(), nil)), geom.Sz(800, 500))
	await(t, h, "decline", has(h, gifttest.ByText("The sign-in was declined.")))
	h.Find(gifttest.ByText("Try again")).Click()
	await(t, h, "second code", has(h, gifttest.ByText("CODE-B")))
	await(t, h, "sign-in", has(h, gifttest.ByText("Signed in")))
	if n := p.codes.Load(); n != 2 {
		t.Fatalf("%d codes requested", n)
	}
}

func TestLoginExpired(t *testing.T) {
	p := newIDP(t, "expired")
	h := mount(t, view(handoff.Login(p.config(), nil)), geom.Sz(800, 500))
	await(t, h, "expiry", has(h, gifttest.ByText("The code has expired.")))
	h.AssertExists(gifttest.ByText("New code"))
}

func TestLoginReportsOtherErrors(t *testing.T) {
	p := newIDP(t, "token")
	p.authErr = true
	var reported error
	h := mount(t, view(handoff.Login(p.config(), nil).OnError(func(err error) { reported = err })), geom.Sz(800, 500))
	await(t, h, "failure", has(h, gifttest.ByText("Something went wrong.")))
	var re *oauth2.RetrieveError
	if !errors.As(reported, &re) || re.ErrorCode != "unauthorized_client" {
		t.Fatalf("reported %v", reported)
	}
	h.AssertExists(gifttest.ByText("Try again"))
}

func TestLoginStopsPollingWhenRemoved(t *testing.T) {
	p := newIDP(t, "pending")
	h := mount(t, func(ctx *gift.Context) gift.View {
		show := ctx.State("show", true)
		if !ctx.Read(show) {
			return ui.Text("gone")
		}
		return ui.VStack(
			ui.Button(ui.Text("Leave"), func() { show.Set(false) }),
			handoff.Login(p.config(), nil),
		)
	}, geom.Sz(800, 600))
	await(t, h, "first poll", func() bool { return p.polls.Load() > 0 })
	h.Find(gifttest.ByText("Leave")).Click()
	h.Settle()
	n := p.polls.Load()
	time.Sleep(2500 * time.Millisecond)
	if m := p.polls.Load(); m > n+1 {
		t.Fatalf("still polling: %d polls after removal, %d before", m, n)
	}
}

// TestTheDemosLookTheWayTheyLook pins the pictures of the component pages in
// both themes.
//
//	GIFT_UPDATE_GOLDEN=1 go test -tags giftgpu -run TestTheDemos ./web/handoff
func TestTheDemosLookTheWayTheyLook(t *testing.T) {
	for _, name := range []string{"device-login", "pay-handoff"} {
		d, ok := showcase.Lookup(name)
		if !ok {
			t.Fatalf("no demo %s", name)
		}
		for _, theme := range []struct {
			name string
			t    ui.Theme
		}{{"light", ui.LightTheme()}, {"dark", ui.DarkTheme()}} {
			h := gifttest.New(t, gifttest.Options{
				Root:  func(ctx *gift.Context) gift.View { return ui.Window(ui.VStack(d.View(ctx)).Padding(20)) },
				Size:  geom.Sz(float32(d.Width)+40, float32(d.Height)+40),
				Theme: theme.t,
				Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
			})
			h.Settle()
			h.AssertNoOverflow()
			h.AssertOpaque()
			h.Warm()
			h.AssertGolden(name + "-" + theme.name)
		}
	}
}
