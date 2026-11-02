// Package showcase holds one small demo of every component of gift: the code
// that draws the screenshots of the component pages, and what the kitchen
// sink example shows.
//
// Each demo is a file named like the page it belongs to, with the lines
// between "// snippet:begin" and "// snippet:end" being the code the page
// prints; the site mounts this directory as its assets/gallery, see the
// gallery-code shortcode. docs/gallery runs one demo in a window for
// docs/capture to photograph, and cmd/example-kitchensink runs all of them
// behind a list.
//
// The demos are views and nothing else: they keep their state in the
// component they are built in, so one can be shown, left and shown again. The
// pictures some of them need are written by [Samples], which a program calls
// once before it shows any.
package showcase

import (
	"sort"

	"github.com/worldiety/gift"
)

// Demo is one component demo.
type Demo struct {
	// Name identifies the demo on the command line and names its page bundle.
	Name string `json:"name"`
	// Group is the section of the component pages the demo belongs to, such
	// as "basic" or "layout"; see [Groups].
	Group string `json:"group"`
	// Title is the name of the component, as its page names it, and Summary
	// the sentence the section index says about it.
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// Width and Height are the window size in logical pixels. The screenshot
	// is cropped to what the demo draws, so they only have to be large enough.
	Width  int `json:"width"`
	Height int `json:"height"`
	// Input, if not empty, is the body of a POST to the /input endpoint of
	// the automation interface, sent before the screenshots are taken.
	Input string `json:"input,omitempty"`
	// Settle is the number of frames the screenshot waits for, so that
	// pictures and icons have arrived. Zero selects 8.
	Settle int `json:"settle,omitempty"`
	// FullWindow keeps the whole window instead of cropping to the content,
	// for demos that fill the window, such as a sheet or a tab bar.
	FullWindow bool `json:"fullWindow,omitempty"`

	view func(ctx *gift.Context) gift.View
}

// View builds the demo.
func (d Demo) View(ctx *gift.Context) gift.View { return d.view(ctx) }

var demos = map[string]Demo{}

// register adds a demo. It is called from the init function of each demo file.
func register(d Demo, view func(ctx *gift.Context) gift.View) {
	if _, dup := demos[d.Name]; dup {
		panic("showcase: demo registered twice: " + d.Name)
	}
	if d.Width == 0 {
		d.Width = 640
	}
	if d.Height == 0 {
		d.Height = 360
	}
	d.view = view
	demos[d.Name] = d
}

// Lookup returns the demo called name.
func Lookup(name string) (Demo, bool) {
	d, ok := demos[name]
	return d, ok
}

// Group is a section of the component pages.
type Group struct {
	// Key is the directory of the section, which a [Demo] names as its Group.
	Key string
	// Title is the heading of the section.
	Title string
}

// Groups are the sections of the component pages, in the order the site
// lists them.
func Groups() []Group {
	return []Group{
		{"basic", "Basic"},
		{"layout", "Layout"},
		{"lists", "Lists"},
		{"navigation", "Navigation"},
		{"dialogs", "Dialogs & Overlays"},
		{"styling", "Styling"},
		{"media", "Media"},
		{"charts", "Charts"},
		{"documents", "Documents"},
	}
}

// All returns every demo, by group in the order of [Groups] and by title
// within a group.
func All() []Demo {
	rank := map[string]int{}
	for i, g := range Groups() {
		rank[g.Key] = i
	}
	out := make([]Demo, 0, len(demos))
	for _, d := range demos {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if rank[a.Group] != rank[b.Group] {
			return rank[a.Group] < rank[b.Group]
		}
		return a.Title < b.Title
	})
	return out
}
