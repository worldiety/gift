// Package web runs logins and payments in a browser that the application
// does not control, and hands it the redirect they end with.
//
//	verifier, state := oauth2.GenerateVerifier(), rand.Text()
//	ret, err := web.Open(ctx, web.Flow{
//		Callbacks: []web.Callback{web.Loopback("/oauth")},
//		Start: func(_ context.Context, redirect string) (string, error) {
//			conf.RedirectURL = redirect
//			return conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
//		},
//	})
//	code, err := ret.AuthCode(state)
//	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
//
// The protocol is golang.org/x/oauth2's; this package is the part between
// the authorization URL and the code: the browser, the redirect and the
// server that catches it.
//
// # Why no web view
//
// A login page shown in a web view of the application is a page whose
// password field the application can read, and the standard for native
// applications, RFC 8252, forbids it: identity providers block it, Google
// with "disallowed_useragent", and passkeys of a provider do not work in one.
// Payment providers say the same: PayPal forbids web views, Stripe sends an
// iOS application to Safari, and wallets such as Apple Pay and Google Pay
// work in a browser but not in a web view. So [Open] asks for a result, not
// a widget, and each platform runs the flow in the browser it has for it.
//
// # A redirect is not a receipt
//
// A [Return] says that the user came back, not that they paid: a redirect
// URI can be called by anyone, and a user may close the tab before the
// redirect. A payment's outcome reaches the application's backend through
// the provider's webhook, and the application learns it from there.
// [Open] waits for its ctx as well as for the redirect, so a flow whose
// result came by another way is ended by cancelling it:
//
//	ctx, cancel := context.WithCancel(ctx)
//	go func() { backend.AwaitPaid(ctx, order); cancel() }()
//	_, err := web.Open(ctx, web.Flow{Start: func(ctx context.Context, _ string) (string, error) {
//		return backend.Checkout(ctx, order) // its success_url: a "return to the app" page
//	}})
//
// # Platforms
//
// A [Callback] names a kind of redirect, and [Flow.Callbacks] lists the
// ones the client is registered for, the preferred first; [Open] takes the
// first one a [Provider] supports here.
//
//   - macOS, Windows, Linux and the BSDs: [Loopback] with the system browser,
//     opened by open(1), ShellExecuteW and xdg-open. A private [Scheme] is
//     not supported on the desktop: the system would have to know it, and
//     another program may register the same scheme and take the redirect.
//   - A kiosk: no browser of its own, and a shared screen is no place to
//     type a password or a card number. web/handoff runs the flow on the
//     user's phone instead, with a QR code: the device flow of RFC 8628 for
//     a login, a payment link for a payment.
//   - Android, iOS and the browser: not yet; [Open] returns [ErrUnsupported].
//     A phone runs a flow in a session of its own, Auth Tab and
//     ASWebAuthenticationSession, which the host application of a gomobile
//     build will provide; a page in the browser redirects itself.
//
// A package that brings another mechanism registers a [Provider] when it is
// imported, and the providers registered later are asked first.
//
// # Threads
//
// [Open] blocks until the flow ends and is called from a goroutine of its
// own; see its documentation. [OpenURL], [Register] and [Return.AuthCode] may
// be called from any goroutine.
package web
