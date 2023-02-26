package e2etests

import (
	_ "embed"
	"testing"
)

// testRoot tests things that affect the root, like background color
func testRoot(t *testing.T) {
	tcs := []testCase{
		{
			name: "fill",
			script: `we all live; in a LightSteelBlue; submarine
style.fill: LightSteelBlue
`,
		},
		{
			name: "stroke-no-width",
			script: `we all live; in a LightSteelBlue; submarine
style.fill: LightSteelBlue
style.stroke: "#191970"
`,
		},
		{
			name: "stroke-width",
			script: `we all live; in a LightSteelBlue; submarine
style.fill: LightSteelBlue
style.stroke: "#191970"
style.stroke-width: 5
`,
		},
		{
			name: "even-stroke-width",
			script: `we all live; in a LightSteelBlue; submarine
style.fill: LightSteelBlue
style.stroke: "#191970"
style.stroke-width: 6
`,
		},
		{
			name: "border-radius",
			script: `we all live; in a LightSteelBlue; submarine
style.fill: LightSteelBlue
style.stroke: "#191970"
style.stroke-width: 5
style.border-radius: 10
`,
		},
		{
			name: "stroke-dash",
			script: `we all live; in a LightSteelBlue; submarine
style.fill: LightSteelBlue
style.stroke: "#191970"
style.stroke-width: 3
style.stroke-dash: 4
`,
		},
		{
			name: "double-border",
			script: `we all live; in a LightSteelBlue; submarine
style.fill: LightSteelBlue
style.stroke: "#191970"
style.stroke-width: 3
style.double-border: true
`,
		},
	}

	runa(t, tcs)
}
