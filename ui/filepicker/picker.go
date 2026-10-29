package filepicker

import (
	"errors"
	"io/fs"
	"log/slog"
	"path"
	"strconv"
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

var pickerType = gift.RegisterType("filepicker.Picker")

// PickerView is the declaration of a file picker. Create one with [New].
//
//	ui.Sheet(screen, filepicker.New(os.DirFS(root), open, cancel).
//	    Mode(filepicker.OpenMulti).
//	    Extensions(".jpg", ".png")).
//	    Presented(picking).
//	    OnDismiss(cancel)
//
// # What it looks like
//
// A header with Cancel, the title and the primary action; a path bar with an
// up button and the directories down to the current one, each of which can be
// tapped; a filter field; and the directory itself as a [ui.VirtualList],
// directories first. In [Save] mode a name field sits under the list.
//
// It fills the space it is given and brings no frame of its own, so it is
// equally at home in a [ui.Sheet], in a [ui.Modal] with a Frame, or as a
// screen of a navigation stack.
//
// # What it reads, and when
//
// A directory is read when it is opened, off the UI goroutine, through the
// fs.FS handed to [New]; the list keeps showing the previous directory, under
// a progress bar, until the new one has arrived. A directory that cannot be
// read leaves the picker where it was, with the error above the list, and the
// error is logged to [PickerView.Logger]. A listing is a snapshot: the picker
// does not watch the directory.
//
// The settings are read on every build, with three exceptions.
// [PickerView.Start], which is where the picker opens, and
// [PickerView.SuggestedName], which fills the name field, apply when it is
// mounted. What [PickerView.Mode], [PickerView.Extensions],
// [PickerView.Filter] and [PickerView.ShowHidden] decide about the listing —
// which entries are listed, and which can be picked — is applied when a
// directory is read: changing them reaches the next directory opened, not the
// one on screen.
//
// # Paths
//
// Every path the picker reports is an fs.FS path, relative to the root of the
// file system it was given and separated by "/": "photos/beach.jpg", or "."
// for the root itself in [Directory] mode. For an os.DirFS, join it back onto
// the directory the file system was made from.
type PickerView struct {
	fsys       fs.FS
	onPick     func([]string)
	onCancel   func()
	mode       Mode
	start      string
	title      string
	rootLabel  string
	exts       []string
	filter     func(fs.DirEntry) bool
	showHidden bool
	suggested  string
	log        *slog.Logger
	key        string
	flex       float32
}

// New returns a picker over fsys that calls onPick with the picked paths and
// onCancel when the person backs out. The default mode is [Open].
//
// onPick receives one path in every mode but [OpenMulti], and never an empty
// slice. Neither function is called while a listing is being read, and both
// run on the UI goroutine.
func New(fsys fs.FS, onPick func(paths []string), onCancel func()) PickerView {
	if fsys == nil {
		panic("gift/ui/filepicker: New with a nil fs.FS")
	}
	if onPick == nil {
		panic("gift/ui/filepicker: New with a nil onPick")
	}
	return PickerView{fsys: fsys, onPick: onPick, onCancel: onCancel, start: ".", rootLabel: "Files"}
}

// Mode sets what the picker picks; see [Mode].
func (p PickerView) Mode(m Mode) PickerView {
	if m > Save {
		panic("gift/ui/filepicker: Mode(" + strconv.Itoa(int(m)) + ") is not one of Open, OpenMulti, Directory and Save")
	}
	p.mode = m
	return p
}

// Start sets the directory the picker opens in, as an fs.FS path. It applies
// when the picker is mounted. The default is the root, ".".
func (p PickerView) Start(dir string) PickerView {
	if !fs.ValidPath(dir) {
		panic("gift/ui/filepicker: Start(" + strconv.Quote(dir) + ") is not a valid fs.FS path")
	}
	p.start = dir
	return p
}

// Title sets the line in the middle of the header. The default names the
// mode: "Open", "Choose a folder", "Save".
func (p PickerView) Title(s string) PickerView { p.title = s; return p }

// RootLabel sets the name of the root in the path bar. The default is
// "Files".
func (p PickerView) RootLabel(s string) PickerView { p.rootLabel = s; return p }

// Extensions limits the files that can be picked to these extensions,
// ignoring case: ".jpg" and "JPG" are the same. Other files are listed,
// disabled. Directories are never limited.
func (p PickerView) Extensions(exts ...string) PickerView {
	p.exts = normalizeExts(exts)
	return p
}

// Filter limits the files that can be picked to those fn accepts, on top of
// [PickerView.Extensions]. It is called off the UI goroutine, once per file,
// while a directory is read, and must be safe for that.
func (p PickerView) Filter(fn func(fs.DirEntry) bool) PickerView { p.filter = fn; return p }

// ShowHidden lists the entries whose name starts with a dot.
func (p PickerView) ShowHidden(b bool) PickerView { p.showHidden = b; return p }

// SuggestedName fills the name field of [Save] mode when the picker is
// mounted.
func (p PickerView) SuggestedName(s string) PickerView { p.suggested = s; return p }

// Logger sets where a directory that cannot be read is reported. The default
// is nowhere; gift never logs to slog.Default.
func (p PickerView) Logger(l *slog.Logger) PickerView { p.log = l; return p }

// Key sets the reconciliation key of this view among its siblings.
func (p PickerView) Key(s string) PickerView { p.key = s; return p }

// Flex makes the picker take a share of the remaining main axis space of its
// parent stack.
func (p PickerView) Flex(f float32) PickerView { p.flex = f; return p }

// ViewType implements gift.View.
func (p PickerView) ViewType() gift.TypeID { return pickerType }

// Build implements gift.View.
//
// The picker keeps state — the directory it shows, the listing, the selection
// — so its body is a [gift.Component] of its own, inside a stack that carries
// the key, the flex and the escape key.
func (p PickerView) Build(bc *gift.BuildContext) gift.Element {
	e := ui.ZStack(gift.Component("filepicker", p.body)).Key(p.key).Flex(p.flex).Build(bc)
	e.Interactor = escapeKey{onCancel: p.onCancel}
	// Escape arrives here on a touchscreen too, where nothing is focused;
	// see gift.Element.KeyFallback. Inside a ui.Sheet the picker is later
	// in document order than the sheet's panel, so the picker's cancel wins
	// over the sheet's dismiss, and the application closes the sheet from
	// onCancel.
	e.KeyFallback = true
	return e
}

// escapeKey cancels the picker on escape when nothing inside it took the key.
type escapeKey struct{ onCancel func() }

// HandleEvent implements gift.Interactor.
func (k escapeKey) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventKeyDown || e.Key != gift.KeyEscape || k.onCancel == nil {
		return false
	}
	k.onCancel()
	return true
}

