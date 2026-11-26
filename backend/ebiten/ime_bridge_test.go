package ebiten

import (
	"strings"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// The bridge between gift's text fields and exp/textinput, driven through the
// callbacks exp/textinput would call. The platform's half cannot run in a
// test binary; what can is everything the bridge decides.

// imeField mounts a text field holding s, focuses it with a platform keyboard
// reported, as newSession reports it on a phone, and returns the harness, the
// editor and a bridge for it.
func imeField(t *testing.T, s string) (*gifttest.Harness, *ui.TextEditor, *imeBridge) {
	t.Helper()
	font := ui.MustFont(ui.FontQuery{Family: inter.Family})
	ed := ui.NewTextEditor(s)
	h := gifttest.New(t, gifttest.Options{
		View: ui.TextField(ed).Font(font).Key("field"),
		Size: geom.Sz(400, 100),
		Font: font,
	})
	h.App().SetPlatformKeyboardAvailable(true)
	h.Settle()
	h.Find(gifttest.ByKey("field")).Focus()
	h.Settle()
	if !h.App().SoftKeyboardRequested() {
		t.Fatal("the focused field asked for no keyboard")
	}
	return h, ed, newIMEBridge(h.App(), nil, func() float64 { return 2 })
}

// inInput runs fn in an input phase of its own, as the bridge runs in the
// backend's poll, and settles.
func inInput(h *gifttest.Harness, fn func()) {
	h.App().BeginInput(h.Now())
	fn()
	h.Settle()
}

// TestIMECompositionIsShownAndCommitted: a composition is shown as text and
// replaced as it changes, and the commit, an autocorrection here, replaces it
// together with the text it corrects.
func TestIMECompositionIsShownAndCommitted(t *testing.T) {
	h, ed, b := imeField(t, "Ich ")
	inInput(h, func() {
		opts := b.onNewSession()
		if opts == nil || opts.TextBeforeCaret != "Ich " || opts.TextAfterCaret != "" {
			t.Fatalf("session seeded with %+v", opts)
		}
		if opts.CaretBounds.Empty() {
			t.Fatal("the session has no caret")
		}
		b.onComposition("te")
		b.onComposition("tes")
		b.onComposition("tesr")
	})
	if got := ed.Text(); got != "Ich tesr" {
		t.Fatalf("while composing the field holds %q, want the composition shown", got)
	}
	st, _ := h.App().TextInput()
	if !b.inStep(st) {
		t.Fatal("the bridge takes its own composition for a change of the field")
	}
	// The keyboard corrects tesr to test, with a space.
	inInput(h, func() { b.onCommit("test ", "Ich ", "") })
	if got := ed.Text(); got != "Ich test " {
		t.Fatalf("after the commit the field holds %q, want %q", got, "Ich test ")
	}
	if b.open || b.shown != "" {
		t.Fatal("a commit leaves the session open; the next one has to be seeded from the field")
	}
}

// TestIMECompositionInALongTextStaysInStep: a text of more than the window
// before the caret reports a window that moves as the composition grows, and
// the session is still the same session; its commit lands where it belongs.
func TestIMECompositionInALongTextStaysInStep(t *testing.T) {
	doc := strings.Repeat("x", 5000) + " "
	h, ed, b := imeField(t, doc)
	inInput(h, func() { b.onNewSession() })
	if len(b.before) >= len(doc) {
		t.Fatalf("the session was seeded with %d bytes; the test needs a window", len(b.before))
	}
	for _, c := range []string{"t", "te", "tes", "te"} {
		inInput(h, func() { b.onComposition(c) })
		st, _ := h.App().TextInput()
		if !b.inStep(st) {
			t.Fatalf("composing %q in a long text is taken for a change of the field", c)
		}
	}
	inInput(h, func() { b.onCommit("test ", b.before, "") })
	if got := ed.Text(); got != doc+"test " {
		t.Fatalf("after the commit the field ends in %q, want %q", got[len(got)-10:], (doc + "test ")[len(doc)-5:])
	}
}

// TestIMEADeletionElsewhereDuringACompositionIsAChange: in a short text the
// exact check still holds; a rune deleted before the composition by other
// means is a change of the field.
func TestIMEADeletionElsewhereDuringACompositionIsAChange(t *testing.T) {
	h, ed, b := imeField(t, "aaa ")
	inInput(h, func() {
		b.onNewSession()
		b.onComposition("b")
	})
	ed.SetText("aa b")
	h.Settle()
	st, _ := h.App().TextInput()
	if b.inStep(st) {
		t.Fatal("a deletion before the composition is still taken for the seeded state")
	}
}

// TestIMEBackspaceDeletesTheRuneBefore: a system keyboard's backspace is a
// deletion in its buffer, and arrives as a commit of nothing.
func TestIMEBackspaceDeletesTheRuneBefore(t *testing.T) {
	h, ed, b := imeField(t, "Grü")
	inInput(h, func() {
		b.onNewSession()
		b.onCommit("", "Gr", "")
	})
	if got := ed.Text(); got != "Gr" {
		t.Fatalf("after a backspace the field holds %q, want %q", got, "Gr")
	}
}

// TestIMEBackspaceWithASelectionDeletesOnlyTheSelection: the platform's buffer
// holds no selection and deletes the rune before the caret; the user meant
// the selection.
func TestIMEBackspaceWithASelectionDeletesOnlyTheSelection(t *testing.T) {
	h, ed, b := imeField(t, "abcdef")
	ed.SetSelection(2, 4)
	inInput(h, func() {
		opts := b.onNewSession()
		if opts.TextBeforeCaret != "ab" || opts.TextAfterCaret != "ef" {
			t.Fatalf("session seeded with %q | %q", opts.TextBeforeCaret, opts.TextAfterCaret)
		}
		b.onCommit("", "a", "ef")
	})
	if got := ed.Text(); got != "abef" {
		t.Fatalf("after a backspace on a selection the field holds %q, want %q", got, "abef")
	}
}

// TestIMETypingOverASelectionReplacesIt: a composition started on a
// selection replaces it, and the session stays in step with the field.
func TestIMETypingOverASelectionReplacesIt(t *testing.T) {
	h, ed, b := imeField(t, "abcdef")
	ed.SetSelection(2, 4)
	inInput(h, func() {
		b.onNewSession()
		b.onComposition("X")
	})
	if got := ed.Text(); got != "abXef" {
		t.Fatalf("typing over a selection gives %q, want %q", got, "abXef")
	}
	st, _ := h.App().TextInput()
	if !b.inStep(st) {
		t.Fatal("the composition that replaced the selection is taken for a change of the field")
	}
}

// TestIMEFieldChangedElsewhereIsOutOfStep: a caret moved by a tap or a key, or
// text changed by the application, makes the session stale, and update ends
// it so that the next one is seeded from the field.
func TestIMEFieldChangedElsewhereIsOutOfStep(t *testing.T) {
	h, ed, b := imeField(t, "hello world")
	inInput(h, func() { b.onNewSession() })

	ed.SetCaret(5)
	st, _ := h.App().TextInput()
	if b.inStep(st) {
		t.Fatal("a moved caret is still taken for the seeded state")
	}

	inInput(h, func() { b.onNewSession() })
	ed.SetText("other")
	st, _ = h.App().TextInput()
	if b.inStep(st) {
		t.Fatal("a rewritten text is still taken for the seeded state")
	}
}

// TestIMESecureFieldIsServedLikeAnyOther: a password field gets the system
// keyboard through the same session as any field, seeded with its text, so
// that a backspace of the keyboard deletes the character before the caret.
func TestIMESecureFieldIsServedLikeAnyOther(t *testing.T) {
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	font := ui.MustFont(ui.FontQuery{Family: inter.Family})
	pw := ui.NewTextEditor("geheim")
	h := gifttest.New(t, gifttest.Options{
		View: ui.SecureField(pw).Font(font).Key("pw"),
		Size: geom.Sz(400, 100),
		Font: font,
	})
	h.Find(gifttest.ByKey("pw")).Focus()
	h.Settle()
	b := newIMEBridge(h.App(), nil, func() float64 { return 2 })
	inInput(h, func() {
		opts := b.onNewSession()
		if opts == nil {
			t.Fatal("no session for a password field")
		}
		if opts.TextBeforeCaret != "geheim" {
			t.Fatalf("the password session is seeded with %q, want the text", opts.TextBeforeCaret)
		}
		b.onCommit("", "gehei", "")
	})
	if got := pw.Text(); got != "gehei" {
		t.Fatalf("a backspace in the password field leaves %q", got)
	}
}

// imeKeys is [imeField] with an input bridge around the bridge, whose keys
// are pressed through pressed, and a session open on the field as it is,
// the system keyboard up. Selecting before the call seeds the session with
// the selection.
func imeKeys(t *testing.T, s string, sel func(*ui.TextEditor)) (*gifttest.Harness, *ui.TextEditor, *inputBridge, map[eb.Key]bool) {
	t.Helper()
	h, ed, ime := imeField(t, s)
	if sel != nil {
		sel(ed)
		h.Settle()
	}
	pressed := map[eb.Key]bool{}
	var chars []rune
	b := scriptedBridge(h.App(), pressed, &chars)
	b.ime = ime
	inInput(h, func() {
		if ime.onNewSession() == nil {
			t.Fatal("no session for the field")
		}
		h.App().SetPlatformKeyboardActive(true)
	})
	return h, ed, b, pressed
}

// keyTick is one poll of the keys in an input phase of its own.
func keyTick(h *gifttest.Harness, b *inputBridge) {
	inInput(h, b.pollKeys)
}

// swallowedKey reports whether the input bridge holds back the release of
// k, a key whose press it kept from gift.
func swallowedKey(b *inputBridge, k gift.Key) bool {
	for i, tk := range trackedKeys {
		if tk.key == k {
			return b.swallowed[i]
		}
	}
	return false
}

// TestIMEDeleteKeysAreTheKeyboardsWhileItServes: a system keyboard that
// deletes with a key event has already deleted in its buffer and sends that
// as a replacement; the key Ebitengine reports as well must not delete a
// second time. Its release is kept back with it, and a press while no system
// keyboard serves reaches gift.
func TestIMEDeleteKeysAreTheKeyboardsWhileItServes(t *testing.T) {
	h, ed, b, pressed := imeKeys(t, "abc", nil)

	pressed[eb.KeyBackspace] = true
	keyTick(h, b)
	if got := ed.Text(); got != "abc" {
		t.Fatalf("a backspace the system keyboard served deleted in the field as well: %q", got)
	}
	if !swallowedKey(b, gift.KeyBackspace) {
		t.Fatal("the release of a kept back backspace is not kept back")
	}
	inInput(h, func() { h.App().SetPlatformKeyboardActive(false) }) // the release must stay back all the same
	pressed[eb.KeyBackspace] = false
	keyTick(h, b)
	if swallowedKey(b, gift.KeyBackspace) {
		t.Fatal("the release did not end the kept back press")
	}

	pressed[eb.KeyBackspace] = true
	keyTick(h, b)
	if got := ed.Text(); got != "ab" {
		t.Fatalf("with no system keyboard up a backspace leaves %q, want %q", got, "ab")
	}
}

// TestIMEBackspaceOnASelectionFromTheStartReachesTheField is select all, then
// backspace, with the system keyboard: the session was seeded with nothing
// before the selection, the keyboard's buffer has nothing to delete and sends
// no replacement, and the key is the only report of the backspace. The field
// takes it and deletes the selection; the release follows the press.
func TestIMEBackspaceOnASelectionFromTheStartReachesTheField(t *testing.T) {
	h, ed, b, pressed := imeKeys(t, "abc", func(ed *ui.TextEditor) { ed.SetSelection(0, 3) })

	pressed[eb.KeyBackspace] = true
	keyTick(h, b)
	if got := ed.Text(); got != "" {
		t.Fatalf("backspace on everything selected leaves %q, want an empty field", got)
	}
	if swallowedKey(b, gift.KeyBackspace) {
		t.Fatal("the release of a backspace the field took would be kept back")
	}
	pressed[eb.KeyBackspace] = false
	keyTick(h, b)
	if swallowedKey(b, gift.KeyBackspace) {
		t.Fatal("a release is kept back")
	}
}

// TestIMEBackspaceAtTheStartReachesTheField: the caret at the start, nothing
// before it in the buffer; the key goes to the field, which deletes nothing,
// and its release goes with it.
func TestIMEBackspaceAtTheStartReachesTheField(t *testing.T) {
	h, ed, b, pressed := imeKeys(t, "abc", func(ed *ui.TextEditor) { ed.SetCaret(0) })

	pressed[eb.KeyBackspace] = true
	keyTick(h, b)
	if swallowedKey(b, gift.KeyBackspace) {
		t.Fatal("a backspace at the start, which the buffer cannot delete, is kept from the field")
	}
	pressed[eb.KeyBackspace] = false
	keyTick(h, b)
	if got := ed.Text(); got != "abc" {
		t.Fatalf("a backspace at the start leaves %q", got)
	}
}

// TestIMEForwardDeleteFollowsTheTextAfter is the same rule for the forward
// delete: with text after the caret the buffer deletes it and the key stays
// back; with a selection to the end there is none, and the field deletes
// the selection.
func TestIMEForwardDeleteFollowsTheTextAfter(t *testing.T) {
	h, ed, b, pressed := imeKeys(t, "abc", func(ed *ui.TextEditor) { ed.SetCaret(1) })
	pressed[eb.KeyDelete] = true
	keyTick(h, b)
	if got := ed.Text(); got != "abc" {
		t.Fatalf("a forward delete the system keyboard served deleted in the field as well: %q", got)
	}
	if !swallowedKey(b, gift.KeyDelete) {
		t.Fatal("a forward delete with text after the caret reached the field")
	}

	h, ed, b, pressed = imeKeys(t, "abc", func(ed *ui.TextEditor) { ed.SetSelection(1, 3) })
	pressed[eb.KeyDelete] = true
	keyTick(h, b)
	if got := ed.Text(); got != "a" {
		t.Fatalf("a forward delete on a selection to the end leaves %q, want %q", got, "a")
	}
	if swallowedKey(b, gift.KeyDelete) {
		t.Fatal("the release of a forward delete the field took would be kept back")
	}
}

// TestIMEBackspaceInsideAComposition: a composition shown at the start of
// the text is in the keyboard's buffer, so its backspace is the keyboard's.
func TestIMEBackspaceInsideAComposition(t *testing.T) {
	h, _, b, pressed := imeKeys(t, "", nil)
	inInput(h, func() { b.ime.onComposition("ab") })
	pressed[eb.KeyBackspace] = true
	keyTick(h, b)
	if !swallowedKey(b, gift.KeyBackspace) {
		t.Fatal("a backspace inside a composition reached the field")
	}
}

// TestIMECaretOnAnotherLineIsOutOfStep: the platform keeps the picture
// shifted for the caret it was given when the session opened, so a caret the
// layout moved — a rotation, a new safe area, a scroll — makes the session
// stale; a caret that only moved along its line, as a composition growing
// does, does not. A move during a composition waits for the commit; see
// [imeBridge.inStep].
func TestIMECaretOnAnotherLineIsOutOfStep(t *testing.T) {
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	font := ui.MustFont(ui.FontQuery{Family: inter.Family})
	// In a window, so that a safe area moves the field the way a rotation
	// does.
	h := gifttest.New(t, gifttest.Options{
		View: ui.Window(ui.TextField(ui.NewTextEditor("Hallo")).Font(font).Key("field")),
		Size: geom.Sz(400, 300),
		Font: font,
	})
	h.Find(gifttest.ByKey("field")).Focus()
	h.Settle()
	b := newIMEBridge(h.App(), nil, func() float64 { return 2 })
	inInput(h, func() {
		b.onNewSession()
		b.onComposition("x")
	})
	st, _ := h.App().TextInput()
	if !b.inStep(st) {
		t.Fatal("a composition that moved the caret along its line is taken for a change of the field")
	}
	h.SetSafeArea(geom.Insets{Top: 40})
	st, _ = h.App().TextInput()
	if !b.inStep(st) {
		t.Fatal("a layout move during a composition ended the word before its commit")
	}
	h.SetSafeArea(geom.Insets{})
	inInput(h, func() {
		b.onCommit("x", "Hallo", "")
		b.onNewSession()
	})
	h.SetSafeArea(geom.Insets{Top: 40})
	st, _ = h.App().TextInput()
	if b.inStep(st) {
		t.Fatalf("a caret the layout moved from %v to %v is still taken for the seeded one", b.caret, st.Caret)
	}
}

// TestIMECompositionThatWrapsStaysInStep: in a text area a growing word that
// wraps moves the caret to the next line, and that is the composition, not a
// change of the field; a new session would end the word mid-way.
func TestIMECompositionThatWrapsStaysInStep(t *testing.T) {
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	font := ui.MustFont(ui.FontQuery{Family: inter.Family})
	h := gifttest.New(t, gifttest.Options{
		View: ui.Window(ui.TextArea(ui.NewTextEditor("aaaa bbbb cccc dddd ")).Font(font).Frame(160, 200).Key("area")),
		Size: geom.Sz(400, 300),
		Font: font,
	})
	h.Find(gifttest.ByKey("area")).Focus()
	h.Settle()
	b := newIMEBridge(h.App(), nil, func() float64 { return 2 })
	inInput(h, func() { b.onNewSession() })
	start := b.caret
	wrapped := false
	word := "wonderfullyamazing"
	for i := 1; i <= len(word); i++ {
		inInput(h, func() { b.onComposition(word[:i]) })
		st, _ := h.App().TextInput()
		if !b.inStep(st) {
			t.Fatalf("the composition %q, caret %v, is taken for a change of the area", word[:i], st.Caret)
		}
		wrapped = wrapped || moved(st.Caret.Min.Y, start.Min.Y)
	}
	if !wrapped {
		t.Fatal("the composition never wrapped; the test proves nothing")
	}
}

// imeForm is a phone's form: a window with the drawn keyboard, as the
// recipes of NewGame place it, and a field at the end of a scrolled form, so
// that a keyboard laid out over it would scroll it.
func imeForm(t *testing.T, available bool) (*gifttest.Harness, gifttest.Node) {
	t.Helper()
	font := ui.MustFont(ui.FontQuery{Family: inter.Family})
	h := gifttest.New(t, gifttest.Options{
		View: ui.Window(
			ui.VScroll(ui.VStack(
				ui.Box().Frame(200, 450),
				ui.TextField(ui.NewTextEditor("")).Font(font).Key("field"),
			)).Key("form"),
			ui.OnScreenKeyboard().Key("kb"),
		).Align(geom.Bottom),
		Size: geom.Sz(400, 500),
		Font: font,
	})
	h.App().SetPlatformKeyboardAvailable(available)
	h.Settle()
	return h, h.Find(gifttest.ByKey("field"))
}

// TestIMEDrawnKeyboardDoesNotFlashForAFieldTheSystemKeyboardTakes: the tap
// that focuses a field is dispatched after the bridge's update, and the build
// of the same tick has to know already that the system keyboard serves it.
// Otherwise the drawn keyboard is laid out for one frame, and the reveal the
// next tick makes against it scrolls the field up by the keyboard's height
// and stays when it steps aside. The field moves by the plain reveal of its
// focus, as it does on a desktop, and no further.
func TestIMEDrawnKeyboardDoesNotFlashForAFieldTheSystemKeyboardTakes(t *testing.T) {
	ref, refField := imeForm(t, false)
	ref.App().BeginInput(ref.Now())
	ref.App().MoveFocus(true)
	ref.Settle()
	want := refField.Bounds()

	h, field := imeForm(t, true)
	var chars []rune
	b := scriptedBridge(h.App(), map[eb.Key]bool{}, &chars)
	b.ime = newIMEBridge(h.App(), nil, func() float64 { return 2 })
	formH := h.Find(gifttest.ByKey("form")).LayoutBounds().Height()

	// One poll: the update sees no request, the input focuses the field,
	// and the report follows the dispatch.
	h.App().BeginInput(h.Now())
	b.ime.update()
	h.App().MoveFocus(true)
	b.ime.report()
	if !h.App().SoftKeyboardRequested() || !h.App().PlatformKeyboardActive() {
		t.Fatal("the field focused by this tick's input is not reported as served by the system keyboard")
	}
	h.Frame()
	if got := h.Find(gifttest.ByKey("kb")).LayoutBounds().Height(); got != 0 {
		t.Fatalf("the drawn keyboard is %v tall in the frame of the focus, want 0", got)
	}
	if got := h.Find(gifttest.ByKey("form")).LayoutBounds().Height(); got != formH {
		t.Fatalf("the form is %v tall in the frame of the focus, want its room, %v", got, formH)
	}
	h.Settle()
	if got := field.Bounds(); got != want {
		t.Fatalf("the field is at %v, want %v, where the plain reveal of its focus puts it", got, want)
	}
}

// TestIMEAFieldThePlatformRefusesKeepsTheDrawnKeyboard: text that is not
// valid UTF-8 is a seed exp/textinput refuses. The bridge opens no session,
// reports no system keyboard once its update has seen that, and leaves the
// delete keys to the field, so that the drawn keyboard serves it rather than
// none.
func TestIMEAFieldThePlatformRefusesKeepsTheDrawnKeyboard(t *testing.T) {
	h, _, b := imeField(t, "ab\xff")
	if b.onNewSession() != nil {
		t.Fatal("a session was seeded with text that is not valid UTF-8")
	}
	for range 3 {
		inInput(h, func() {
			b.update()
			b.report()
		})
	}
	if h.App().PlatformKeyboardActive() {
		t.Fatal("no session is open and the system keyboard is reported as serving the field")
	}
	if b.open || b.canDelete(gift.KeyBackspace) {
		t.Fatal("a backspace would be swallowed for a buffer that does not exist")
	}
}
