package d2sketch

import (
	"encoding/json"
	"fmt"

	_ "embed"

	"github.com/dop251/goja"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/svg"
)

//go:embed fillpattern.svg
var fillPattern string

//go:embed rough.js
var roughJS string

//go:embed setup.js
var setupJS string

type Runner goja.Runtime

var baseRoughProps = `fillWeight: 2.0,
hachureGap: 16,
fillStyle: "solid",
bowing: 2,
seed: 1,`

func (r *Runner) run(js string) (goja.Value, error) {
	vm := (*goja.Runtime)(r)
	return vm.RunString(js)
}

func InitSketchVM() (*Runner, error) {
	vm := goja.New()
	if _, err := vm.RunString(roughJS); err != nil {
		return nil, err
	}
	if _, err := vm.RunString(setupJS); err != nil {
		return nil, err
	}
	r := Runner(*vm)
	return &r, nil
}

// DefineFillPattern adds a reusable pattern that is overlayed on shapes with
// fill. This gives it a subtle streaky effect that subtly looks hand-drawn but
// not distractingly so.
func DefineFillPattern() string {
	return fmt.Sprintf(`<defs>
  <pattern id="streaks"
           x="0" y="0" width="100" height="100"
           patternUnits="userSpaceOnUse" >
      %s
  </pattern>
</defs>`, fillPattern)
}

func shapeStyle(shape d2target.Shape) string {
	out := ""

	out += fmt.Sprintf(`fill:%s;`, shape.Fill)
	out += fmt.Sprintf(`stroke:%s;`, shape.Stroke)
	out += fmt.Sprintf(`opacity:%f;`, shape.Opacity)
	out += fmt.Sprintf(`stroke-width:%d;`, shape.StrokeWidth)
	if shape.StrokeDash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(shape.StrokeWidth), shape.StrokeDash)
		out += fmt.Sprintf(`stroke-dasharray:%f,%f;`, dashSize, gapSize)
	}

	return out
}

func Rect(r *Runner, shape d2target.Shape) (string, error) {
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	if _, err := r.run(js); err != nil {
		return "", err
	}
	paths, err := extractPaths(r)
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shapeStyle(shape),
		)
	}
	output += fmt.Sprintf(
		`<rect class="sketch-overlay" transform="translate(%d %d)" width="%d" height="%d" />`,
		shape.Pos.X, shape.Pos.Y, shape.Width, shape.Height,
	)
	return output, nil
}

func Oval(r *Runner, shape d2target.Shape) (string, error) {
	js := fmt.Sprintf(`node = rc.ellipse(%d, %d, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width/2, shape.Height/2, shape.Width, shape.Height, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	if _, err := r.run(js); err != nil {
		return "", err
	}
	paths, err := extractPaths(r)
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shapeStyle(shape),
		)
	}
	output += fmt.Sprintf(
		`<ellipse class="sketch-overlay" transform="translate(%d %d)" rx="%d" ry="%d" />`,
		shape.Pos.X+shape.Width/2, shape.Pos.Y+shape.Height/2, shape.Width/2, shape.Height/2,
	)
	return output, nil
}

// TODO need to personalize this per shape like we do in Terrastruct app
func Paths(r *Runner, shape d2target.Shape, paths []string) (string, error) {
	output := ""
	for _, path := range paths {
		js := fmt.Sprintf(`node = rc.path("%s", {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, path, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
		if _, err := r.run(js); err != nil {
			return "", err
		}
		sketchPaths, err := extractPaths(r)
		if err != nil {
			return "", err
		}
		for _, p := range sketchPaths {
			output += fmt.Sprintf(
				`<path class="shape" d="%s" style="%s" />`,
				p, shapeStyle(shape),
			)
		}
		for _, p := range sketchPaths {
			output += fmt.Sprintf(
				`<path class="sketch-overlay" d="%s" />`,
				p,
			)
		}
	}
	return output, nil
}

func connectionStyle(connection d2target.Connection) string {
	out := ""

	out += fmt.Sprintf(`stroke:%s;`, connection.Stroke)
	out += fmt.Sprintf(`opacity:%f;`, connection.Opacity)
	out += fmt.Sprintf(`stroke-width:%d;`, connection.StrokeWidth)
	if connection.StrokeDash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(connection.StrokeWidth), connection.StrokeDash)
		out += fmt.Sprintf(`stroke-dasharray:%f,%f;`, dashSize, gapSize)
	}

	return out
}

func Connection(r *Runner, connection d2target.Connection, path, attrs string) (string, error) {
	roughness := 1.0
	js := fmt.Sprintf(`node = rc.path("%s", {roughness: %f, seed: 1});`, path, roughness)
	if _, err := r.run(js); err != nil {
		return "", err
	}
	paths, err := extractPaths(r)
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="connection" fill="none" d="%s" style="%s" %s/>`,
			p, connectionStyle(connection), attrs,
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

func extractPaths(r *Runner) ([]string, error) {
	val, err := r.run("JSON.stringify(node.children)")
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