// --- the retained state ------------------------------------------------------

// picker is everything the picker keeps between builds that is not a
// comparable value: the listing, the selection, the editors and the list. It
// lives behind one pointer in one state slot; the slots that cause rebuilds
// are the small comparable ones in [slots].
type picker struct {
	// loaded is the directory the listing is of, and loading the one a read
	// was started for, "" for none.
	loaded  string
	loading string
	// from is the directory the person was in when they asked for the one
	// being read; a read that fails goes back to it.
	from    string
	listing listing
	// failed says that the listing is empty because its directory could
	// not be read, which is only ever the start directory: any other
	// failure goes back to the directory the person came from.
	failed bool
	// shown is the listing narrowed to the filter, rebuilt by every build.
	shown []entry
	// selected holds the names of the selected files of the current
	// directory.
	selected map[string]bool
	// err is the message of the last directory that could not be read, ""
	// for none.
	err string

	list   *ui.ListState
	query  *ui.TextEditor
	name   *ui.TextEditor
	reason string // why the typed name cannot be saved, "" when it can
}

// slots are the state slots of the picker's component.
type slots struct {
	dir     *gift.State[string]
	version *gift.State[int] // bumped when anything in [picker] changes
	confirm *gift.State[bool]
}

func (s slots) touch() { s.version.Set(s.version.Get() + 1) }

