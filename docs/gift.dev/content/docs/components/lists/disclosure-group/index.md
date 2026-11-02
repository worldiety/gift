---
title: Disclosure Group
---

A disclosure group is a row that shows or hides content below it, after SwiftUI's
[`DisclosureGroup`](https://developer.apple.com/documentation/swiftui/disclosuregroup): advanced settings under
their basic ones, the answer under a question. Its chevron points to the right while the content is hidden and
turns down as it opens. An accordion is a list of them with at most one open.

{{< gallery-shot alt="A settings card with an open group Advanced showing three indented rows, beside a card of three questions of which the first is open and shows its answer" >}}

{{< gallery-code >}}

## Usage

```go
ui.DisclosureGroup("Advanced",
	ui.Toggle(ctx.Read(debug), debug.Set),
	ui.Text("Logs go to the system journal."),
).Icon(outline.Cog)
```

The label is a [Row](../row/) with the title, and `Icon`, `Subtitle`, `Value` and `Leading` give it what they give
a row, so a group sits among the rows of a [List](../list/) or a [Card](../../layout/card/) as one of them, with the
same hover, press, focus and keyboard. A tap, a click, space or enter on the label opens and closes the group.

The content is stacked below the label, eight points apart, and inset as far as the title, so that a paragraph lines
up with it. Rows go into a list of their own, which gives them separators, with an inset that indents them to the
title:

```go
ui.DisclosureGroup("Advanced",
	ui.List(proxy, dns).SeparatorInsets(ui.RowPadding, 0),
).Icon(outline.Cog).ContentPadding(geom.Insets{Left: 22 + 12})
```

## Open or closed

Without `Expanded` a group keeps whether it is open and starts closed. With it, the application owns the value, as
with SwiftUI's `isExpanded` binding: the group shows what it is given and reports a tap through the callback.

```go
advanced := ctx.State("advanced", false)
ui.DisclosureGroup("Advanced", content...).Expanded(ctx.Read(advanced), advanced.Set)
```

Opening and closing take `ui.DisclosureDuration`, 220 ms: the content is revealed from the top and the chevron
turns, on gift's clock and without a build per frame. Content that is not shown yet takes no taps. A group built open
shows its content at once.

The content is built when the group opens and leaves the tree when it has closed, as in SwiftUI. A closed group costs
its label and nothing more, however much it holds, and what the content kept in its state starts over the next time
it opens; state that must outlive that belongs to the component around the group.

## Accordion

`ui.Accordion` puts groups into a list and keeps at most one of them open: opening one closes the one that was, and
a tap on the open one closes it. Without `Open` it keeps the open group itself and starts with none.

```go
faq := ctx.State("faq", 0) // the first question open, -1 for none
ui.Card(ui.Accordion(
	ui.DisclosureGroup("How long does shipping take?", ui.Text(shipping)),
	ui.DisclosureGroup("Can I return an order?", ui.Text(returns)),
).Open(ctx.Read(faq), faq.Set)).Padding(0)
```

SwiftUI has no accordion; an app builds one from disclosure groups that share one `isExpanded` state, which is what
`ui.Accordion` does.

## API

{{< api pkg="ui" type="DisclosureGroupView" >}}

{{< api pkg="ui" type="AccordionView" >}}

## Related

- [Row](../row/) is the label of a group.
- [List](../list/) groups rows and disclosure groups with separators.
