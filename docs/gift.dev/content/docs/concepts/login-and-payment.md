---
title: Login and Payment
weight: 19
---

A login at Google or Microsoft and a payment with PayPal or Stripe run in a browser that the application does not
control, and the application gets their result: the redirect they end with. Package
[`web`](https://github.com/worldiety/gift/tree/main/web) runs them in the system browser on the desktop, and package
[`web/handoff`](https://github.com/worldiety/gift/tree/main/web/handoff) hands them to the user's phone on a kiosk.
Neither needs cgo.

```go
verifier, state := oauth2.GenerateVerifier(), rand.Text()
ret, err := web.Open(ctx, web.Flow{
	Callbacks: []web.Callback{web.Loopback("/oauth")},
	Start: func(_ context.Context, redirect string) (string, error) {
		conf.RedirectURL = redirect
		return conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
	},
})
code, err := ret.AuthCode(state)
tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
```

The protocol is [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2)'s, PKCE included. `web.Open` is the
part between the authorization URL and the code: it listens on a free port of `127.0.0.1`, builds the redirect URI
with it, opens the system browser at the URL `Start` returns and waits for the redirect. The browser then shows a
page that says the user may close it, or the page `Flow.Landing` names.

## Why no web view

A login page in a web view of the application is a page whose password field the application could read. RFC 8252,
the standard for OAuth in native applications, forbids it, and identity providers enforce it: Google refuses with
`disallowed_useragent`, and the passkeys of a provider do not work in a web view. Payment providers say the same:
PayPal forbids web views, Stripe sends an iOS app to Safari, and Apple Pay and Google Pay work in a browser but not
in a web view. So `web.Open` asks for a result, not for a widget, and each platform runs the flow in the browser it
has for it.

## Where a flow ends

A `Callback` names a kind of redirect, and `Flow.Callbacks` lists the ones the client is registered for, the
preferred first. `Open` takes the first one the platform supports, so a client registered with several runs
everywhere:

| Callback | Redirect URI | Where |
|---|---|---|
| `web.Loopback("/oauth")` | `http://127.0.0.1:{port}/oauth` | macOS, Windows, Linux: the system browser |
| `web.Scheme("com.example.app", "/oauth")` | `com.example.app:/oauth` | an app on a phone, once its host supports it |
| `web.HTTPS("app.example.com", "/oauth")` | `https://app.example.com/oauth` | a universal link or app link |

Every identity provider accepts a loopback redirect for a native client, with any port. A private scheme is not
supported on the desktop, where another program could register the same scheme and take the redirect. The loopback
server sets the socket options RFC 8252 asks for, so that no other process can bind its port, and accepts only
requests to its own address, which keeps a web page out that reaches it through DNS rebinding.

## A redirect is not a receipt

`web.Return` says that the user came back, not that they paid: anyone can call a redirect URI, and a user may close
the tab before it. For a login, `Return.AuthCode` checks the state and the code is redeemed at the token endpoint,
which is the proof. For a payment, the outcome reaches the application's backend through the payment provider's
webhook, and the application asks its backend. `Open` waits for its context as well as for the redirect, so a flow
whose result came by another way is ended by cancelling it:

```go
ctx, cancel := context.WithCancel(ctx)
go func() { backend.AwaitPaid(ctx, order); cancel() }()
_, err := web.Open(ctx, web.Flow{Start: func(ctx context.Context, _ string) (string, error) {
	return backend.Checkout(ctx, order) // its success_url: a "return to the app" page
}})
```

A flow without callbacks is only opened, and `Open` returns when its context ends. `web.OpenURL` opens a page
without waiting for anything: a help page, the terms, a receipt.

## On a kiosk

A kiosk has no browser of its own, and its shared screen is no place for a password or a card number. The flow runs
on the user's phone instead, and the kiosk shows a QR code:

- A login uses the device flow of RFC 8628: [Device Login](../../components/web/device-login/) asks the identity
  provider for a code, shows it, and polls for the token. The password never touches the kiosk, and there are no
  cookies to clear for the next user.
- A payment uses a payment link: [Payment Handoff](../../components/web/pay-handoff/) shows the link the backend
  created and the state the backend reports. A card typed into a browser on a kiosk would bring it into the full scope
  of PCI DSS; on the phone the customer has their wallets and their bank's app for 3-D Secure.

Some Microsoft Entra tenants block the device flow, as Microsoft recommends against phishing. A browser on the kiosk
itself, a second window under labwc, is the fallback for those and is not built yet.

## Threads

`web.Open` blocks until the flow ends, so it runs on a goroutine of its own and posts its result back, as every
asynchronous result does; see [Concurrency](../concurrency/):

```go
tok := ctx.Request()
go func() {
	ret, err := web.Open(context.Background(), flow)
	tok.Post(func() { result.Set(outcome(ret, err)) })
}()
```

`handoff.Login` does this by itself and calls back on the UI goroutine.

## Platforms

| Platform | Login | Payment |
|---|---|---|
| macOS, Windows, Linux | `web.Open` with `Loopback`: `open`, `ShellExecuteW`, `xdg-open` | `web.Open` without callbacks, the outcome from the backend |
| Kiosk | `handoff.Login`, the device flow | `handoff.Pay`, a payment link |
| Android, iOS | not yet: Auth Tab and `ASWebAuthenticationSession` through the host app of a gomobile build | Custom Tab, Safari |
| Browser (wasm) | not yet: a redirect of the page, resumed when it comes back | the same |

`web.Open` returns `web.ErrUnsupported` where nothing can run a flow, and so does `web.OpenURL` on a kiosk without
a browser. A package that brings another mechanism registers a `web.Provider` when it is imported, and providers
registered later take precedence: `ASWebAuthenticationSession` on macOS, with a session of its own and
`Flow.Ephemeral`, is the next one planned.

## API

{{< api pkg="web" type="Flow" >}}

{{< api pkg="web" type="Callback" >}}

{{< api pkg="web" type="Return" >}}

{{< api-funcs pkg="web" names="OpenURL,Register" >}}
