---
title: Virtual List
---

A virtual list shows a very long list of uniform rows, such as a directory with ten thousand files or a log with a
hundred thousand entries. Only the rows on screen have nodes. When you scroll, the list reuses the rows that leave
the viewport for the items that enter it, so scrolling costs the same at a thousand items as at a hundred thousand.
No view function runs while you scroll, and `gift.Diagnostics.Builds` does not move.

The price is a fixed row layout. A row is data, a `ui.ListRow`, not a view: a leading icon, a title over an optional
subtitle, a trailing value and a chevron, all rows the same height. Use a [List](../list/) when you need arbitrary
views in your rows, sections or rows of different heights, and the list stays short (a `ListView` builds every row
it has; its documentation puts the limit at about 280 rows on a Raspberry Pi 4).

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### State and data

`ui.VirtualList` takes three things: a `*ui.ListState`, the number of items and a function that returns the
`ui.ListRow` for item `i`.

The `ListState` holds what has to survive a build: the pool of rows, the keyboard cursor and the position of the
last layout. Create it once with `ui.NewListState` and keep it in state. `ctx.State` takes a pointer as readily as a
number:

```go
list := ctx.State("files", (*ui.ListState)(nil))
if list.Get() == nil {
	list.Set(ui.NewListState())
}

return ui.VStack(
	ui.VirtualList(ctx.Read(list), len(files), func(i int) ui.ListRow {
		return ui.ListRow{Icon: outline.File, Title: files[i].Name}
	}).OnTap(open).Flex(1),
)
```

The nil check uses `Get` because it only fills the state on the first build. The value passed to the list is read
with `ctx.Read`, like any other state the build depends on.

One `ListState` belongs to one list. Two virtual lists over the same state panic during layout; create one
`ListState` per list.

The list calls the row function during layout, and only for the visible items. It must not block and must not
write state. It is a lookup into data you already have. Every build of the list reads the visible rows again, so a
change to your data reaches the screen like any other state change: write the state your component reads, and the
rebuild rebinds the rows.

### Rows

A `ListRow` looks like a [Row](../row/) with a fixed set of parts:

- `Icon` is drawn in `ui.ColorAccent` at the leading edge. Without it the labels start at the leading edge.
- `Title` is the first line. It is also the accessible name of the row, so a test finds it with
  `gifttest.ByText(title)`.
- `Subtitle` is a quieter second line.
- `Value` is a trailing label in `ui.ColorSecondaryLabel`.
- `Chevron` is a trailing symbol that says the row leads somewhere.
- `Disabled` quietens the labels and the icon and refuses taps and activation.

A row that is too narrow cuts its labels and its value with an ellipsis. A list never overflows.

### Selection and taps

`OnTap` sets the function that receives the index of a tapped row. A drag that starts on a row scrolls the list
instead of tapping it. A disabled row does not call `OnTap`.

Which rows are selected is your model. Set `Selected` in the row function, and the list tints the row with a light
`ui.ColorAccent`. `SelectedIcon` adds a trailing mark, usually a check mark. The list keeps room for the mark on
every row, so selecting a row does not move its labels.

Read the selection with `ctx.Read` during the build, not with `Get` inside the row function. `Get` registers no
dependency, so a tap would write the state without rebuilding the list, and the rows would keep their old binding:

```go
selected := ctx.State("selected", -1)
sel := ctx.Read(selected)

ui.VirtualList(ctx.Read(list), len(files), func(i int) ui.ListRow {
	return ui.ListRow{Title: files[i].Name, Selected: i == sel}
}).
	OnTap(selected.Set).
	SelectedIcon(outline.Check).
	MaxHeight(400)
```

For multiple selection, keep the set of selected items in your model and answer `Selected` from it.

### Keyboard

The list as a whole takes the keyboard focus, not its rows. The up and down arrows, page up, page down, home and end
move a cursor through the items and scroll it into view. Without a cursor, the first of these keys puts it on the
first item. Enter or space activates the item under the cursor, which calls `OnTap` like a tap. A tap on an enabled row also moves the
cursor to that row. While the focus is visible, the row under the cursor draws a focus ring.

The `ListState` gives you the cursor and the visible items from code:

- `Cursor` returns the item the cursor is on, or -1.
- `SetCursor(i)` moves the cursor to item `i` and scrolls it into view. A negative `i` removes the cursor.
- `Reveal(i)` scrolls item `i` into view in the next layout, by the shortest distance. An item that is already
  fully visible does not move. The demo calls it once on a new `ListState` to start in the middle of the log.
- `VisibleRange` returns the half open range of items the last layout bound to rows.

### Size and look

A virtual list needs a bounded height, like a [Scroll View](../../layout/scroll-view/): give it a `Flex`, a `Frame`
or a `MaxHeight`.

All rows have one height. The default fits a title over a subtitle. `RowHeight` sets another one, but never less
than `ui.RowHeight` (44 points), because a row is a touch target. `Separators(false)` removes the hairlines between
the rows. The hairlines start where the labels start, not under the icons.

`Background`, `Border`, `CornerRadius` and `Shadow` style the viewport; the background does not scroll. `Padding`
is inside the viewport, so the rows scroll through it. `ScrollBar` styles the indicator along the trailing edge,
`Config` sets the gesture constants of the scroll container, and `Label` sets the accessible name of the whole list.

## API

{{< api "VirtualListView" >}}

{{< api "ListState" >}}

`ListRow` is a plain struct, see [ui.ListRow on pkg.go.dev](https://pkg.go.dev/github.com/worldiety/gift/ui#ListRow):

| Field      | Type     | Meaning                                                                  |
|------------|----------|--------------------------------------------------------------------------|
| `Icon`     | `Symbol` | leading symbol in `ColorAccent`; the zero `Symbol` is no icon            |
| `Title`    | `string` | first line and accessible name of the row                                |
| `Subtitle` | `string` | quieter second line                                                      |
| `Value`    | `string` | trailing label                                                           |
| `Chevron`  | `Symbol` | trailing symbol for a row that leads somewhere                           |
| `Selected` | `bool`   | tints the row and shows the mark set with `SelectedIcon`                 |
| `Disabled` | `bool`   | quietens the labels and refuses taps and activation                      |

## Related

- [List](../list/) and [Row](../row/), for short lists with arbitrary rows
- [Image Gallery](../image-gallery/), the virtual container for pictures
- [Scroll View](../../layout/scroll-view/), the scroll bar and the gesture configuration
- [State](../../../concepts/state/), [Input and Focus](../../../concepts/input-and-focus/) and
  [Diagnostics](../../../concepts/diagnostics/)
- The example [kitchen sink](../../../examples/example-kitchensink/), whose list tab scrolls 100,000 rows
