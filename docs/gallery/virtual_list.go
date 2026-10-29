package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "virtual-list", Group: "lists", Width: 760, Height: 440}, virtualListDemo)
}

func virtualListDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The retained half of each list lives in state, so the pool of rows and
	// the scroll position survive every build.
	files := ctx.State("files", (*ui.ListState)(nil))
	if files.Get() == nil {
		files.Set(ui.NewListState())
	}
	log := ctx.State("log", (*ui.ListState)(nil))
	if log.Get() == nil {
		s := ui.NewListState()
		s.Reveal(48_210) // start in the middle of the log
		log.Set(s)
	}

	// Which row is selected is your model; the list only draws it. Read it
	// with ctx.Read, so that a tap rebuilds the list and rebinds its rows.
	selected := ctx.State("selected", 2)
	sel := ctx.Read(selected)

	names := []string{"Documents", "Downloads", "Invoices", "System", "Photos",
		"Archive.zip", "Notes.txt", "Report.pdf", "Budget.xlsx"}

	// A file browser: icons, subtitles, trailing values, a check mark for
	// the selected row and a disabled row.
	browser := ui.VirtualList(files.Get(), len(names), func(i int) ui.ListRow {
		row := ui.ListRow{
			Icon:     outline.Folder,
			Title:    names[i],
			Subtitle: fmt.Sprintf("%d items", 12+i*7),
			Chevron:  outline.ChevronRight,
			Selected: i == sel,
			Disabled: names[i] == "System",
		}
		if strings.Contains(names[i], ".") {
			row.Icon, row.Chevron = outline.File, ui.Symbol{}
			row.Subtitle, row.Value = "Edited today", fmt.Sprintf("%d KB", 40+i*13)
		}
		return row
	}).
		OnTap(selected.Set).
		SelectedIcon(outline.Check).
		Label("Files").
		Frame(340, 352).
		Background(ui.ColorSurface).CornerRadius(12).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator})

	// A hundred thousand rows cost as much as a screenful: only the visible
	// rows have nodes, and row is called for those alone. 352 is eight rows
	// of ui.RowHeight, so the log shows whole rows.
	events := ui.VirtualList(log.Get(), 100_000, func(i int) ui.ListRow {
		return ui.ListRow{
			Title: "Event " + strconv.Itoa(i+1),
			Value: fmt.Sprintf("%02d:%02d", i/60%24, i%60),
		}
	}).
		RowHeight(ui.RowHeight).
		Separators(false).
		Label("Event log").
		Frame(240, 352).
		Background(ui.ColorSurface).CornerRadius(12).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator})

	caption := func(s string) gift.View {
		return ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel)
	}
	return ui.HStack(
		ui.VStack(caption("Files"), browser).Gap(8),
		ui.VStack(caption("Event log, 100,000 rows"), events).Gap(8),
	).Gap(24)
	// snippet:end
}
