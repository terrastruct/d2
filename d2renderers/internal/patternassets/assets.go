// Package patternassets owns the canonical built-in texture definitions shared
// by D2's SVG and scene renderers.
package patternassets

import (
	_ "embed"
	"encoding/base64"
	"sync"

	"github.com/d2lang/d2/lib/compression"
)

//go:embed paper.txt.br
var paperBrotli []byte

//go:embed grain.png
var grainPNG []byte

//go:embed streak_path.txt
var streakPathData string

const grainSVGPrefix = `<pattern id="grain-%[1]s" x="0" y="0" width="300" height="300" patternUnits="userSpaceOnUse">
<g opacity="0.8" clip-path="url(#clip0_3943_51420-%[1]s)">
<path d="M300 0H0V300H300V0Z" fill="url(#pattern0-%[1]s)" fill-opacity="0.9"/>
</g>
<defs>
<pattern id="pattern0-%[1]s" patternContentUnits="objectBoundingBox" width="1" height="1">
<use xlink:href="#image0_3943_51420-%[1]s" transform="scale(0.00214592 0.00286533)"/>
</pattern>
<clipPath id="clip0_3943_51420-%[1]s">
<rect width="300" height="300" fill="white"/>
</clipPath>
<image id="image0_3943_51420-%[1]s" width="466" height="349" xlink:href="data:image/png;base64,`

const grainSVGSuffix = `"/>
</defs>
</pattern>
`

const streakSVGPrefix = `<pattern id="streaks-%s-%s" x="0" y="0" width="100" height="100" patternUnits="userSpaceOnUse">
    <path fill="%s" fill-rule="evenodd" clip-rule="evenodd" d="`

const streakSVGSuffix = `" />
</pattern>
`

var (
	paperSVG = sync.OnceValue(func() string {
		value, err := compression.DecompressBrotli(paperBrotli)
		if err != nil {
			panic("patternassets: decompress canonical paper texture: " + err.Error())
		}
		return value
	})
	grainSVG = sync.OnceValue(func() string {
		return grainSVGPrefix + base64.StdEncoding.EncodeToString(grainPNG) + grainSVGSuffix
	})
	streakSVG = sync.OnceValue(func() string {
		return streakSVGPrefix + streakPathData + streakSVGSuffix
	})
)

// PaperSVG returns the canonical paper pattern definition.
func PaperSVG() string { return paperSVG() }

// GrainPNG returns the canonical grain texture. Callers must treat the returned
// bytes as immutable.
func GrainPNG() []byte { return grainPNG }

// GrainSVG returns the SVG wrapper used by the SVG renderer.
func GrainSVG() string { return grainSVG() }

// StreakPathData returns the canonical sketch streak path data.
func StreakPathData() string { return streakPathData }

// StreaksSVG returns the SVG wrapper used by the SVG renderer.
func StreaksSVG() string { return streakSVG() }
