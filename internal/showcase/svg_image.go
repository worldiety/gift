package showcase

import (
	"sync"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/svg"
)

func init() {
	register(Demo{Name: "svg-image", Group: "basic", Title: "SVG Image",
		Summary: "A vector picture from an SVG document, sharp at any size, or tinted like an icon.",
		Width:   640, Height: 260}, svgImageDemo)
}

// weatherSVG is a picture with the common parts of SVG: gradients, a symbol
// used eight times turned, arcs, a clip path, a hole by the even-odd rule
// and a dashed line.
const weatherSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 160" width="200" height="160">
  <defs>
    <radialGradient id="sun" cx="0.4" cy="0.4" r="0.65">
      <stop offset="0" stop-color="#fff2a8"/>
      <stop offset="1" stop-color="#ff9f1a"/>
    </radialGradient>
    <linearGradient id="cloud" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#ffffff"/>
      <stop offset="1" stop-color="#cfd8e6"/>
    </linearGradient>
    <line id="ray" x1="0" y1="-44" x2="0" y2="-56" stroke="#ffb31a" stroke-width="6" stroke-linecap="round"/>
    <clipPath id="land"><rect x="0" y="118" width="200" height="42" rx="0"/></clipPath>
  </defs>
  <g transform="translate(72 64)">
    <use href="#ray"/><use href="#ray" transform="rotate(45)"/><use href="#ray" transform="rotate(90)"/>
    <use href="#ray" transform="rotate(135)"/><use href="#ray" transform="rotate(180)"/>
    <use href="#ray" transform="rotate(225)"/><use href="#ray" transform="rotate(270)"/>
    <use href="#ray" transform="rotate(315)"/>
    <circle r="34" fill="url(#sun)"/>
  </g>
  <g clip-path="url(#land)">
    <ellipse cx="46" cy="160" rx="92" ry="36" fill="#4cbb6c"/>
    <ellipse cx="164" cy="168" rx="84" ry="38" fill="#2f9e55"/>
  </g>
  <path d="M74 132 h84 a24 24 0 0 0 0-48 a34 34 0 0 0-64-8 a28 28 0 0 0-20 56 z"
        fill="url(#cloud)" stroke="#8d9bb0" stroke-width="2.5" stroke-linejoin="round"/>
  <path fill-rule="evenodd" fill="#5b6b82" d="M160 12h28v28h-28z M166 18h16v16h-16z"/>
  <polyline points="12,150 32,140 52,147 72,134" fill="none" stroke="#1e6fd9" stroke-width="3"
            stroke-linecap="round" stroke-dasharray="6 6"/>
</svg>`

// leafSVG is an icon in one colour, drawn in the colour of its surroundings.
const leafSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <path d="M5 21c0-9 6-15 15-16-1 9-7 15-15 16z M5 21 15 11" fill="none" stroke="currentColor"
        stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`

var demoSVG = sync.OnceValue(func() [2]*svg.Image {
	w, _ := svg.Parse([]byte(weatherSVG))
	l, _ := svg.Parse([]byte(leafSVG))
	return [2]*svg.Image{w, l}
})

func svgImageDemo(ctx *gift.Context) gift.View {
	d := demoSVG()
	weather, leaf := d[0], d[1]
	// snippet:begin
	return ui.HStack(
		svg.View(weather).Frame(300, 0).Label("Sun behind a cloud over hills"),
		svg.View(weather).Frame(100, 0),
		ui.VStack(
			svg.View(leaf).Frame(32, 32).Tint(ui.ColorAccent),
			svg.View(leaf).Frame(32, 32).Tint(ui.ColorLabel),
			svg.View(leaf).Frame(32, 32).Tint(ui.RGB(52, 199, 89)),
		).Gap(12),
	).Gap(24).Align(geom.Center)
	// snippet:end
}
