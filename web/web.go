package web

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// Callback is where a flow ends: the redirect URI the provider of the login or
// the payment sends the browser to, and which the platform catches. It is
// made by [Loopback], [Scheme] or [HTTPS].
type Callback struct {
	kind   CallbackKind
	scheme string
	host   string
	path   string
}

// CallbackKind tells the three kinds of [Callback] apart.
type CallbackKind uint8

// The kinds of [Callback].
const (
	// LoopbackCallback is a [Loopback] callback.
	LoopbackCallback CallbackKind = iota + 1
	// SchemeCallback is a [Scheme] callback.
	SchemeCallback
	// HTTPSCallback is an [HTTPS] callback.
	HTTPSCallback
)

// Loopback is a redirect to a server of the application's own on the
// loopback interface, http://127.0.0.1:port/path, on a port chosen when the
// flow starts. It is what RFC 8252 recommends for desktop applications and
// what every identity provider accepts for a native client, Google
// included; the port is free in the registration of the client.
//
// The system browser opens the flow and the application cannot close its tab
// afterwards, so the server answers the redirect with a page that says the
// user may close it, or sends the browser on to [Flow.Landing].
func Loopback(path string) Callback {
	return Callback{kind: LoopbackCallback, scheme: "http", path: cleanPath(path)}
}

// Scheme is a redirect to a private URI scheme of the application, such as
// "com.example.app:/oauth": a reverse domain name the application owns, as
// RFC 8252 asks. Only a platform that registers schemes for an application
// can catch it, an app on a phone or a macOS bundle; on the desktop it is
// not supported, see the package documentation.
func Scheme(scheme, path string) Callback {
	return Callback{kind: SchemeCallback, scheme: strings.ToLower(scheme), path: cleanPath(path)}
}

// HTTPS is a redirect to an https URL that the platform hands to the
// application instead of a browser: a universal link on iOS and macOS, an
// app link on Android. The domain has to say so, with an
// apple-app-site-association or an assetlinks.json file.
func HTTPS(host, path string) Callback {
	return Callback{kind: HTTPSCallback, scheme: "https", host: strings.ToLower(host), path: cleanPath(path)}
}

func cleanPath(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// Kind returns the kind of the callback; the zero Callback has none.
func (c Callback) Kind() CallbackKind { return c.kind }

// URIScheme returns the scheme of the redirect URI: "http" for a loopback,
// the application's own for a [Scheme], "https" for an [HTTPS].
func (c Callback) URIScheme() string { return c.scheme }

// Host returns the host of an [HTTPS] callback, and "" for the others.
func (c Callback) Host() string { return c.host }

// Path returns the path of the redirect URI, which begins with a slash.
func (c Callback) Path() string { return c.path }

// String returns the redirect URI, with the port of a loopback left open.
func (c Callback) String() string {
	switch c.kind {
	case LoopbackCallback:
		return "http://127.0.0.1:*" + c.path
	case SchemeCallback:
		return c.scheme + ":" + c.path
	case HTTPSCallback:
		return "https://" + c.host + c.path
	}
	return "none"
}

// Flow is a login or a payment in a browser that the application does not
// control: it starts at a URL and ends with a redirect to a [Callback].
type Flow struct {
	// Start returns the URL to open, once the redirect URI is known. A
	// loopback's port is chosen only now, and the URI has to be part of the
	// request, so the URL cannot be built before:
	//
	//   - OAuth: the authorization URL, with the redirect URI, PKCE, state
	//     and nonce; golang.org/x/oauth2's AuthCodeURL builds it.
	//   - Payment: the URL of the checkout session, which the application's
	//     backend creates with the redirect URI as the return URL.
	//
	// Without Callbacks, Start is given "" and the URL is opened without
	// waiting for a redirect.
	Start func(ctx context.Context, redirectURI string) (string, error)

	// Callbacks are the kinds of redirect the flow can end with, the
	// preferred first. [Open] takes the first one the platform supports;
	// they differ by platform, and a client registered with several of them
	// runs everywhere. Empty, the flow is only opened, and [Open] waits for
	// its ctx.
	Callbacks []Callback

	// Ephemeral asks for a browser session that keeps no cookies, so that
	// the next user of a shared device is not signed in. Only a platform
	// with a session of its own for the flow can honour it; the system
	// browser of a [Loopback] cannot.
	Ephemeral bool

	// Landing is the page a [Loopback] callback sends the browser on to,
	// once the redirect has arrived: the application's own "signed in,
	// return to the app" page, say. Empty, the loopback server answers with
	// a short page of its own.
	Landing string
}

// Return is the redirect that ended a flow, as it arrived. It is a signal,
// not a receipt: anyone can call a redirect URI. For a login,
// [Return.AuthCode] checks the state and the code is redeemed against the
// token endpoint; whether a payment succeeded, the application learns from
// its backend, which learns it from the payment provider's webhook.
type Return struct {
	// URL is the redirect URI with everything the provider appended.
	URL *url.URL
}

var (
	// ErrCanceled is returned by [Open] when the user closed the flow, on a
	// platform where that can be seen.
	ErrCanceled = errors.New("gift/web: flow closed by the user")
	// ErrUnsupported is returned by [Open] and [OpenURL] when the platform
	// has no way to run the flow: none of its callbacks is supported, or
	// there is no browser to open, as on a kiosk without one.
	ErrUnsupported = errors.New("gift/web: not supported on this platform")
	// ErrState is returned by [Return.AuthCode] for a redirect whose state is
	// not the one the flow was started with: a forged or a stale redirect.
	ErrState = errors.New("gift/web: redirect with a foreign state")
)

// Open runs f and waits for it to end: for the redirect, for the user to
// close it where that can be seen, or for ctx to end, whose cause it then
// returns. It closes what it opened, the loopback server for one; a system
// browser's tab stays.
//
// It blocks, and so it is called from a goroutine of its own, never from a
// build or an event handler; the result goes back to the interface as every
// asynchronous result does:
//
//	tok := ctx.Request()
//	go func() {
//		ret, err := web.Open(context.Background(), flow)
//		tok.Post(func() { result.Set(...) })
//	}()
//
// Cancelling ctx is how an application ends a flow it no longer waits for,
// a payment its backend has seen paid, say, or a screen the user left.
func Open(ctx context.Context, f Flow) (Return, error) {
	if f.Start == nil {
		return Return{}, errors.New("gift/web: a Flow needs a Start")
	}
	if len(f.Callbacks) == 0 {
		u, err := f.Start(ctx, "")
		if err != nil {
			return Return{}, err
		}
		if err := OpenURL(u); err != nil {
			return Return{}, err
		}
		<-ctx.Done()
		return Return{}, context.Cause(ctx)
	}
	for _, c := range f.Callbacks {
		if p := providerFor(c); p != nil {
			return p.Run(ctx, f, c)
		}
	}
	names := make([]string, len(f.Callbacks))
	for i, c := range f.Callbacks {
		names[i] = c.String()
	}
	return Return{}, fmt.Errorf("%w: no mechanism for the callbacks %s", ErrUnsupported, strings.Join(names, ", "))
}

// OpenURL opens u in the system's browser and returns, without waiting for
// anything: a help page, the terms, a receipt. u must be an absolute URL.
// Where there is no browser to open, as on a kiosk, the error wraps
// [ErrUnsupported].
func OpenURL(u string) error {
	p, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("gift/web: %w", err)
	}
	if p.Scheme == "" || strings.HasPrefix(u, "-") {
		return fmt.Errorf("gift/web: %q is not an absolute URL", u)
	}
	return browse(u)
}

