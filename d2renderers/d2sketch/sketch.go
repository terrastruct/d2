package d2sketch

import (
	"encoding/json"
	"fmt"

	_ "embed"

	"github.com/dop251/goja"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/svg"
	svg_style "oss.terrastruct.com/d2/lib/svg/style"
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

func Rect(r *Runner, shape d2target.Shape) (string, error) {
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	pathEl := svg_style.NewThemableElement("path")
	pathEl.Transform = fmt.Sprintf("translate(%d %d)", shape.Pos.X, shape.Pos.Y)
	pathEl.Fill, pathEl.Stroke = svg_style.ShapeTheme(shape)
	pathEl.Class = "shape"
	pathEl.Style = svg_style.ShapeStyle(shape)
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}
	output += fmt.Sprintf(
		`<rect class="sketch-overlay" transform="translate(%d %d)" width="%d" height="%d" />`,
		shape.Pos.X, shape.Pos.Y, shape.Width, shape.Height,
	)
	return output, nil
}

func Oval(r *Runner, shape d2target.Shape) (string, error) {
	js := fmt.Sprintf(`node = rc.ellipse(%d, %d, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width/2, shape.Height/2, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	pathEl := svg_style.NewThemableElement("path")
	pathEl.Transform = fmt.Sprintf("translate(%d %d)", shape.Pos.X, shape.Pos.Y)
	pathEl.Fill, pathEl.Stroke = svg_style.ShapeTheme(shape)
	pathEl.Class = "shape"
	pathEl.Style = svg_style.ShapeStyle(shape)
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
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
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, path, shape.StrokeWidth, baseRoughProps)
		sketchPaths, err := computeRoughPaths(r, js)
		if err != nil {
			return "", err
		}
		pathEl := svg_style.NewThemableElement("path")
		pathEl.Fill, pathEl.Stroke = svg_style.ShapeTheme(shape)
		pathEl.Class = "shape"
		pathEl.Style = svg_style.ShapeStyle(shape)
		for _, p := range sketchPaths {
			pathEl.D = p
			output += pathEl.Render()
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

func Connection(r *Runner, connection d2target.Connection, path, attrs string) (string, error) {
	roughness := 1.0
	js := fmt.Sprintf(`node = rc.path("%s", {roughness: %f, seed: 1});`, path, roughness)
	paths, err := computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	pathEl := svg_style.NewThemableElement("path")
	pathEl.Fill = color.None
	pathEl.Stroke = svg_style.ConnectionTheme(connection)
	pathEl.Class = "connection"
	pathEl.Style = svg_style.ConnectionStyle(connection)
	pathEl.Attributes = attrs
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}
	return output, nil
}

// TODO cleanup
func Table(r *Runner, shape d2target.Shape) (string, error) {
	output := ""
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	pathEl := svg_style.NewThemableElement("path")
	pathEl.Transform = fmt.Sprintf("translate(%d %d)", shape.Pos.X, shape.Pos.Y)
	pathEl.Fill, pathEl.Stroke = svg_style.ShapeTheme(shape)
	pathEl.Class = "shape"
	pathEl.Style = svg_style.ShapeStyle(shape)
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)
	rowHeight := box.Height / float64(1+len(shape.SQLTable.Columns))
	headerBox := geo.NewBox(box.TopLeft, box.Width, rowHeight)

	js = fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %f, {
		fill: "#000",
		%s
	});`, shape.Width, rowHeight, baseRoughProps)
	paths, err = computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	pathEl = svg_style.NewThemableElement("path")
	pathEl.Transform = fmt.Sprintf("translate(%d %d)", shape.Pos.X, shape.Pos.Y)
	pathEl.Fill = shape.Fill
	pathEl.Class = "class_header"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	if shape.Label != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			headerBox,
			20,
			float64(shape.LabelWidth),
			float64(shape.LabelHeight),
		)

		textEl := svg_style.NewThemableElement("text")
		textEl.X = tl.X
		textEl.Y = tl.Y + float64(shape.LabelHeight)*3/4
		textEl.Fill = shape.Stroke
		textEl.Class = "text"
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx",
			"start", 4+shape.FontSize,
		)
		textEl.Content = svg.EscapeText(shape.Label)
		output += textEl.Render()
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

		textEl := svg_style.NewThemableElement("text")
		textEl.X = nameTL.X
		textEl.Y = nameTL.Y + float64(shape.FontSize)*3/4
		textEl.Fill = shape.PrimaryAccentColor
		textEl.Class = "text"
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "start", float64(shape.FontSize))
		textEl.Content = svg.EscapeText(f.Name.Label)
		output += textEl.Render()

		textEl.X = nameTL.X + float64(longestNameWidth) + 2*d2target.NamePadding
		textEl.Fill = shape.NeutralAccentColor
		textEl.Content = svg.EscapeText(f.Type.Label)
		output += textEl.Render()

		textEl.X = constraintTR.X
		textEl.Y = constraintTR.Y + float64(shape.FontSize)*3/4
		textEl.Fill = shape.SecondaryAccentColor
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx;letter-spacing:2px", "end", float64(shape.FontSize))
		textEl.Content = f.ConstraintAbbr()
		output += textEl.Render()

		rowBox.TopLeft.Y += rowHeight

		js = fmt.Sprintf(`node = rc.line(%f, %f, %f, %f, {
		%s
	});`, rowBox.TopLeft.X, rowBox.TopLeft.Y, rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y, baseRoughProps)
		paths, err = computeRoughPaths(r, js)
		if err != nil {
			return "", err
		}
		pathEl := svg_style.NewThemableElement("path")
		pathEl.Fill = shape.Fill
		for _, p := range paths {
			pathEl.D = p
			output += pathEl.Render()
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
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	pathEl := svg_style.NewThemableElement("path")
	pathEl.Transform = fmt.Sprintf("translate(%d %d)", shape.Pos.X, shape.Pos.Y)
	pathEl.Fill, pathEl.Stroke = svg_style.ShapeTheme(shape)
	pathEl.Class = "shape"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)

	rowHeight := box.Height / float64(2+len(shape.Class.Fields)+len(shape.Class.Methods))
	headerBox := geo.NewBox(box.TopLeft, box.Width, 2*rowHeight)

	js = fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %f, {
		fill: "#000",
		%s
	});`, shape.Width, headerBox.Height, baseRoughProps)
	paths, err = computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	pathEl = svg_style.NewThemableElement("path")
	pathEl.Transform = fmt.Sprintf("translate(%d %d)", shape.Pos.X, shape.Pos.Y)
	pathEl.Fill = shape.Fill
	pathEl.Class = "class_header"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	output += fmt.Sprintf(
		`<rect class="sketch-overlay" transform="translate(%d %d)" width="%d" height="%f" />`,
		shape.Pos.X, shape.Pos.Y, shape.Width, headerBox.Height,
	)

	if shape.Label != "" {
		tl := label.InsideMiddleCenter.GetPointOnBox(
			headerBox,
			0,
			float64(shape.LabelWidth),
			float64(shape.LabelHeight),
		)

		textEl := svg_style.NewThemableElement("text")
		textEl.X = tl.X + float64(shape.LabelWidth)/2
		textEl.Y = tl.Y + float64(shape.LabelHeight)*3/4
		textEl.Fill = shape.Stroke
		textEl.Class = "text-mono"
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx",
			"middle",
			4+shape.FontSize,
		)
		textEl.Content = svg.EscapeText(shape.Label)
		output += textEl.Render()
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range shape.Fields {
		output += classRow(shape, rowBox, f.VisibilityToken(), f.Name, f.Type, float64(shape.FontSize))
		rowBox.TopLeft.Y += rowHeight
	}

	js = fmt.Sprintf(`node = rc.line(%f, %f, %f, %f, {
%s
	});`, rowBox.TopLeft.X, rowBox.TopLeft.Y, rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y, baseRoughProps)
	paths, err = computeRoughPaths(r, js)
	if err != nil {
		return "", err
	}
	pathEl = svg_style.NewThemableElement("path")
	pathEl.Fill = shape.Fill
	pathEl.Class = "class_header"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	for _, m := range shape.Methods {
		output += classRow(shape, rowBox, m.VisibilityToken(), m.Name, m.Return, float64(shape.FontSize))
		rowBox.TopLeft.Y += rowHeight
	}

	return output, nil
}

func classRow(shape d2target.Shape, box *geo.Box, prefix, nameText, typeText string, fontSize float64) string {
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

	textEl := svg_style.NewThemableElement("text")
	textEl.X = prefixTL.X
	textEl.Y = prefixTL.Y + fontSize*3/4
	textEl.Fill = shape.PrimaryAccentColor
	textEl.Class = "text-mono"
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "start", fontSize)
	textEl.Content = prefix
	output += textEl.Render()

	textEl.X = prefixTL.X + d2target.PrefixWidth
	textEl.Fill = shape.Fill
	textEl.Content = svg.EscapeText(nameText)
	output += textEl.Render()

	textEl.X = typeTR.X
	textEl.Y = typeTR.Y + fontSize*3/4
	textEl.Fill = shape.SecondaryAccentColor
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "end", fontSize)
	textEl.Content = svg.EscapeText(typeText)
	output += textEl.Render()

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
