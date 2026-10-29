---
title: Gift
layout: hextra-home
---

{{< hextra/hero-badge link="https://github.com/worldiety/gift" >}}
  <div class="hx:w-2 hx:h-2 hx:rounded-full hx:bg-primary-400"></div>
  <span>Source on GitHub</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  Desktop and kiosk interfaces&nbsp;<br class="hx:sm:block hx:hidden" />in pure Go
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  Declarative views, typed state and a GPU renderer built on Ebitengine.&nbsp;<br class="hx:sm:block hx:hidden" />No browser, no WebView, no C compiler.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
</div>

<figure class="gift-shot" style="max-width: 720px">
  <img class="gift-shot-light" src="docs/examples/example-kitchensink/screenshot-light.png" width="900" height="760" alt="The kitchen sink example: a title bar, a card with badges, a list of rows, a strip of pictures and a tab bar">
  <img class="gift-shot-dark" src="docs/examples/example-kitchensink/screenshot-dark.png" width="900" height="760" alt="The kitchen sink example in the dark theme">
</figure>

<div class="hx:mt-12"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Views are Go values"
    icon="template"
    link="docs/concepts/views/"
    subtitle="You compose the interface from plain Go functions and configure each view with chained modifiers. The compiler checks every one of them."
  >}}
  {{< hextra/feature-card
    title="Typed state"
    icon="refresh"
    link="docs/concepts/state/"
    subtitle="`ctx.State` holds a typed value. Writing it rebuilds only the components that read it, and gift lays out only what changed."
  >}}
  {{< hextra/feature-card
    title="Components included"
    icon="view-grid"
    link="docs/components/"
    subtitle="Buttons, text fields, lists, a navigation stack, a tab bar, sheets, alerts, a file picker and more, each with a screenshot and its API."
  >}}
  {{< hextra/feature-card
    title="Made for kiosks"
    icon="desktop-computer"
    link="docs/concepts/kiosks/"
    subtitle="Touch input, an on-screen keyboard and a draw-on-demand mode for touchscreen appliances. The target hardware is a Raspberry Pi 4 or 5."
  >}}
  {{< hextra/feature-card
    title="Pictures at scale"
    icon="photograph"
    link="docs/concepts/images/"
    subtitle="Virtualized image galleries and a background image pipeline with thumbnails, a disk cache and fixed memory budgets."
  >}}
  {{< hextra/feature-card
    title="Tested from Go"
    icon="beaker"
    link="docs/concepts/testing/"
    subtitle="`gifttest` runs your views headless inside `go test`: find a node by its text, click it, assert the result."
  >}}
  {{< hextra/feature-card
    title="Light and dark"
    icon="color-swatch"
    link="docs/concepts/theming/"
    subtitle="Semantic colors take their value from the theme in force. Switching the theme at run time redraws the whole window."
  >}}
  {{< hextra/feature-card
    title="Its own text"
    icon="pencil"
    link="docs/concepts/text-and-fonts/"
    subtitle="gift shapes and draws every glyph itself, so a label looks the same on macOS, Linux and a Raspberry Pi."
  >}}
  {{< hextra/feature-card
    title="Early stage"
    icon="exclamation"
    link="docs/getting-started/"
    subtitle="gift is under development and its API may still change. Read the getting started guide before you build on it."
  >}}
{{< /hextra/feature-grid >}}
