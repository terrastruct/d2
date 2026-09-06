package d2svg

// Frozen font embedding from 0515f60e985ad829d652456e07831170583011a0.
// Keep trigger construction independent of the production lazy path.
import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2fonts"
)

func legacyEmbedFonts(buf *bytes.Buffer, diagramHash, source string, fontFamily *d2fonts.FontFamily, monoFontFamily *d2fonts.FontFamily, corpus string) {
	// Markdown generates text that may not exist literally in the D2 source,
	// such as list markers and decoded entities. Include those rendered runs in
	// every font subset, including multi-board animations where render-local
	// Markdown layout state is not available here.
	for _, match := range nativeMarkdownTextPattern.FindAllStringSubmatch(source, -1) {
		corpus += html.UnescapeString(match[1])
	}
	fmt.Fprint(buf, `<style type="text/css"><![CDATA[`)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`class="text"`,
			`class="text `,
			`class="md"`,
			`class="md `,
		},
		fmt.Sprintf(`
.%s .text {
	font-family: "%s-font-regular";
}
@font-face {
	font-family: %s-font-regular;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_REGULAR).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-semibold`,
		},
		fmt.Sprintf(`
.%s .text-semibold {
	font-family: "%s-font-semibold";
}
@font-face {
	font-family: %s-font-semibold;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_SEMIBOLD).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-underline`,
		},
		`
.text-underline {
	text-decoration: underline;
}`,
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-link`,
		},
		`
.text-link {
	fill: blue;
}

.text-link:visited {
	fill: purple;
}`,
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`animated-connection`,
		},
		`
@keyframes dashdraw {
	from {
		stroke-dashoffset: 0;
	}
}
`,
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`animated-shape`,
		},
		`
@keyframes shapeappear {
    0%, 100% { transform: translateY(0); filter: drop-shadow(0px 0px 0px rgba(0,0,0,0)); }
    50% { transform: translateY(-4px); filter: drop-shadow(0px 12.6px 25.2px rgba(50,50,93,0.25)) drop-shadow(0px 7.56px 15.12px rgba(0,0,0,0.1)); }
}
.animated-shape {
	animation: shapeappear 1s linear infinite;
}
`,
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`appendix-icon`,
		},
		`
.appendix-icon {
	filter: drop-shadow(0px 0px 32px rgba(31, 36, 58, 0.1));
}`,
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-bold`,
			`<b>`,
			`<strong>`,
		},
		fmt.Sprintf(`
.%s .text-bold {
	font-family: "%s-font-bold";
}
@font-face {
	font-family: %s-font-bold;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_BOLD).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-italic`,
			`<em>`,
			`<dfn>`,
		},
		fmt.Sprintf(`
.%s .text-italic {
	font-family: "%s-font-italic";
}
@font-face {
	font-family: %s-font-italic;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_ITALIC).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-mono`,
			`<pre>`,
			`<code>`,
			`<kbd>`,
			`<samp>`,
		},
		fmt.Sprintf(`
.%s .text-mono {
	font-family: "%s-font-mono";
}
@font-face {
	font-family: %s-font-mono;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			monoFontFamily.Font(0, d2fonts.FONT_STYLE_REGULAR).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-mono-semibold`,
		},
		fmt.Sprintf(`
.%s .text-mono-semibold {
	font-family: "%s-font-mono-semibold";
}
@font-face {
	font-family: %s-font-mono-semibold;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			monoFontFamily.Font(0, d2fonts.FONT_STYLE_SEMIBOLD).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-mono-bold`,
		},
		fmt.Sprintf(`
.%s .text-mono-bold {
	font-family: "%s-font-mono-bold";
}
@font-face {
	font-family: %s-font-mono-bold;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			monoFontFamily.Font(0, d2fonts.FONT_STYLE_BOLD).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`text-mono-italic`,
		},
		fmt.Sprintf(`
.%s .text-mono-italic {
	font-family: "%s-font-mono-italic";
}
@font-face {
	font-family: %s-font-mono-italic;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			monoFontFamily.Font(0, d2fonts.FONT_STYLE_ITALIC).GetEncodedSubset(corpus),
		),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-bright`,
		},
		fmt.Sprintf(`
.sketch-overlay-bright {
	fill: url(#streaks-bright-%s);
	mix-blend-mode: darken;
}`, diagramHash),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-normal`,
		},
		fmt.Sprintf(`
.sketch-overlay-normal {
	fill: url(#streaks-normal-%s);
	mix-blend-mode: color-burn;
}`, diagramHash),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-dark`,
		},
		fmt.Sprintf(`
.sketch-overlay-dark {
	fill: url(#streaks-dark-%s);
	mix-blend-mode: overlay;
}`, diagramHash),
	)

	legacyAppendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-darker`,
		},
		fmt.Sprintf(`
.sketch-overlay-darker {
	fill: url(#streaks-darker-%s);
	mix-blend-mode: lighten;
}`, diagramHash),
	)

	fmt.Fprint(buf, `]]></style>`)
}

func legacyAppendOnTrigger(buf *bytes.Buffer, source string, triggers []string, newContent string) {
	for _, trigger := range triggers {
		if strings.Contains(source, trigger) {
			fmt.Fprint(buf, newContent)
			break
		}
	}
}
