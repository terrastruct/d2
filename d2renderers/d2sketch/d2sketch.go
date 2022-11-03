// package d2sketch implements the sketch plugin for d2
// TODO
// - It currently uses Regex replacement, but this is not robust. It should switch to XML parsing.
// - Support different shapes. This isn't that much work, they're mostly path replacements.
// - Adjust parameters dynamically. For example, dashed lines look better less "sketchy".
// - Tune default set of parameters.
package d2sketch

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	v8 "rogchap.com/v8go"
)

//go:embed rough.js
var roughJS string

var rectRegex = regexp.MustCompile(`<rect class="shape" x="([0-9]+)" y="([0-9]+)" width="([0-9]+)" height="([0-9]+)" style="([^"]+)" />`)
var connectionRegex = regexp.MustCompile(`<path d="([A-Za-z0-9,\.\s]+)" class="connection" style="([^"]+)" ([^\/]*) />`)

func Sketch(in []byte) ([]byte, error) {
	d, err := newDrawer()
	if err != nil {
		return nil, err
	}

	svg := string(in)

	rects := rectRegex.FindAllStringSubmatch(svg, -1)
	for _, rect := range rects {
		x, err := strconv.Atoi(rect[1])
		if err != nil {
			return nil, err
		}
		y, err := strconv.Atoi(rect[2])
		if err != nil {
			return nil, err
		}
		width, err := strconv.Atoi(rect[3])
		if err != nil {
			return nil, err
		}
		height, err := strconv.Atoi(rect[4])
		if err != nil {
			return nil, err
		}
		sketchedRect, err := d.Rect(x, y, width, height, rect[5])
		if err != nil {
			return nil, err
		}
		svg = strings.Replace(svg, rect[0], sketchedRect, 1)
	}

	connections := connectionRegex.FindAllStringSubmatch(svg, -1)
	for _, c := range connections {
		sketchedC, err := d.Connection(c[1], c[2], c[3])
		if err != nil {
			return nil, err
		}
		svg = strings.Replace(svg, c[0], sketchedC, 1)
	}

	return []byte(svg), nil
}

type drawer struct {
	v8ctx *v8.Context
}

func newDrawer() (*drawer, error) {
	v8ctx := v8.NewContext()
	if _, err := v8ctx.RunScript(roughJS, "rough.js"); err != nil {
		return nil, err
	}
	js := `const root = {
ownerDocument: {
		createElementNS: (ns, tagName) => {
			const children = []
			const attrs = {}
			const style = {}
			return {
				style,
				tagName,
				attrs,
				setAttribute: (key, value) => (attrs[key] = value),
				appendChild: node => children.push(node),
				children,
			}
		},
	},
}
const rc = rough.svg(root, { seed: 1 });
let node;
`
	if _, err := v8ctx.RunScript(js, "setup.js"); err != nil {
		return nil, err
	}
	return &drawer{v8ctx: v8ctx}, nil
}

func (d *drawer) Connection(path, style, attrs string) (string, error) {
	// TODO get the stroke dash, adjust roughness
	roughness := 1.0
	js := fmt.Sprintf(`node = rc.path("%s", {roughness: %f, seed: 1});`, path, roughness)
	if _, err := d.v8ctx.RunScript(js, "draw.js"); err != nil {
		return "", err
	}
	paths, err := d.extractPaths()
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="connection" fill="none" d="%s" style="%s" %s/>`,
			p, style, attrs,
		)
	}
	return output, nil
}

func (d *drawer) Rect(x, y, width, height int, style string) (string, error) {
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fillWeight: 2.0,
		hachureGap: 16,
		fillStyle: "solid",
		bowing: 2,
		seed: 1,
	});`, width, height)
	if _, err := d.v8ctx.RunScript(js, "draw.js"); err != nil {
		return "", err
	}
	paths, err := d.extractPaths()
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			x, y, p, style,
		)
	}
	return output, nil
}

type attrs struct {
	D string `json:"d"`
}

type node struct {
	Attrs attrs `json:"attrs"`
}

func (d *drawer) extractPaths() ([]string, error) {
	val, err := d.v8ctx.RunScript("JSON.stringify(node.children)", "value.js")
	if err != nil {
		return nil, err
	}

	var nodes []node

	err = json.Unmarshal([]byte(val.String()), &nodes)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, n := range nodes {
		paths = append(paths, n.Attrs.D)
	}

	return paths, nil
}
