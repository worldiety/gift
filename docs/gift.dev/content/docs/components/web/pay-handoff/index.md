---
title: Payment Handoff
---

A payment handoff lets the customer pay on their own phone: the screen shows a payment link as a QR code, the phone
opens the payment provider's page with the customer's wallets and saved cards, and the screen shows the outcome. The
card never touches the shared screen of a kiosk.

{{< gallery-shot alt="A QR code beside the title Pay with your phone, the amount 12,50 € and the line Waiting for the payment" >}}

{{< gallery-code >}}

## Usage

The link comes from the application's backend, which creates it with the payment provider: a Stripe Payment Link or
Checkout Session, a PayPal order, the page of any provider. The outcome comes from the backend too, which learns it
from the provider's webhook; the view shows the `State` it is given.

```go
handoff.Pay(session.URL).
	Amount("12,50 €").
	Expiry(session.ExpiresAt).
	State(state).         // handoff.Waiting, Done, Failed or Expired
	OnRestart(newSession) // a button once the link has expired or the payment failed
```

`Done` replaces the code by a check mark, so that nobody pays twice. With an `Expiry` the view counts down and
enters `Expired` by itself.

## Why on the phone

A card number typed into a browser on a kiosk brings the kiosk into the full scope of PCI DSS: the light
self-assessments are for devices that never see the card, or for certified payment terminals. The phone also has
what a kiosk lacks: Apple Pay and Google Pay, the bank's app for 3-D Secure, and the customer's own saved cards. The
other way that keeps the card off the screen is a certified unattended terminal, which the backend drives through
the provider's cloud API; it needs no view of gift beyond a line of status.

## API

{{< api pkg="handoff" type="PayView" >}}

{{< api pkg="handoff" type="State" >}}

## Related

- [Device Login](../device-login/) is the same for a login.
- [Login and Payment](../../../concepts/login-and-payment/) explains a payment on the desktop.
