package ebiten

import (
	"image"
	"log/slog"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// imeBridge serves gift's request for a keyboard with the system keyboard of
// Android and iOS, through Ebitengine's exp/textinput; see
// [Config.NoNativeKeyboard] and the project plan, section 52.
//
// # The protocol
//
// A text field that wants characters says so with
// [gift.EventContext.RequestSoftKeyboard], and its interactor describes the
// text around the caret as a [gift.TextInputClient]. While both hold, the
// bridge keeps an input session open: the session seeds the platform's text
// buffer with that text, and the system keyboard edits the buffer. What comes
// back is one of three things, each turned into a [gift.App.ReplaceText]:
//
//   - a composition, the word the keyboard is still working on. gift has no
//     preedit (the project plan, section 14), so the word is shown as text:
//     each change replaces the previous composition, which the bridge
//     remembers, with the new one.
//   - a commit, the composition made final or text the keyboard inserted,
//     possibly replacing part of the text around the caret — an
//     autocorrection, or a backspace, which on a system keyboard is a
//     deletion in the buffer and not a key. The composition shown so far is
//     replaced along with it.
//   - the end of the session by the user, the keyboard put away with Back,
//     which takes the focus from the field as gift's own keyboard does.
//
// After a commit the session ends and the next one is seeded from the field
// again, so the platform's buffer never drifts from gift's text.
//
// # Secure fields
//
// A password field is served like any other, through the same session and
// the same calls. That is a compromise, stated rather than hidden: a buffer
// opened through exp/textinput is an ordinary text buffer to the system
// keyboard, which may learn from what is typed into it and suggest it, and
// Ebitengine has no way to ask for a keyboard in password mode. Reaching
// past Ebitengine into the platform's view to switch the mode would depend
// on Ebitengine's internals; the field reports
// [gift.TextInputState.Secure] so that a backend can ask for password mode
// once Ebitengine offers it.
//
// # On a desktop
//
// The bridge is made only where mobilePlatform holds, so a desktop never
// opens a session. The import is not behind a build tag, though, so that the
// bridge's tests run on the desktop: exp/textinput is linked into every
// binary, and its init registers one Objective-C class on macOS and the
// passive hooks of an Ebitengine VM guest everywhere.
type imeBridge struct {
	app *gift.App
	log *slog.Logger
	c   textinput.Composer

	// scale converts gift's device space, the space of
	// [gift.TextInputState.Caret] and [gift.Event.Pos], into the logical
	// pixels of Ebitengine, the pixels of the screen gift draws: it is
	// [gift.App.Density].
	scale func() float64

	// focus is the node the open session belongs to, and before and after
	// the text the session was seeded with: the reference every commit is
	// expressed against.
	focus         gift.NodeRef
	before, after string

	// selected is whether the field had a selection when the session was
	// seeded; the platform's buffer does not hold it. See onCommit.
	selected bool

	// caret is the caret the session was opened with, in device space. The
	// platform takes it once, at the start of a session, and keeps the
	// picture shifted so that it stays above the keyboard; see inStep.
	caret geom.Rect

	// shown is the composition currently shown as text before the caret.
	shown string

	// open is whether a session is open; see [imeBridge.update].
	open bool

	// judged and judgedServe are the focus and the answer of the last
	// [imeBridge.update], which [imeBridge.report] compares against to find a
	// request this tick's input made.
	judged      gift.NodeRef
	judgedServe bool
}

func newIMEBridge(app *gift.App, log *slog.Logger, scale func() float64) *imeBridge {
	b := &imeBridge{app: app, log: log, scale: scale}
	b.c.OnNewSession = b.onNewSession
	b.c.OnComposition = func(c *textinput.Composition) { b.onComposition(c.Text()) }
	b.c.OnCommit = func(c *textinput.Commit) {
		bef, aft := c.SurroundingText()
		b.onCommit(c.Text(), bef, aft)
	}
	b.c.OnEndByUser = b.onEndByUser
	return b
}

// update runs once per tick in the input phase and reports whether the system
// keyboard consumed input this tick, in which case the characters Ebitengine
// reports are not typed a second time.
//
// It also reports to the App whether the system keyboard serves the request:
// only while a session is open, so that a field the platform refused a
// session for — text that is not valid UTF-8, see [imeBridge.onNewSession] —
// falls back to gift's drawn keyboard instead of being served by none. One
// refusal the bridge cannot see: on Android, a view without a text input
// driver gets no session and no error from exp/textinput.
func (b *imeBridge) update() (handled bool) {
	st, ok := b.app.TextInput()
	serve := b.app.SoftKeyboardRequested() && ok
	focus, _ := b.app.Focus()
	if b.open && (!serve || focus != b.focus || !b.inStep(st)) {
		// The field went away, another one took the focus, or the field
		// changed by other means than the keyboard: a tap moved the caret,
		// a key edited, a binding rewrote the text. What was composed stays
		// where it was typed, and the next session is seeded from the field
		// as it is now; a replacement computed against the old text would
		// land in the wrong place.
		b.shown = ""
		b.c.Cancel()
		b.open = false
	}
	b.judged, b.judgedServe = focus, serve
	if !serve {
		b.app.SetPlatformKeyboardActive(false)
		return false
	}
	handled, err := b.c.Update()
	if err != nil {
		// The composer dropped its session; the bridge follows, so that no
		// key is swallowed for a buffer that is gone.
		b.shown = ""
		b.open = false
		if b.log != nil {
			b.log.Warn("gift/backend/ebiten: text input", slog.Any("err", err))
		}
	}
	b.app.SetPlatformKeyboardActive(b.open)
	return handled
}

// report tells the App, after the input of this tick has been dispatched and
// before the build, whether the system keyboard serves the request as it now
// stands. When a backend has to report, and why, is the rule of
// [gift.App.SetPlatformKeyboardActive]; this is the Ebitengine half of it.
//
// The answer for a new request is optimistic, because the session opens only
// in the next [imeBridge.update], which corrects it where the platform
// refuses one. A request that update already judged, [imeBridge.judgedServe]
// for the same focus, keeps that answer.
func (b *imeBridge) report() {
	_, ok := b.app.TextInput()
	serve := b.app.SoftKeyboardRequested() && ok
	if !serve {
		b.app.SetPlatformKeyboardActive(false)
		return
	}
	if focus, _ := b.app.Focus(); !b.judgedServe || focus != b.judged {
		b.app.SetPlatformKeyboardActive(true)
	}
}

// inStep reports whether the field still holds what the session expects:
// the text it was seeded with, with the composition shown before the caret.
//
// A field that reports only a window of a long text around the caret moves
// the window as the composition grows: the window keeps its length, within a
// rune, and loses at its start what the composition added at its end. That is
// still in step, as long as what precedes the composition is the end of the
// text the session was seeded with. The case is accepted only where the
// window really kept its length and the seeded text was long enough to have
// been a window at all, [imeLongText], so that in a short text a deletion made
// elsewhere during a composition is still a change.
//
// The caret counts too, vertically: the platform was told where it is when
// the session opened and shifts the picture to keep it above the keyboard,
// so a caret that moved to another line — the field scrolled, the window
// rotated, the layout changed — needs a session that says so. A caret that
// only moved along its line, as a composition growing does, does not.
//
// While a composition is shown, a caret on another line is accepted all the
// same: a growing word that wraps in a text area moves the caret down, and a
// new session there would end the word mid-way, with its suggestions and its
// autocorrection. The price is a layout that moves the caret during a
// composition: the platform keeps the old caret's shift until the word is
// committed, and the comparison after the commit, against the session seeded
// then, starts a new one.
func (b *imeBridge) inStep(st gift.TextInputState) bool {
	if b.shown == "" && (moved(st.Caret.Min.Y, b.caret.Min.Y) || moved(st.Caret.Max.Y, b.caret.Max.Y)) {
		return false
	}
	if st.After != b.after || st.Selected != (b.selected && b.shown == "") ||
		!strings.HasSuffix(st.Before, b.shown) {
		return false
	}
	rest := st.Before[:len(st.Before)-len(b.shown)]
	if rest == b.before {
		return true
	}
	// The window of a long text; see above.
	n := len(st.Before)
	return len(b.before) >= imeLongText && n < len(b.before)+len(b.shown) &&
		n+utf8.UTFMax > len(b.before) && n <= len(b.before)+utf8.UTFMax &&
		strings.HasSuffix(b.before, rest)
}

// imeLongText is the shortest text before the caret that [imeBridge.inStep]
// takes for a window of a longer one. A client may report a window of a long
// text (see [gift.TextInputState]); ui's fields use 4 KiB. A text shorter than
// this is all the field holds, and compared exactly.
const imeLongText = 1 << 10

func (b *imeBridge) onNewSession() *textinput.SessionOptions {
	st, ok := b.app.TextInput()
	if !ok || !b.app.SoftKeyboardRequested() {
		return nil
	}
	if !utf8.ValidString(st.Before) || !utf8.ValidString(st.After) {
		// exp/textinput refuses such a seed, and the platform could not hold
		// it either: a field bound to bytes that are not text. No session;
		// the field keeps gift's drawn keyboard.
		return nil
	}
	b.focus, _ = b.app.Focus()
	b.before, b.after, b.shown, b.selected, b.caret = st.Before, st.After, "", st.Selected, st.Caret
	b.open = true
	return &textinput.SessionOptions{
		CaretBounds:     scaleRect(st.Caret, b.scale()),
		TextBeforeCaret: st.Before,
		TextAfterCaret:  st.After,
	}
}

func (b *imeBridge) onComposition(text string) {
	if !b.open || text == b.shown {
		return
	}
	b.app.ReplaceText(len(b.shown), 0, text)
	b.shown = text
}

func (b *imeBridge) onCommit(text, bef, aft string) {
	if !b.open {
		return
	}
	del, delAfter, insert := imeReplacement(b.before, b.after, bef, aft, text)
	if b.selected && b.shown == "" && insert == "" {
		// A backspace or a forward delete with a selection. The platform's
		// buffer had no selection and deleted the rune next to the caret;
		// the user meant the selection, which a replacement deletes anyway.
		del, delAfter = 0, 0
	}
	b.app.ReplaceText(len(b.shown)+del, delAfter, insert)
	b.shown = ""
	// The composer ends the session after a commit and asks for a new one,
	// seeded from the field as it is now.
	b.open = false
}

// canDelete reports whether k is a key whose whole effect in a text field
// is an edit the system keyboard delivers itself, through the buffer of the
// open session: backspace where the buffer holds text before the caret, the
// composition shown counted, and forward delete where it holds text after.
//
// Where the buffer holds nothing in that direction the keyboard has nothing
// to delete in it and no replacement follows: the caret at the start of the
// text, or a selection that begins there — select all, then backspace —
// whose text the buffer never held; see [gift.TextInputState]. The key is
// then the field's to take. The text a field reports is a window of the
// document, but one that ends only where the document does, so an empty
// side is the edge of the document, where the field's own deletion in that
// direction is the selection or nothing.
//
// The answer is the session's as it stands when the key is read, after
// [imeBridge.update]: a deletion committed in the same tick has already
// reseeded the session from the field, and the field, with the deletion
// applied, then holds nothing more to delete only if the key would delete
// nothing either.
func (b *imeBridge) canDelete(k gift.Key) bool {
	if !b.open {
		return false
	}
	switch k {
	case gift.KeyBackspace:
		return b.before != "" || b.shown != ""
	case gift.KeyDelete:
		return b.after != ""
	}
	return false
}

func (b *imeBridge) onEndByUser() {
	b.shown = ""
	b.open = false
	b.app.DismissSoftKeyboard()
}

// imeReplacement expresses a commit of exp/textinput as the caret relative
// replacement of [gift.App.ReplaceText].
//
// The session was seeded with before and after, the caret between them. The
// commit says the document is now prefix + text + suffix, where prefix and
// suffix are what [textinput.Commit.SurroundingText] left of before + after:
// the region between them was replaced. In the usual case the region lies
// around the caret — an autocorrection replaces the word before it, a
// backspace deletes the rune before it — and the answer is how much to delete
// on either side. A region that lies entirely on one side of the caret keeps
// the text between it and the caret by inserting that text again, so the
// document comes out right in every case.
func imeReplacement(before, after, prefix, suffix, text string) (deleteBefore, deleteAfter int, insert string) {
	caret := len(before)
	start := len(prefix)
	end := len(before) + len(after) - len(suffix)
	if end < start {
		// Not a region; nothing sensible can be replaced. Insert at the caret.
		return 0, 0, text
	}
	insert = text
	if start <= caret {
		deleteBefore = caret - start
	} else {
		insert = after[:start-caret] + insert
	}
	if end >= caret {
		deleteAfter = end - caret
	} else {
		// The caret then ends behind the text kept, not behind the commit,
		// where exp/textinput would put it. No keyboard has been seen to
		// replace a region that ends before the caret.
		insert += before[end:]
	}
	return deleteBefore, deleteAfter, insert
}

// scaleRect converts a rectangle of gift's device space into Ebitengine's
// logical pixels, rounded outwards.
func scaleRect(r geom.Rect, s float64) image.Rectangle {
	if !(s > 0) {
		s = 1
	}
	return image.Rect(
		int(math.Floor(float64(r.Min.X)*s)), int(math.Floor(float64(r.Min.Y)*s)),
		int(math.Ceil(float64(r.Max.X)*s)), int(math.Ceil(float64(r.Max.Y)*s)))
}

// moved reports whether two coordinates of the caret differ by more than half
// a pixel.
func moved(a, b float32) bool { return a-b > 0.5 || b-a > 0.5 }
