---
title: Getting Started
weight: 1
---

Gift is a declarative UI toolkit for Go. You describe your interface with SwiftUI-style views, keep its data in
typed state, and gift draws it on the GPU with [Ebitengine](https://ebitengine.org/). There is no browser, no
WebView and no web stack. Gift focuses on desktop applications and touchscreen kiosks, particularly on
Raspberry Pi 4 and 5.

```go
func counter(ctx *gift.Context) gift.View {
	count := ctx.State("count", 0)
	n := ctx.Read(count)
	return ui.Window(ui.VStack(
		ui.Text(fmt.Sprintf("%d clicks", n)),
		ui.Button(ui.Text("+1"), func() { count.Set(count.Get() + 1) }),
	).Gap(16).Padding(24))
}
```

[Your first app](first-app/) turns this function into a complete program.

## How it works

- **Views are values.** A view is a short-lived Go value such as `ui.Text` or `ui.VStack`. You compose views in
  plain Go functions and configure them with chained methods such as `Gap` or modifiers such as `Padding`. See
  [Views](../concepts/views/) and [Modifiers](../concepts/modifiers/).
- **State is typed.** `ctx.State` creates a named, typed value that belongs to a component. `ctx.Read` returns its
  value and records that the component depends on it. See [State](../concepts/state/).
- **Updates are targeted.** Writing a state rebuilds only the components that read it. Gift keeps a retained tree
  of nodes and lays out only the parts that changed, instead of rebuilding the whole interface.
- **Rendering runs on the GPU.** The backend in `backend/ebiten` opens the window, polls mouse, keyboard and touch
  input, and draws the display list with Ebitengine.
- **One goroutine owns the UI.** Background work hands its result back with `App.Post`. See
  [Concurrency](../concepts/concurrency/).

## Who it is for

Gift fits you if you:

- write a desktop or kiosk application in Go and want to describe its interface in Go as well,
- target modest hardware such as a Raspberry Pi with a touchscreen,
- build image-heavy interfaces: gift brings virtualized galleries, asynchronous image loading and bounded resource
  budgets,
- want to test your interface from Go tests, see [Testing](../concepts/testing/).

Gift is not the right tool if you:

- build a web application: gift draws into a native window, not into a browser,
- need native platform widgets: gift draws every control itself, including its own file picker,
- need screen reader support: gift has no accessibility bridge yet.

## Development status

{{< callout type="warning" >}}
Gift is under development. APIs may still change. 1080p at 60 Hz on a Raspberry Pi is a target, not a verified
performance guarantee.
{{< /callout >}}

## Next steps

{{< cards >}}
  {{< card link="installation/" title="Installation" icon="download" subtitle="Requirements, adding the module and running the examples." >}}
  {{< card link="first-app/" title="Your first app" icon="play" subtitle="Write, run and extend a counter application." >}}
  {{< card link="../concepts/" title="Concepts" icon="book-open" subtitle="Views, state, layout, theming and everything else a gift app is made of." >}}
{{< /cards >}}