// Provider is a mechanism that runs flows: the loopback server with the
// system browser of this package, and the ones of packages that register
// themselves when they are imported.
type Provider interface {
	// Supports reports whether the provider can catch c on this machine.
	Supports(c Callback) bool
	// Run runs f to the redirect to c, as [Open] describes.
	Run(ctx context.Context, f Flow, c Callback) (Return, error)
}

var providers struct {
	mu   sync.Mutex
	list []Provider
}

// Register adds a provider. One registered later is asked before the ones
// registered earlier, so that a package imported for a better mechanism
// takes over the callbacks it supports. It is meant for the init function of
// such a package.
func Register(p Provider) {
	providers.mu.Lock()
	defer providers.mu.Unlock()
	providers.list = append(providers.list, p)
}

func providerFor(c Callback) Provider {
	providers.mu.Lock()
	defer providers.mu.Unlock()
	for i := len(providers.list) - 1; i >= 0; i-- {
		if p := providers.list[i]; p.Supports(c) {
			return p
		}
	}
	return nil
}

func init() { Register(loopback{}) }

// browse opens a URL in the system browser; tests replace it.
var browse = systemBrowse

// AuthCode returns the authorization code of an OAuth redirect, after
// checking that its state is the one the request was sent with. A redirect
// with another state is [ErrState]; one that reports an error, a user who
// declined say, is an [*AuthError].
//
// The code is then redeemed at the token endpoint, with the PKCE verifier the
// request was made with:
//
//	code, err := ret.AuthCode(state)
//	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
func (r Return) AuthCode(state string) (string, error) {
	if r.URL == nil {
		return "", errors.New("gift/web: no redirect")
	}
	q := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
		return "", ErrState
	}
	if e := q.Get("error"); e != "" {
		return "", &AuthError{Code: e, Description: q.Get("error_description"), URI: q.Get("error_uri")}
	}
	code := q.Get("code")
	if code == "" {
		return "", errors.New("gift/web: redirect without a code")
	}
	return code, nil
}

// AuthError is the error response of an authorization request, RFC 6749
// section 4.1.2.1: "access_denied" when the user declined.
type AuthError struct {
	Code        string
	Description string
	URI         string
}

func (e *AuthError) Error() string {
	if e.Description != "" {
		return "gift/web: " + e.Code + ": " + e.Description
	}
	return "gift/web: " + e.Code
}
