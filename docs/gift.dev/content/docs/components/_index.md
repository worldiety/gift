---
title: Components
weight: 3
sidebar:
  open: false
---

Components are the views you build an interface from. They live in package
[`ui`](https://github.com/worldiety/gift/tree/main/ui), the file picker in
[`ui/filepicker`](https://github.com/worldiety/gift/tree/main/ui/filepicker). Every page shows a screenshot, the code
that drew it, how to use the component well and its API. The pages are grouped into:

{{< cards >}}
  {{< card link="basic/" title="Basic" icon="cursor-click" subtitle="Text, icons, images, badges, buttons and the input controls." >}}
  {{< card link="layout/" title="Layout" icon="view-boards" subtitle="Stacks, ZStack, window, box, spacer, card and scroll view." >}}
  {{< card link="lists/" title="Lists" icon="view-list" subtitle="Lists, rows, virtual lists and image galleries." >}}
  {{< card link="navigation/" title="Navigation" icon="map" subtitle="Navigation stack and tab bar." >}}
  {{< card link="dialogs/" title="Dialogs & Overlays" icon="collection" subtitle="Modal, alert, sheet, on-screen keyboard and file picker." >}}
  {{< card link="styling/" title="Styling" icon="color-swatch" subtitle="Colors, shadow and border, gradients and glass." >}}
{{< /cards >}}

## Views are values

A constructor such as `ui.Text` or `ui.Button` returns a concrete value type: `ui.TextView`, `ui.ButtonView`. Every
one of them implements `gift.View`. You configure it with chained methods, and each method returns the same concrete
type, so the chain keeps all methods of the view available:

```go
ui.VStack(
	ui.Box().Frame(80, 24).Background(ui.RGB(200, 40, 40)),
	ui.Spacer(),
	ui.Box().Frame(80, 24),
).Gap(8).Padding(16).CornerRadius(12)
```

A method never changes the value it is called on. It returns a changed copy. If you build a view step by step,
assign the result:

```go
t := ui.Text("Hello")
t.FontSize(20)     // no effect, the result is discarded
t = t.FontSize(20) // correct
```

A modifier sets a field of the view. It does not wrap the view in another one, so the drawing order is fixed
(shadow, background, content, border) and setting a modifier twice replaces the first value.
[Modifiers](../concepts/modifiers/) explains the details.

## Each view has only the modifiers it honors

There is no shared modifier interface. A view has exactly the methods it acts on: a `ui.Divider()` has `Color`,
`Thickness`, `Inset`, `Flex` and `Key`, but no `Padding` or `Background`, because it would ignore them. A call that
would do nothing does not compile. The one documented exception is the ZStack: it honors its own `Flex` but ignores
the `Flex` of its children, see [ZStack](layout/zstack/).

The API section of every page lists the constructors and methods of the view. The common modifiers, such as `Frame`,
`Padding` or `Background`, are collapsed into one line that links to [Modifiers](../concepts/modifiers/).

## What is not there

gift has no checkbox, radio button, drop-down, menu, tooltip or general grid layout. Use a [toggle](basic/toggle/) for an on
and off setting, a [segmented control](basic/segmented-control/) for a small set of choices and a [list](lists/list/)
of [rows](lists/row/) with chevrons for longer ones. When you need something else, compose it from the views here
or write a [custom view](../concepts/custom-views/).
