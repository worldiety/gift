package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "disclosure-group", Group: "lists", Title: "Disclosure Group",
		Summary: "A row that shows or hides content below it, and an accordion of them with one open at a time.",
		Width:   740, Height: 370}, disclosureGroupDemo)
}

func disclosureGroupDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	wifi := ctx.State("wifi", true)
	advanced := ctx.State("advanced", true)
	faq := ctx.State("faq", 0)

	// A group among the rows of a settings list. Its rows are a list of
	// their own, indented to the title; the state says it is open.
	network := ui.Card(
		ui.List(
			ui.Row("Wi-Fi").Icon(outline.Globe).Accessory(ui.Toggle(ctx.Read(wifi), wifi.Set)),
			ui.DisclosureGroup("Advanced",
				ui.List(
					ui.Row("Proxy").Value("Automatic"),
					ui.Row("DNS").Value("1.1.1.1"),
					ui.Row("MTU").Value("1500"),
				).SeparatorInsets(ui.RowPadding, 0),
			).Icon(outline.Cog).Value("3 settings").
				ContentPadding(geom.Insets{Left: 22 + 12}).
				Expanded(ctx.Read(advanced), advanced.Set),
			ui.Row("Server").Icon(outline.Server).Value("kiosk.local"),
		).SeparatorInsets(ui.RowPadding+22+12, 0).Frame(340, geom.Unbounded()),
	).Padding(0).Header("Network")

	// An accordion: opening one question closes the other.
	answer := func(s string) gift.View {
		return ui.Text(s).FontSize(14).Foreground(ui.ColorSecondaryLabel)
	}
	questions := ui.Card(
		ui.Accordion(
			ui.DisclosureGroup("How long does shipping take?",
				answer("Two to three working days within Germany, a week to the rest of Europe."),
			).Icon(outline.Truck),
			ui.DisclosureGroup("Can I return an order?",
				answer("Within 30 days, in its original packaging. The label is in the parcel."),
			).Icon(outline.Refresh),
			ui.DisclosureGroup("How can I pay?",
				answer("By card, with Apple Pay or Google Pay, or on account for business customers."),
			).Icon(outline.CreditCard),
		).Open(ctx.Read(faq), faq.Set).SeparatorInsets(ui.RowPadding+22+12, 0).Frame(340, geom.Unbounded()),
	).Padding(0).Header("Questions")

	return ui.HStack(network, questions).Gap(24).Align(geom.Alignment{X: 0, Y: 0})
	// snippet:end
}
