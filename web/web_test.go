package web

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeBrowser replaces the system browser with a function of the test for
// the duration of it. The function is given the URL to open.
func fakeBrowser(t *testing.T, fn func(u string) error) {
	t.Helper()
	old := browse
	browse = fn
	t.Cleanup(func() { browse = old })
}

// follow is a browser that goes to the authorization URL's redirect_uri, as
// an identity provider would send it there after the login, with the query.
func follow(t *testing.T, query string, seen chan<- *http.Response) func(string) error {
	return func(u string) error {
		au, err := url.Parse(u)
		if err != nil {
			return err
		}
		redirect := au.Query().Get("redirect_uri")
		go func() {
			res, err := noFollow.Get(redirect + "?" + query)
			if err != nil {
				t.Errorf("redirect: %v", err)
				return
			}
			if seen != nil {
				seen <- res
			}
		}()
		return nil
	}
}

// noFollow is a client that reports a redirect instead of following it.
var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func authFlow(redirects chan<- string) Flow {
	return Flow{
		Callbacks: []Callback{Loopback("/oauth")},
		Start: func(_ context.Context, redirect string) (string, error) {
			if redirects != nil {
				redirects <- redirect
			}
			return "https://idp.example/authorize?redirect_uri=" + url.QueryEscape(redirect), nil
		},
	}
}

func TestOpenLoopbackReturnsTheRedirect(t *testing.T) {
	seen := make(chan *http.Response, 1)
	fakeBrowser(t, follow(t, "code=abc&state=s1", seen))
	redirects := make(chan string, 1)
	ret, err := Open(t.Context(), authFlow(redirects))
	if err != nil {
		t.Fatal(err)
	}
	redirect := <-redirects
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/oauth") {
		t.Fatalf("redirect URI %q", redirect)
	}
	if got := ret.URL.String(); got != redirect+"?code=abc&state=s1" {
		t.Fatalf("return %q", got)
	}
	code, err := ret.AuthCode("s1")
	if err != nil || code != "abc" {
		t.Fatalf("AuthCode = %q, %v", code, err)
	}
	res := <-seen
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "return to the application") {
		t.Fatalf("landing %d %s", res.StatusCode, body)
	}
	if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers %v", res.Header)
	}
	// The server is gone once Open returned.
	if c, err := net.DialTimeout("tcp", strings.TrimPrefix(strings.TrimSuffix(redirect, "/oauth"), "http://"), time.Second); err == nil {
		c.Close()
		t.Fatal("loopback server still listening")
	}
}

func TestOpenLoopbackLanding(t *testing.T) {
	seen := make(chan *http.Response, 1)
	fakeBrowser(t, follow(t, "code=abc&state=s1", seen))
	f := authFlow(nil)
	f.Landing = "https://app.example/signed-in"
	if _, err := Open(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	res := <-seen
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != f.Landing {
		t.Fatalf("landing: %d %v", res.StatusCode, res.Header)
	}
}

func TestOpenLoopbackFormPost(t *testing.T) {
	fakeBrowser(t, func(u string) error {
		au, _ := url.Parse(u)
		go func() {
			res, err := http.PostForm(au.Query().Get("redirect_uri"), url.Values{"code": {"c2"}, "state": {"s2"}, "id_token": {"x.y.z"}})
			if err == nil {
				res.Body.Close()
			}
		}()
		return nil
	})
	ret, err := Open(t.Context(), authFlow(nil))
	if err != nil {
		t.Fatal(err)
	}
	if code, err := ret.AuthCode("s2"); err != nil || code != "c2" || ret.URL.Query().Get("id_token") != "x.y.z" {
		t.Fatalf("form_post return %v: %q, %v", ret.URL, code, err)
	}
}

func TestOpenLoopbackIgnoresOtherRequests(t *testing.T) {
	statuses := make(chan int, 3)
	fakeBrowser(t, func(u string) error {
		au, _ := url.Parse(u)
		redirect, _ := url.Parse(au.Query().Get("redirect_uri"))
		go func() {
			do := func(req *http.Request) {
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Errorf("%v", err)
					statuses <- 0
					return
				}
				res.Body.Close()
				statuses <- res.StatusCode
			}
			// Another path, a foreign host name, a wrong method.
			other := *redirect
			other.Path = "/favicon.ico"
			req, _ := http.NewRequest("GET", other.String(), nil)
			do(req)
			req, _ = http.NewRequest("GET", redirect.String()+"?code=evil&state=s", nil)
			req.Host = "rebound.example:" + redirect.Port()
			do(req)
			req, _ = http.NewRequest("DELETE", redirect.String(), nil)
			do(req)
			res, err := http.Get(redirect.String() + "?code=good&state=s")
			if err == nil {
				res.Body.Close()
			}
		}()
		return nil
	})
	ret, err := Open(t.Context(), authFlow(nil))
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := ret.AuthCode("s"); code != "good" {
		t.Fatalf("took %v", ret.URL)
	}
	want := []int{404, 421, 405}
	for i, w := range want {
		if got := <-statuses; got != w {
			t.Fatalf("request %d: status %d, want %d", i, got, w)
		}
	}
}

func TestOpenEndsWithItsContext(t *testing.T) {
	opened := make(chan string, 1)
	fakeBrowser(t, func(u string) error { opened <- u; return nil })
	paid := errors.New("paid")
	ctx, cancel := context.WithCancelCause(t.Context())
	redirects := make(chan string, 1)
	go func() {
		<-opened
		cancel(paid)
	}()
	_, err := Open(ctx, authFlow(redirects))
	if !errors.Is(err, paid) {
		t.Fatalf("err = %v, want the cause", err)
	}
	redirect := <-redirects
	if c, err := net.DialTimeout("tcp", strings.TrimSuffix(strings.TrimPrefix(redirect, "http://"), "/oauth"), time.Second); err == nil {
		c.Close()
		t.Fatal("loopback server still listening")
	}
}

