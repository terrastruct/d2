package d2sketch

import (
	"encoding/json"
	"fmt"
	"strings"

	_ "embed"

	"github.com/dop251/goja"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
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
	paths, err := computeRoughPaths(r, js)
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
	paths, err := computeRoughPaths(r, js)
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
		sketchPaths, err := computeRoughPaths(r, js)
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
	paths, err := computeRoughPaths(r, js)
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

// TODO cleanup
func Table(r *Runner, shape d2target.Shape) (string, error) {
	output := ""
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shapeStyle(shape),
		)
	}

	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)
	rowHeight := box.Height / float64(1+len(shape.SQLTable.Columns))
	headerBox := geo.NewBox(box.TopLeft, box.Width, rowHeight)

	js = fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %f, {
		fill: "%s",
		%s
	});`, shape.Width, rowHeight, shape.Fill, baseRoughProps)
	paths, err = computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		// TODO header fill
		output += fmt.Sprintf(
			`<path class="class_header" transform="translate(%d %d)" d="%s" style="fill:%s" />`,
			shape.Pos.X, shape.Pos.Y, p, "#0a0f25",
		)
	}

	if shape.Label != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			headerBox,
			20,
			float64(shape.LabelWidth),
			float64(shape.LabelHeight),
		)

		// TODO header font color
		output += fmt.Sprintf(`<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			"text",
			tl.X,
			tl.Y+float64(shape.LabelHeight)*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s",
				"start",
				4+shape.FontSize,
				"white",
			),
			svg.EscapeText(shape.Label),
		)
	}

	var longestNameWidth int
	for _, f := range shape.Columns {
		longestNameWidth = go2.Max(longestNameWidth, f.Name.LabelWidth)
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range shape.Columns {
		nameTL := label.InsideMiddleLeft.GetPointOnBox(
			rowBox,
			d2target.NamePadding,
			rowBox.Width,
			float64(shape.FontSize),
		)
		constraintTR := label.InsideMiddleRight.GetPointOnBox(
			rowBox,
			d2target.TypePadding,
			0,
			float64(shape.FontSize),
		)

		// TODO theme based
		primaryColor := "rgb(13, 50, 178)"
		accentColor := "rgb(74, 111, 243)"
		neutralColor := "rgb(103, 108, 126)"

		output += strings.Join([]string{
			fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
				nameTL.X,
				nameTL.Y+float64(shape.FontSize)*3/4,
				fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", float64(shape.FontSize), primaryColor),
				svg.EscapeText(f.Name.Label),
			),

			// TODO light font
			fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
				nameTL.X+float64(longestNameWidth)+2*d2target.NamePadding,
				nameTL.Y+float64(shape.FontSize)*3/4,
				fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", float64(shape.FontSize), neutralColor),
				svg.EscapeText(f.Type.Label),
			),

			fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
				constraintTR.X,
				constraintTR.Y+float64(shape.FontSize)*3/4,
				fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s;letter-spacing:2px;", "end", float64(shape.FontSize), accentColor),
				f.ConstraintAbbr(),
			),
		}, "\n")

		rowBox.TopLeft.Y += rowHeight

		js = fmt.Sprintf(`node = rc.line(%f, %f, %f, %f, {
		%s
	});`, rowBox.TopLeft.X, rowBox.TopLeft.Y, rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y, baseRoughProps)
		paths, err = computeRoughPaths(r, js)
		if err != nil {
			return "", err
		}
		for _, p := range paths {
			output += fmt.Sprintf(
				`<path class="class_header" d="%s" style="fill:%s" />`,
				p, "#0a0f25",
			)
		}
	}
	output += fmt.Sprintf(
		`<rect class="sketch-overlay" transform="translate(%d %d)" width="%d" height="%d" />`,
		shape.Pos.X, shape.Pos.Y, shape.Width, shape.Height,
	)
	return output, nil
}

func Class(r *Runner, shape d2target.Shape) (string, error) {
	output := ""
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shapeStyle(shape),
		)
	}

	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)

	rowHeight := box.Height / float64(2+len(shape.Class.Fields)+len(shape.Class.Methods))
	headerBox := geo.NewBox(box.TopLeft, box.Width, 2*rowHeight)

	js = fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %f, {
		fill: "%s",
		%s
	});`, shape.Width, headerBox.Height, shape.Fill, baseRoughProps)
	paths, err = computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		// TODO header fill
		output += fmt.Sprintf(
			`<path class="class_header" transform="translate(%d %d)" d="%s" style="fill:%s" />`,
			shape.Pos.X, shape.Pos.Y, p, "#0a0f25",
		)
	}

	output += fmt.Sprintf(
		`<rect class="sketch-overlay" transform="translate(%d %d)" width="%d" height="%f" />`,
		shape.Pos.X, shape.Pos.Y, shape.Width, headerBox.Height,
	)

	if shape.Label != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			headerBox,
			0,
			float64(shape.LabelWidth),
			float64(shape.LabelHeight),
		)

		// TODO header font color
		output += fmt.Sprintf(`<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			"text",
			tl.X+float64(shape.LabelWidth)/2,
			tl.Y+float64(shape.LabelHeight)*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s",
				"middle",
				4+shape.FontSize,
				"white",
			),
			svg.EscapeText(shape.Label),
		)
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range shape.Fields {
		output += classRow(rowBox, f.VisibilityToken(), f.Name, f.Type, float64(shape.FontSize))
		rowBox.TopLeft.Y += rowHeight
	}

	js = fmt.Sprintf(`node = rc.line(%f, %f, %f, %f, {
%s
	});`, rowBox.TopLeft.X, rowBox.TopLeft.Y, rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y, baseRoughProps)
	paths, err = computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="class_header" d="%s" style="fill:%s" />`,
			p, "#0a0f25",
		)
	}

	for _, m := range shape.Methods {
		output += classRow(rowBox, m.VisibilityToken(), m.Name, m.Return, float64(shape.FontSize))
		rowBox.TopLeft.Y += rowHeight
	}

	return output, nil
}

func classRow(box *geo.Box, prefix, nameText, typeText string, fontSize float64) string {
	output := ""
	prefixTL := label.InsideMiddleLeft.GetPointOnBox(
		box,
		d2target.PrefixPadding,
		box.Width,
		fontSize,
	)
	typeTR := label.InsideMiddleRight.GetPointOnBox(
		box,
		d2target.TypePadding,
		0,
		fontSize,
	)

	// TODO theme based
	accentColor := "rgb(13, 50, 178)"

	output += strings.Join([]string{
		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			prefixTL.X,
			prefixTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, accentColor),
			prefix,
		),

		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			prefixTL.X+d2target.PrefixWidth,
			prefixTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, "black"),
			svg.EscapeText(nameText),
		),

		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			typeTR.X,
			typeTR.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s;", "end", fontSize, accentColor),
			svg.EscapeText(typeText),
		),
	}, "\n")
	return output
}

func computeRoughPaths(r *Runner, js string) ([]string, error) {
	if _, err := r.run(js); err != nil {
		return nil, err
	}
	return extractPaths(r)
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
