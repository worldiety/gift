---
title: Device Login
---

A device login signs the user in on their own phone: the screen shows a QR code and a short code, the user scans the
one or types the other on the page of the identity provider, and the application gets a token. It is the device
authorization grant of RFC 8628, the way a TV signs in to a streaming service, and the way a kiosk signs a user in
without a password on its shared screen.

{{< gallery-shot alt="A QR code beside the title Sign in with your phone, the address login.example.com/device and the code WDJB-MJHT in large letters" >}}

{{< gallery-code >}}

## Usage

`handoff.Login` runs the whole flow with [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2): it asks for
a code when it is shown, polls the token endpoint at the interval the server asks for, and calls back with the token.

```go
conf := &oauth2.Config{
	ClientID: "kiosk",
	Scopes:   []string{"openid", "profile"},
	Endpoint: oauth2.Endpoint{
		DeviceAuthURL: "https://login.example.com/device/code",
		TokenURL:      "https://login.example.com/token",
	},
}
handoff.Login(conf, func(tok *oauth2.Token) { session.Set(tok) }).
	OnError(func(err error) { log.Error("login", "err", err) })
```

The code counts down and expires; an expired code and a declined login show a button for a new one. The flow ends
when the view is taken out of the tree, and no code is requested before it is shown.

`handoff.Device` is the view alone, for a flow the application runs itself, through its backend say: it shows a
`*oauth2.DeviceAuthResponse` in the `State` the application gives it.

```go
handoff.Device(da).State(handoff.Waiting).OnRestart(newCode)
```

The QR code opens `verification_uri_complete`, the page with the code filled in, where the provider sends one;
Microsoft Entra does not, and the QR code then opens the page where the code is typed. The words are English;
`Texts` gives them in the application's language.

## Providers

Google, Microsoft Entra, GitHub, Auth0, Okta, Keycloak and Zitadel support the device flow, each with conditions.
GitHub has it turned off until the application turns it on. Google allows it only for a few scopes, among them
`openid`, `email` and `profile`. Microsoft recommends that tenants block it by conditional access, because it is used
for phishing, so a kiosk for a company on Entra may need a browser on the device instead. x/oauth2 sends a token
request that fails a second time in the other style of client authentication, until it knows which one the server
takes; an `AuthStyle` in the `oauth2.Endpoint` saves the second request.

## API

{{< api pkg="handoff" type="LoginView" >}}

{{< api pkg="handoff" type="DeviceView" >}}

{{< api pkg="handoff" type="Texts" >}}

## Related

- [Payment Handoff](../pay-handoff/) is the same for a payment.
- [Login and Payment](../../../concepts/login-and-payment/) explains the login on the desktop and on the phone.
- [QR Code](../../basic/qr-code/) is the code alone.
