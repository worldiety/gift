package filepicker

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// back performs the back action and waits for the directory it opened.
func back(t *testing.T, h *gifttest.Harness) bool {
	t.Helper()
	ok := h.Back()
	waitListed(t, h)
	return ok
}

func TestBackGoesUpADirectoryAtATimeAndCancelsInTheRoot(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	tapRow(t, h, "alpha")
	tapRow(t, h, "sub")
	h.Find(gifttest.ByText("x.jpg")).AssertVisible()

	if !h.CanGoBack() || !back(t, h) {
		t.Fatal("back in alpha/sub was not taken")
	}
	h.Find(gifttest.ByText("two.txt")).AssertVisible()
	if !h.CanGoBack() || !back(t, h) {
		t.Fatal("back in alpha was not taken")
	}
	h.Find(gifttest.ByText("b.txt")).AssertVisible()
	if o.cancelled != 0 {
		t.Fatalf("going up cancelled %d times", o.cancelled)
	}
	// In the root, back cancels.
	if !h.CanGoBack() || !back(t, h) {
		t.Fatal("back in the root of a picker that can cancel was not taken")
	}
	if o.cancelled != 1 {
		t.Fatalf("back in the root cancelled %d times, want once", o.cancelled)
	}
	if len(o.picked) != 0 {
		t.Fatalf("back picked %v", o.picked)
	}
}

func TestEscapeInADirectoryStillCancelsAndDoesNotGoUp(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	tapRow(t, h, "alpha")
	h.Key(gift.KeyEscape)
	waitListed(t, h)
	if o.cancelled != 1 {
		t.Fatalf("escape in alpha cancelled %d times, want once", o.cancelled)
	}
	h.Find(gifttest.ByText("two.txt")).AssertVisible()
}

func TestBackInTheRootOfAPickerThatCannotCancelIsNotTaken(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, nil))
	if h.CanGoBack() {
		t.Error("a picker without onCancel takes back in its root")
	}
	tapRow(t, h, "alpha")
	if !h.CanGoBack() {
		t.Fatal("a picker without onCancel does not go up from alpha")
	}
	back(t, h)
	h.Find(gifttest.ByText("b.txt")).AssertVisible()
	if h.CanGoBack() || h.Back() {
		t.Error("back in the root of a picker without onCancel was taken, which would keep the application from being left")
	}
}

func TestBackInTheRootOfAPickerThatCannotCancelDismissesTheSheetAroundIt(t *testing.T) {
	var o outcome
	dismissed := 0
	h := gifttest.New(t, gifttest.Options{
		Root: func(ctx *gift.Context) gift.View {
			shown := ctx.State("shown", true)
			return ui.Window(ui.Sheet(ui.Text("behind").Key("behind"), New(testFS(), o.pick, nil)).
				Presented(ctx.Read(shown)).
				OnDismiss(func() { dismissed++; shown.Set(false) }))
		},
		Size:  geom.Sz(420, 720),
		Theme: ui.LightTheme(),
		Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	waitListed(t, h)
	tapRow(t, h, "alpha")
	if !back(t, h) {
		t.Fatal("back in alpha was not taken")
	}
	h.Find(gifttest.ByText("b.txt")).AssertVisible()
	if dismissed != 0 {
		t.Fatalf("back in alpha dismissed the sheet instead of going up")
	}
	if !h.CanGoBack() || !back(t, h) {
		t.Fatal("back in the root of a picker in a sheet was not taken by the sheet")
	}
	if dismissed != 1 {
		t.Fatalf("back in the root dismissed the sheet %d times, want once", dismissed)
	}
}

func TestBackAnswersTheReplaceAlertWithCancel(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel).Mode(Save))
	tapRow(t, h, "b.txt")
	tapKey(t, h, "primary")
	h.Find(gifttest.ByText("Replace").And(gifttest.ByType("ui.Button"))).AssertVisible()
	if !h.Back() {
		t.Fatal("back over the replace alert was not taken")
	}
	h.AssertNone(gifttest.ByText("Replace").And(gifttest.ByType("ui.Button")))
	if len(o.picked) != 0 || o.cancelled != 0 {
		t.Fatalf("back over the alert picked %v and cancelled %d times, want neither", o.picked, o.cancelled)
	}
}

func TestAskingAPickerWhetherItGoesBackDoesNotAllocate(t *testing.T) {
	var o outcome
	h := mountPicker(t, New(testFS(), o.pick, o.cancel))
	tapRow(t, h, "alpha")
	app := h.App()
	if n := testing.AllocsPerRun(100, func() { app.CanGoBack() }); n != 0 {
		t.Errorf("CanGoBack in a picker allocates %v times", n)
	}
}