func (p PickerView) body(ctx *gift.Context) gift.View {
	st := ctx.State("picker", (*picker)(nil))
	pk := st.Get()
	if pk == nil {
		pk = &picker{
			selected: map[string]bool{},
			list:     ui.NewListState(),
			query:    ui.NewTextEditor(""),
			name:     ui.NewTextEditor(p.suggested),
		}
		st.Set(pk)
	}
	sl := slots{
		dir:     ctx.State("dir", p.start),
		version: ctx.State("version", 0),
		confirm: ctx.State("confirm", false),
	}
	dir := ctx.Read(sl.dir)
	ctx.Read(sl.version)
	confirm := ctx.Read(sl.confirm)

	if pk.loaded != dir && pk.loading != dir {
		p.load(ctx, pk, sl, dir)
	}
	pk.shown = filterEntries(pk.shown, pk.listing.entries, pk.query.Text())
	if p.mode == Save {
		pk.reason = p.nameProblem(pk)
	}

	body := ui.VStack(
		p.header(pk, sl),
		p.pathBar(pk, sl, dir),
		ui.TextField(pk.query).
			Placeholder("Filter").
			OnChange(func(string) { sl.touch() }).
			Key("filter"),
		p.status(pk),
		p.rows(pk, sl),
		p.saveRow(pk, sl),
	).Gap(12).Padding(16)

	var alert gift.View
	if confirm {
		name := strings.TrimSpace(pk.name.Text())
		alert = ui.Alert(
			"Replace "+strconv.Quote(name)+"?",
			"A file with that name exists in this folder. Saving replaces it.",
			ui.AlertCancel("Cancel", func() { sl.confirm.Set(false) }),
			ui.AlertDestructive("Replace", func() {
				sl.confirm.Set(false)
				p.onPick([]string{path.Join(pk.listing.dir, name)})
			}),
		)
	}
	return ui.Modal(body, alert)
}

// load starts reading dir. The result arrives through the request's token, so
// a read that was overtaken by another, or that finishes after the picker was
// unmounted, is dropped.
func (p PickerView) load(ctx *gift.Context, pk *picker, sl slots, dir string) {
	pk.from = pk.loaded
	pk.loading = dir
	tok := ctx.Request()
	fsys := p.fsys
	r := rules{mode: p.mode, showHidden: p.showHidden, exts: p.exts, filter: p.filter}
	log := p.log
	go func() {
		l, err := listDir(fsys, dir, r)
		tok.Post(func() {
			pk.loading = ""
			if err != nil {
				if log != nil {
					log.Warn("filepicker: cannot read a directory", "dir", dir, "err", err)
				}
				pk.err = readError(dir, err)
				if pk.from != "" && pk.from != dir {
					// Back where the person was; that listing is still here.
					sl.dir.Set(pk.from)
				} else {
					// Nowhere to go back to: the start directory itself.
					pk.loaded, pk.listing, pk.failed = dir, listing{dir: dir}, true
				}
				sl.touch()
				return
			}
			if dir != pk.listing.dir {
				// The selection names files of the directory it was made
				// in, and this is another one.
				clear(pk.selected)
			}
			pk.loaded, pk.listing, pk.err, pk.failed = dir, l, "", false
			sl.touch()
		})
	}()
}

// readError is the line shown above the list for a directory that could not
// be read.
func readError(dir string, err error) string {
	name := path.Base(dir)
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "No permission to open " + strconv.Quote(name) + "."
	case errors.Is(err, fs.ErrNotExist):
		return strconv.Quote(name) + " does not exist."
	}
	return strconv.Quote(name) + " cannot be opened."
}

// open goes into dir, leaving the selection and the filter behind.
func (p PickerView) open(pk *picker, sl slots, dir string) {
	clear(pk.selected)
	pk.query.SetText("")
	pk.err = ""
	pk.list.SetCursor(-1)
	pk.list.Reveal(0)
	sl.dir.Set(dir)
	sl.touch()
}

// tap is what a tap on row i of the shown entries does.
//
// The entries are those of pk.listing, which while another directory is being
// read is still the previous one, so every path is joined onto the
// directory of the listing and not onto the directory being read.
//
// Nothing happens while a directory is being read. The rows still on the
// screen are about to be replaced, and a selection made in them would either
// be lost or, worse, name a file of the same name in the directory arriving.
func (p PickerView) tap(pk *picker, sl slots, i int) {
	if pk.loading != "" || i < 0 || i >= len(pk.shown) {
		return
	}
	e := pk.shown[i]
	dir := pk.listing.dir
	if e.dir {
		p.open(pk, sl, path.Join(dir, e.name))
		return
	}
	switch p.mode {
	case Open:
		if pk.selected[e.name] {
			p.onPick([]string{path.Join(dir, e.name)})
			return
		}
		clear(pk.selected)
		pk.selected[e.name] = true
	case OpenMulti:
		if pk.selected[e.name] {
			delete(pk.selected, e.name)
		} else {
			pk.selected[e.name] = true
		}
	case Save:
		pk.name.SetText(e.name)
	}
	sl.touch()
}

