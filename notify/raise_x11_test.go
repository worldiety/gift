//go:build linux && !android && !faketime

package notify

import "testing"

func TestTokenTime(t *testing.T) {
	for token, want := range map[string]uint32{
		"": 0,
		"gnome-shell/Photos/4711-0-host_TIME123456": 123456,
		"kwin-1-host-_TIME0":                        0,
		"no-time-here":                              0,
		"x_TIMEnot-a-number":                        0,
		"x_TIME99999999999":                         0, // more than 32 bits
		"a_TIME1_TIME2":                             2,
	} {
		if got := tokenTime(token); got != want {
			t.Errorf("tokenTime(%q) = %d, want %d", token, got, want)
		}
	}
}