func TestOpenWithoutCallbacksOpensAndWaits(t *testing.T) {
	opened := make(chan string, 1)
	fakeBrowser(t, func(u string) error { opened <- u; return nil })
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-opened
		cancel()
	}()
	var given string
	_, err := Open(ctx, Flow{Start: func(_ context.Context, redirect string) (string, error) {
		given = redirect
		return "https://pay.example/session/1", nil
	}})
	if !errors.Is(err, context.Canceled) || given != "" {
		t.Fatalf("err = %v, redirect %q", err, given)
	}
}

func TestOpenErrors(t *testing.T) {
	fakeBrowser(t, func(string) error { return nil })
	if _, err := Open(t.Context(), Flow{Callbacks: []Callback{Loopback("/")}}); err == nil {
		t.Fatal("a Flow without Start opened")
	}
	_, err := Open(t.Context(), Flow{
		Callbacks: []Callback{Scheme("com.example.app", "/oauth"), HTTPS("app.example", "/oauth")},
		Start:     func(context.Context, string) (string, error) { return "https://idp.example", nil },
	})
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "com.example.app:/oauth, https://app.example/oauth") {
		t.Fatalf("err = %v", err)
	}
	startErr := errors.New("no session")
	f := authFlow(nil)
	f.Start = func(context.Context, string) (string, error) { return "", startErr }
	if _, err := Open(t.Context(), f); !errors.Is(err, startErr) {
		t.Fatalf("err = %v", err)
	}
	fakeBrowser(t, func(string) error { return ErrUnsupported })
	if _, err := Open(t.Context(), authFlow(nil)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

// scheme is a provider for private schemes, as a mobile host would bring.
type scheme struct{ ret string }

func (scheme) Supports(c Callback) bool { return c.Kind() == SchemeCallback }

func (s scheme) Run(_ context.Context, f Flow, c Callback) (Return, error) {
	u, _ := url.Parse(c.String() + "?" + s.ret)
	return Return{URL: u}, nil
}

func TestOpenTakesTheFirstSupportedCallback(t *testing.T) {
	defer func(n int) {
		providers.mu.Lock()
		providers.list = providers.list[:n]
		providers.mu.Unlock()
	}(len(providers.list))
	fakeBrowser(t, follow(t, "code=loop&state=s", nil))
	Register(scheme{"code=scheme&state=s"})

	f := authFlow(nil)
	f.Callbacks = []Callback{Scheme("com.example.app", "oauth"), Loopback("/oauth")}
	ret, err := Open(t.Context(), f)
	if code, _ := ret.AuthCode("s"); err != nil || code != "scheme" {
		t.Fatalf("scheme first: %v, %v", ret.URL, err)
	}
	f.Callbacks = []Callback{Loopback("/oauth"), Scheme("com.example.app", "oauth")}
	ret, err = Open(t.Context(), f)
	if code, _ := ret.AuthCode("s"); err != nil || code != "loop" {
		t.Fatalf("loopback first: %v, %v", ret.URL, err)
	}
}

func TestAuthCode(t *testing.T) {
	ret := func(q string) Return {
		u, _ := url.Parse("http://127.0.0.1:1/oauth?" + q)
		return Return{URL: u}
	}
	if _, err := ret("code=a&state=other").AuthCode("s"); !errors.Is(err, ErrState) {
		t.Fatalf("foreign state: %v", err)
	}
	if _, err := ret("code=a").AuthCode("s"); !errors.Is(err, ErrState) {
		t.Fatalf("no state: %v", err)
	}
	_, err := ret("error=access_denied&error_description=The+user+declined&state=s").AuthCode("s")
	var ae *AuthError
	if !errors.As(err, &ae) || ae.Code != "access_denied" || err.Error() != "gift/web: access_denied: The user declined" {
		t.Fatalf("error response: %v", err)
	}
	if _, err := ret("state=s").AuthCode("s"); err == nil {
		t.Fatal("no code accepted")
	}
	if _, err := (Return{}).AuthCode("s"); err == nil {
		t.Fatal("no redirect accepted")
	}
}

func TestCallbacks(t *testing.T) {
	for _, tc := range []struct {
		c    Callback
		kind CallbackKind
		s    string
	}{
		{Loopback("oauth"), LoopbackCallback, "http://127.0.0.1:*/oauth"},
		{Loopback(""), LoopbackCallback, "http://127.0.0.1:*/"},
		{Scheme("Com.Example.App", "/cb"), SchemeCallback, "com.example.app:/cb"},
		{HTTPS("App.Example", "cb"), HTTPSCallback, "https://app.example/cb"},
		{Callback{}, 0, "none"},
	} {
		if tc.c.Kind() != tc.kind || tc.c.String() != tc.s {
			t.Errorf("%v: kind %d, %q", tc.c, tc.c.Kind(), tc.c.String())
		}
	}
	if c := HTTPS("app.example", "/cb"); c.Host() != "app.example" || c.Path() != "/cb" || c.URIScheme() != "https" {
		t.Errorf("accessors %v", c)
	}
}

func TestOpenURLChecksTheURL(t *testing.T) {
	var opened []string
	fakeBrowser(t, func(u string) error { opened = append(opened, u); return nil })
	for _, bad := range []string{"", "example.com/help", "-a /Applications/Calculator.app", "%zz"} {
		if err := OpenURL(bad); err == nil {
			t.Errorf("OpenURL(%q) accepted", bad)
		}
	}
	if err := OpenURL("https://example.com/help"); err != nil || len(opened) != 1 {
		t.Fatalf("OpenURL: %v, %v", err, opened)
	}
}