// primary is the header's action: open, choose or save. It is not available
// while a directory is read, so the listing is of the directory shown.
func (p PickerView) primary(pk *picker, sl slots) {
	if !p.canPick(pk) {
		return
	}
	dir := pk.listing.dir
	switch p.mode {
	case Directory:
		p.onPick([]string{dir})
	case Save:
		name := strings.TrimSpace(pk.name.Text())
		if p.exists(pk, name) {
			sl.confirm.Set(true)
			return
		}
		p.onPick([]string{path.Join(dir, name)})
	default:
		// In listing order, which is the order the person sees.
		var out []string
		for _, e := range pk.listing.entries {
			if pk.selected[e.name] {
				out = append(out, path.Join(dir, e.name))
			}
		}
		if len(out) == 0 {
			// New promises never to report an empty pick.
			return
		}
		p.onPick(out)
	}
}

// canPick reports whether the primary action is available.
func (p PickerView) canPick(pk *picker) bool {
	if pk.loading != "" {
		return false
	}
	switch p.mode {
	case Directory:
		return pk.loaded != "" && !pk.failed
	case Save:
		return !pk.failed && pk.reason == ""
	default:
		return len(pk.selected) > 0
	}
}

// nameProblem is why the name typed in Save mode cannot be saved, "" when it
// can.
func (p PickerView) nameProblem(pk *picker) string {
	name := strings.TrimSpace(pk.name.Text())
	if err := validateName(pk.listing.dir, name); err != nil {
		return err.Error()
	}
	if isDir, ok := pk.listing.index[name]; ok && isDir {
		return errNameIsFolder.Error()
	}
	return ""
}

// exists reports whether a file called name is in the listing.
func (p PickerView) exists(pk *picker, name string) bool {
	isDir, ok := pk.listing.index[name]
	return ok && !isDir
}

// --- the parts ---------------------------------------------------------------

func (p PickerView) header(pk *picker, sl slots) gift.View {
	title := p.title
	if title == "" {
		title = [...]string{Open: "Open", OpenMulti: "Open", Directory: "Choose a folder", Save: "Save"}[p.mode]
	}
	action := [...]string{Open: "Open", OpenMulti: "Open", Directory: "Choose", Save: "Save"}[p.mode]
	if p.mode == OpenMulti && len(pk.selected) > 0 {
		action += " (" + strconv.Itoa(len(pk.selected)) + ")"
	}
	can := p.canPick(pk)
	fg := ui.ColorOnAccent
	if !can {
		fg = ui.ColorSecondaryLabel
	}
	cancel := gift.View(ui.Spacer().Key("no-cancel"))
	if p.onCancel != nil {
		cancel = ui.Button(ui.Text("Cancel"), p.onCancel).MinHeight(ui.ControlHitTarget).Key("cancel")
	}
	return ui.HStack(
		cancel,
		ui.Spacer(),
		ui.Text(title).FontSize(17).MaxLines(1).Key("title"),
		ui.Spacer(),
		ui.Button(ui.Text(action).Foreground(fg), func() { p.primary(pk, sl) }).
			Style(ui.ButtonStyle{Background: ui.ColorAccent, CornerRadius: 9}).
			DisabledStyle(ui.ButtonStyle{Background: ui.ColorControlDisabled}).
			Disabled(!can).
			MinHeight(ui.ControlHitTarget).
			Label(action).
			Key("primary"),
	).Align(geom.Center).Gap(8)
}

