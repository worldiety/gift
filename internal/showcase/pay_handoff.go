package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/web/handoff"
)

func init() {
	register(Demo{Name: "pay-handoff", Group: "web", Title: "Payment Handoff",
		Summary: "Paying on the phone, for a kiosk: a payment link as a QR code, with the state the backend reports.",
		Width:   720, Height: 320}, payHandoffDemo)
}

func payHandoffDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The link comes from the application's backend, a Stripe Checkout
	// Session say, and so does the state, from the provider's webhook.
	return handoff.Pay("https://pay.example.com/c/4711").Amount("12,50 €")
	// snippet:end
}
