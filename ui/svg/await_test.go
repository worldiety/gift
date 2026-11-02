package svg_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift/gifttest"
)

// awaitRasters runs frames for a while, so that the rasters of goroutines
// arrive.
func awaitRasters(t *testing.T, h *gifttest.Harness) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		h.Frame()
	}
	h.Settle()
}