func (p PickerView) pathBar(pk *picker, sl slots, dir string) gift.View {
	parts := []gift.View{
		ui.Button(ui.Icon(outline.ArrowUp).Size(18), func() {
			if dir != "." {
				p.open(pk, sl, parent(dir))
			}
		}).Disabled(dir == ".").Label("Up").MinHeight(36).Key("up"),
	}
	cs := crumbs(dir, p.rootLabel)
	for i, c := range cs {
		if i > 0 {
			parts = append(parts, ui.Text("/").Foreground(ui.ColorSecondaryLabel).Key("sep"+strconv.Itoa(i)))
		}
		fg := ui.ColorAccent
		if i == len(cs)-1 {
			fg = ui.ColorLabel
		}
		target := c.dir
		parts = append(parts, ui.Button(ui.Text(c.label).Foreground(fg).MaxLines(1), func() {
			if target != dir {
				p.open(pk, sl, target)
			}
		}).
			Background(ui.ColorClear).Border(ui.Border{}).
			Padding(4).MinHeight(36).
			Label(c.label).
			Key("crumb"+strconv.Itoa(i)))
	}
	return ui.HScroll(ui.HStack(parts...).Align(geom.Center).Gap(4)).Key("path")
}

// status is the line between the filter and the list: the error, a progress
// bar while a directory is read, or nothing.
func (p PickerView) status(pk *picker) gift.View {
	switch {
	case pk.err != "":
		return ui.Text(pk.err).FontSize(13).Foreground(ui.ColorDanger).Key("error")
	case pk.loading != "":
		return ui.ProgressBar(0).Indeterminate().Key("loading")
	}
	return ui.Box().Frame(0, 0).Key("idle")
}

func (p PickerView) rows(pk *picker, sl slots) gift.View {
	if len(pk.shown) == 0 && pk.loading == "" {
		msg := "This folder is empty."
		if len(pk.listing.entries) > 0 {
			msg = "Nothing here matches the filter."
		}
		return ui.VStack(ui.Text(msg).Foreground(ui.ColorSecondaryLabel)).
			Align(geom.Center).Padding(24).Flex(1).Key("empty")
	}
	shown := pk.shown
	return ui.Card(
		ui.VirtualList(pk.list, len(shown), func(i int) ui.ListRow {
			e := shown[i]
			r := ui.ListRow{
				Icon:     symbolFor(e),
				Title:    e.name,
				Subtitle: describe(e),
				Selected: pk.selected[e.name],
				Disabled: !e.pickable,
			}
			if e.dir {
				r.Chevron = outline.AngleRight
			}
			return r
		}).
			SelectedIcon(outline.Check).
			OnTap(func(i int) { p.tap(pk, sl, i) }).
			Label("Files").
			Flex(1).
			Key("list"),
	).Padding(0).Flex(1).Key("card")
}

func (p PickerView) saveRow(pk *picker, sl slots) gift.View {
	if p.mode != Save {
		return ui.Box().Frame(0, 0).Key("no-save")
	}
	field := ui.TextField(pk.name).
		Placeholder("Name").
		Label("Name").
		OnChange(func(string) { sl.touch() }).
		OnSubmit(func(string) { p.primary(pk, sl) }).
		Flex(1).
		Key("name")
	note := ""
	if pk.reason != "" && pk.name.Text() != "" {
		note = pk.reason
	}
	return ui.VStack(
		ui.HStack(ui.Text("Save as"), field).Align(geom.Center).Gap(8),
		ui.Text(note).FontSize(13).Foreground(ui.ColorDanger).Key("name-problem"),
	).Gap(4).Key("save")
}

// symbolFor picks the icon of an entry from its kind and its extension.
func symbolFor(e entry) ui.Symbol {
	switch {
	case e.broken:
		return outline.ExclamationCircle
	case e.dir:
		return outline.Folder
	}
	switch strings.ToLower(path.Ext(e.name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".bmp", ".tif", ".tiff":
		return outline.FileImage
	case ".txt", ".md", ".csv", ".log":
		return outline.FileLines
	case ".pdf":
		return outline.FilePdf
	case ".mp4", ".mov", ".mkv", ".webm":
		return outline.FileVideo
	case ".mp3", ".wav", ".flac", ".ogg":
		return outline.FileMusic
	case ".zip", ".tar", ".gz", ".7z":
		return outline.FileZip
	case ".go", ".js", ".ts", ".py", ".json", ".html", ".css", ".xml", ".yaml", ".yml":
		return outline.FileCode
	}
	return outline.File
}
