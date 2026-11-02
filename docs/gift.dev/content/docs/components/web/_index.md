---
title: Login & Payment
weight: 10
---

A login and a payment run in a browser the application does not control: on the desktop in the system browser, on a
kiosk on the customer's own phone. These components show the phone's part on the screen of a kiosk. They live in
package [`web/handoff`](https://github.com/worldiety/gift/tree/main/web/handoff); the desktop's part is package
[`web`](https://github.com/worldiety/gift/tree/main/web). [Login and Payment](../../concepts/login-and-payment/)
explains why there is no web view for either.

- [Device Login](device-login/): signing in on the phone with the device flow, as a QR code and a code to type.
- [Payment Handoff](pay-handoff/): paying on the phone with a payment link as a QR code.
