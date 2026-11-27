---
title: Menu
---

A menu is a control that opens a list of actions: share, rename, delete, or a choice of how to sort. It opens below
its label as soon as it is pressed, lined up with the label's leading edge, and grows out of it as on iOS. A finger
can slide on from the label onto an item and lift there to choose it, without a second tap.

{{< gallery-shot alt="An Options button with an open menu below it: Share and Bookmark with icons, a Sort by section with a check mark at Date, and a red Delete with a trash icon" >}}

{{< gallery-code >}}

## Usage

`ui.Menu(label, items...)` makes any view the label of a menu:

```go
ui.Menu(ui.Icon(outline.DotsHorizontal),
	ui.MenuItem("Share", share).Icon(outline.ShareNodes),
	ui.MenuSection("Sort by",
		ui.MenuItem("Date", byDate).Checked(sort == "date"),
		ui.MenuItem("Name", byName).Checked(sort == "name"),
	),
	ui.MenuSection("",
		ui.MenuItem("Delete", remove).Icon(outline.TrashBin).Destructive(),
	),
)
```

| Element                       | What it is                                                                       |
|-------------------------------|----------------------------------------------------------------------------------|
| `MenuItem(title, action)`     | an action; the menu closes before it runs                                         |
| `.Icon(symbol)`               | a symbol at the trailing end                                                     |
| `.Checked(b)`                 | a check mark in front: the current choice                                        |
| `.Destructive()`              | drawn in `ColorDanger`                                                           |
| `.Disabled(b)`                | shown, not choosable                                                             |
| `MenuDivider()`               | a hairline                                                                        |
| `MenuSection(title, items...)` | items set apart by a hairline with room around it, under a small title when there is one |

## Choosing

- **Tap**: the label opens the menu, a tap on an item chooses it.
- **Press, slide, lift**: the menu opens on the press; the item under the finger is highlighted while it slides, and
  lifting on it chooses it. Lifting elsewhere after sliding closes the menu.
- **Mouse**: the item under the pointer is highlighted.
- **Keys**: enter or space opens the menu with the first item highlighted; the arrow keys, home and end move, enter
  chooses, escape closes.

A press outside the menu closes it, and so do Android's back button and back gesture, without going back any
further. See [Android and iOS](../../../concepts/mobile/#back).

The items are 44 pixels tall and the menu is 250 wide, the metrics of iOS, so a finger hits them on a five inch
panel.

## API

- [`ui.Menu`](https://pkg.go.dev/github.com/worldiety/gift/ui#Menu)
- [`ui.MenuItem`](https://pkg.go.dev/github.com/worldiety/gift/ui#MenuItem),
  [`ui.MenuSection`](https://pkg.go.dev/github.com/worldiety/gift/ui#MenuSection),
  [`ui.MenuDivider`](https://pkg.go.dev/github.com/worldiety/gift/ui#MenuDivider)
