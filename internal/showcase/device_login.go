package showcase

import (
	"golang.org/x/oauth2"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/web/handoff"
)

func init() {
	register(Demo{Name: "device-login", Group: "web", Title: "Device Login",
		Summary: "Signing in on the phone, for a kiosk: the device flow as a QR code and a code to type.",
		Width:   720, Height: 380}, deviceLoginDemo)
}

func deviceLoginDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// What golang.org/x/oauth2's Config.DeviceAuth returns; handoff.Login
	// asks for it, polls for the token and shows this view by itself.
	da := &oauth2.DeviceAuthResponse{
		UserCode:                "WDJB-MJHT",
		VerificationURI:         "https://login.example.com/device",
		VerificationURIComplete: "https://login.example.com/device?user_code=WDJB-MJHT",
	}
	return handoff.Device(da)
	// snippet:end
}
