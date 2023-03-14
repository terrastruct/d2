package e2etests

import (
	_ "embed"
	"testing"
)

func testPatterns(t *testing.T) {
	tcs := []testCase{
		{
			name: "root-dots",
			script: `style.fill-pattern: dots
x -> y -> z
x -> abcd
x -> g
x -> z
`,
		},
		{
			name: "root-dots-with-fill",
			script: `style.fill-pattern: dots
style.fill: honeydew
x -> y -> z
x -> abcd
x -> g
x -> z
`,
		},
		{
			name: "shape",
			script: `x -> y -> z
x -> abcd
x -> g
x -> z
x.style.fill-pattern: dots
abcd.style.fill-pattern: dots
`,
		},
		{
			name: "3d",
			script: `x: {style.3d: true; style.fill-pattern: dots}
y: {shape: hexagon; style.3d: true; style.fill-pattern: dots}
`,
		},
		{
			name: "multiple",
			script: `
rectangle: {shape: "rectangle"; style.fill-pattern: dots; style.multiple: true}
square: {shape: "square"; style.fill-pattern: dots; style.multiple: true}
page: {shape: "page"; style.fill-pattern: dots; style.multiple: true}
parallelogram: {shape: "parallelogram"; style.fill-pattern: dots; style.multiple: true}
document: {shape: "document"; style.fill-pattern: dots; style.multiple: true}
cylinder: {shape: "cylinder"; style.fill-pattern: dots; style.multiple: true}
queue: {shape: "queue"; style.fill-pattern: dots; style.multiple: true}
package: {shape: "package"; style.fill-pattern: dots; style.multiple: true}
step: {shape: "step"; style.fill-pattern: dots; style.multiple: true}
callout: {shape: "callout"; style.fill-pattern: dots; style.multiple: true}
stored_data: {shape: "stored_data"; style.fill-pattern: dots; style.multiple: true}
person: {shape: "person"; style.fill-pattern: dots; style.multiple: true}
diamond: {shape: "diamond"; style.fill-pattern: dots; style.multiple: true}
oval: {shape: "oval"; style.fill-pattern: dots; style.multiple: true}
circle: {shape: "circle"; style.fill-pattern: dots; style.multiple: true}
hexagon: {shape: "hexagon"; style.fill-pattern: dots; style.multiple: true}
cloud: {shape: "cloud"; style.fill-pattern: dots; style.multiple: true}

rectangle -> square -> page
parallelogram -> document -> cylinder
queue -> package -> step
callout -> stored_data -> person
diamond -> oval -> circle
hexagon -> cloud
`,
		},
		{
			name: "all_shapes",
			script: `
rectangle: {shape: "rectangle"; style.fill-pattern: dots}
square: {shape: "square"; style.fill-pattern: dots}
page: {shape: "page"; style.fill-pattern: dots}
parallelogram: {shape: "parallelogram"; style.fill-pattern: dots}
document: {shape: "document"; style.fill-pattern: dots}
cylinder: {shape: "cylinder"; style.fill-pattern: dots}
queue: {shape: "queue"; style.fill-pattern: dots}
package: {shape: "package"; style.fill-pattern: dots}
step: {shape: "step"; style.fill-pattern: dots}
callout: {shape: "callout"; style.fill-pattern: dots}
stored_data: {shape: "stored_data"; style.fill-pattern: dots}
person: {shape: "person"; style.fill-pattern: dots}
diamond: {shape: "diamond"; style.fill-pattern: dots}
oval: {shape: "oval"; style.fill-pattern: dots}
circle: {shape: "circle"; style.fill-pattern: dots}
hexagon: {shape: "hexagon"; style.fill-pattern: dots}
cloud: {shape: "cloud"; style.fill-pattern: dots}

rectangle -> square -> page
parallelogram -> document -> cylinder
queue -> package -> step
callout -> stored_data -> person
diamond -> oval -> circle
hexagon -> cloud
`,
		},
		{
			name: "real",
			script: `
NETWORK: {
  style: {
	  stroke: black
    fill-pattern: dots
    double-border: true
    fill: "#E7E9EE"
    font: mono
  }
  CELL TOWER: {
		style: {
			stroke: black
			fill-pattern: dots
			fill: "#F5F6F9"
			font: mono
		}
		satellites: SATELLITES {
			shape: stored_data
			style: {
				font: mono
				fill: white
				stroke: black
				multiple: true
			}
		}

		transmitter: TRANSMITTER {
			style: {
				font: mono
				fill: white
				stroke: black
			}
		}

		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
  }
}
`,
		},
	}

	for i := range tcs {
		tcs[i].justDagre = true
	}

	runa(t, tcs)
}
