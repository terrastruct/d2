package d2sketch

import (
	"encoding/json"
	"fmt"
	"regexp"
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

var floatRE = regexp.MustCompile(`(\d+)\.(\d+)`)

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
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.CSSStyle(),
		)
	}
	output += fmt.Sprintf(
		`<rect class="sketch-overlay" transform="translate(%d %d)" width="%d" height="%d" />`,
		shape.Pos.X, shape.Pos.Y, shape.Width, shape.Height,
	)
	return output, nil
}

func DoubleRect(r *Runner, shape d2target.Shape) (string, error) {
	jsBigRect := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	pathsBigRect, err := computeRoughPathData(r, jsBigRect)
	if err != nil {
		return "", err
	}
	jsSmallRect := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width-d2target.INNER_BORDER_OFFSET*2, shape.Height-d2target.INNER_BORDER_OFFSET*2, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	pathsSmallRect, err := computeRoughPathData(r, jsSmallRect)
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range pathsBigRect {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.CSSStyle(),
		)
	}
	for _, p := range pathsSmallRect {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X+d2target.INNER_BORDER_OFFSET, shape.Pos.Y+d2target.INNER_BORDER_OFFSET, p, shape.CSSStyle(),
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
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.CSSStyle(),
		)
	}
	output += fmt.Sprintf(
		`<ellipse class="sketch-overlay" transform="translate(%d %d)" rx="%d" ry="%d" />`,
		shape.Pos.X+shape.Width/2, shape.Pos.Y+shape.Height/2, shape.Width/2, shape.Height/2,
	)
	return output, nil
}

func DoubleOval(r *Runner, shape d2target.Shape) (string, error) {
	jsBigCircle := fmt.Sprintf(`node = rc.ellipse(%d, %d, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width/2, shape.Height/2, shape.Width, shape.Height, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	jsSmallCircle := fmt.Sprintf(`node = rc.ellipse(%d, %d, %d, %d, {
		fill: "%s",
		stroke: "%s",
		strokeWidth: %d,
		%s
	});`, shape.Width/2, shape.Height/2, shape.Width-d2target.INNER_BORDER_OFFSET*2, shape.Height-d2target.INNER_BORDER_OFFSET*2, shape.Fill, shape.Stroke, shape.StrokeWidth, baseRoughProps)
	pathsBigCircle, err := computeRoughPathData(r, jsBigCircle)
	if err != nil {
		return "", err
	}
	pathsSmallCircle, err := computeRoughPathData(r, jsSmallCircle)
	if err != nil {
		return "", err
	}
	output := ""
	for _, p := range pathsBigCircle {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.CSSStyle(),
		)
	}
	for _, p := range pathsSmallCircle {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.CSSStyle(),
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
		sketchPaths, err := computeRoughPathData(r, js)
		if err != nil {
			return "", err
		}
		for _, p := range sketchPaths {
			output += fmt.Sprintf(
				`<path class="shape" d="%s" style="%s" />`,
				p, shape.CSSStyle(),
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

func Connection(r *Runner, connection d2target.Connection, path, attrs string) (string, error) {
	roughness := 1.0
	js := fmt.Sprintf(`node = rc.path("%s", {roughness: %f, seed: 1});`, path, roughness)
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	animatedClass := ""
	if connection.Animated {
		animatedClass = " animated-connection"
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="connection%s" fill="none" d="%s" style="%s" %s/>`,
			animatedClass, p, connection.CSSStyle(), attrs,
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
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.CSSStyle(),
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
	paths, err = computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="class_header" transform="translate(%d %d)" d="%s" style="fill:%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.Fill,
		)
	}

	if shape.Label != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			headerBox,
			20,
			float64(shape.LabelWidth),
			float64(shape.LabelHeight),
		)

		output += fmt.Sprintf(`<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			"text",
			tl.X,
			tl.Y+float64(shape.LabelHeight)*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s",
				"start",
				4+shape.FontSize,
				shape.Stroke,
			),
			svg.EscapeText(shape.Label),
		)
	}

	var longestNameWidth int
	for _, f := range shape.Columns {
		longestNameWidth = go2.Max(longestNameWidth, f.Label.LabelWidth)
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

		output += strings.Join([]string{
			fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
				nameTL.X,
				nameTL.Y+float64(shape.FontSize)*3/4,
				fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", float64(shape.FontSize), shape.PrimaryAccentColor),
				svg.EscapeText(f.Label.Label),
			),
			fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
				nameTL.X+float64(longestNameWidth)+2*d2target.NamePadding,
				nameTL.Y+float64(shape.FontSize)*3/4,
				fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", float64(shape.FontSize), shape.NeutralAccentColor),
				svg.EscapeText(f.Type.Label),
			),
			fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
				constraintTR.X,
				constraintTR.Y+float64(shape.FontSize)*3/4,
				fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s;letter-spacing:2px;", "end", float64(shape.FontSize), shape.SecondaryAccentColor),
				f.ConstraintAbbr(),
			),
		}, "\n")

		rowBox.TopLeft.Y += rowHeight

		js = fmt.Sprintf(`node = rc.line(%f, %f, %f, %f, {
		%s
	});`, rowBox.TopLeft.X, rowBox.TopLeft.Y, rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y, baseRoughProps)
		paths, err = computeRoughPathData(r, js)
		if err != nil {
			return "", err
		}
		for _, p := range paths {
			output += fmt.Sprintf(
				`<path d="%s" style="fill:%s" />`,
				p, shape.Fill,
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
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="shape" transform="translate(%d %d)" d="%s" style="%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.CSSStyle(),
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
	paths, err = computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="class_header" transform="translate(%d %d)" d="%s" style="fill:%s" />`,
			shape.Pos.X, shape.Pos.Y, p, shape.Fill,
		)
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

		output += fmt.Sprintf(`<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			"text-mono",
			tl.X+float64(shape.LabelWidth)/2,
			tl.Y+float64(shape.LabelHeight)*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s",
				"middle",
				4+shape.FontSize,
				shape.Stroke,
			),
			svg.EscapeText(shape.Label),
		)
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
	paths, err = computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		output += fmt.Sprintf(
			`<path class="class_header" d="%s" style="fill:%s" />`,
			p, shape.Fill,
		)
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

	output += strings.Join([]string{
		fmt.Sprintf(`<text class="text-mono" x="%f" y="%f" style="%s">%s</text>`,
			prefixTL.X,
			prefixTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, shape.PrimaryAccentColor),
			prefix,
		),

		fmt.Sprintf(`<text class="text-mono" x="%f" y="%f" style="%s">%s</text>`,
			prefixTL.X+d2target.PrefixWidth,
			prefixTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, shape.Fill),
			svg.EscapeText(nameText),
		),

		fmt.Sprintf(`<text class="text-mono" x="%f" y="%f" style="%s">%s</text>`,
			typeTR.X,
			typeTR.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s;", "end", fontSize, shape.SecondaryAccentColor),
			svg.EscapeText(typeText),
		),
	}, "\n")
	return output
}

func computeRoughPathData(r *Runner, js string) ([]string, error) {
	if _, err := r.run(js); err != nil {
		return nil, err
	}
	roughPaths, err := extractRoughPaths(r)
	if err != nil {
		return nil, err
	}
	return extractPathData(roughPaths)
}

func computeRoughPaths(r *Runner, js string) ([]roughPath, error) {
	if _, err := r.run(js); err != nil {
		return nil, err
	}
	return extractRoughPaths(r)
}

type attrs struct {
	D string `json:"d"`
}

type style struct {
	Stroke      string `json:"stroke,omitempty"`
	StrokeWidth string `json:"strokeWidth,omitempty"`
	Fill        string `json:"fill,omitempty"`
}

type roughPath struct {
	Attrs attrs `json:"attrs"`
	Style style `json:"style"`
}

func (rp roughPath) StyleCSS() string {
	style := ""
	if rp.Style.Fill != "" {
		style += fmt.Sprintf("fill:%s;", rp.Style.Fill)
	}
	if rp.Style.Stroke != "" {
		style += fmt.Sprintf("stroke:%s;", rp.Style.Stroke)
	}
	if rp.Style.StrokeWidth != "" {
		style += fmt.Sprintf("stroke-width:%s;", rp.Style.StrokeWidth)
	}
	return style
}

func extractRoughPaths(r *Runner) ([]roughPath, error) {
	val, err := r.run("JSON.stringify(node.children, null, '  ')")
	if err != nil {
		return nil, err
	}

	var roughPaths []roughPath
	err = json.Unmarshal([]byte(val.String()), &roughPaths)
	if err != nil {
		return nil, err
	}

	// we want to have a fixed precision to the decimals in the path data
	for i := range roughPaths {
		// truncate all floats in path to only use up to 6 decimal places
		roughPaths[i].Attrs.D = floatRE.ReplaceAllStringFunc(roughPaths[i].Attrs.D, func(floatStr string) string {
			i := strings.Index(floatStr, ".")
			decimalLen := len(floatStr) - i - 1
			end := i + go2.Min(decimalLen, 6)
			return floatStr[:end+1]
		})
	}

	return roughPaths, nil
}

func extractPathData(roughPaths []roughPath) ([]string, error) {
	var paths []string
	for _, rp := range roughPaths {
		paths = append(paths, rp.Attrs.D)
	}
	return paths, nil
}

func ArrowheadJS(r *Runner, arrowhead d2target.Arrowhead, stroke string, strokeWidth int) (arrowJS, extraJS string) {
	// Note: selected each seed that looks the good for consistent renders
	switch arrowhead {
	case d2target.ArrowArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.linearPath(%s, { strokeWidth: %d, stroke: "%s", seed: 3 })`,
			`[[-10, -4], [0, 0], [-10, 4]]`,
			strokeWidth,
			stroke,
		)
	case d2target.TriangleArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.polygon(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", seed: 2 })`,
			`[[-10, -4], [0, 0], [-10, 4]]`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.DiamondArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.polygon(%s, { strokeWidth: %d, stroke: "%s", fill: "white", fillStyle: "solid", seed: 1 })`,
			`[[-20, 0], [-10, 5], [0, 0], [-10, -5], [-20, 0]]`,
			strokeWidth,
			stroke,
		)
	case d2target.FilledDiamondArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.polygon(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "zigzag", fillWeight: 4, seed: 1 })`,
			`[[-20, 0], [-10, 5], [0, 0], [-10, -5], [-20, 0]]`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.CfManyRequired:
		arrowJS = fmt.Sprintf(
			// TODO why does fillStyle: "zigzag" error with path
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 2 })`,
			`"M-15,-10 -15,10 M0,10 -15,0 M0,-10 -15,0"`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.CfMany:
		arrowJS = fmt.Sprintf(
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 8 })`,
			`"M0,10 -15,0 M0,-10 -15,0"`,
			strokeWidth,
			stroke,
			stroke,
		)
		extraJS = fmt.Sprintf(
			`node = rc.circle(-20, 0, 8, { strokeWidth: %d, stroke: "%s", fill: "white", fillStyle: "solid", fillWeight: 1, seed: 4 })`,
			strokeWidth,
			stroke,
		)
	case d2target.CfOneRequired:
		arrowJS = fmt.Sprintf(
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 2 })`,
			`"M-15,-10 -15,10 M-10,-10 -10,10"`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.CfOne:
		arrowJS = fmt.Sprintf(
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 3 })`,
			`"M-10,-10 -10,10"`,
			strokeWidth,
			stroke,
			stroke,
		)
		extraJS = fmt.Sprintf(
			`node = rc.circle(-20, 0, 8, { strokeWidth: %d, stroke: "%s", fill: "white", fillStyle: "solid", fillWeight: 1, seed: 5 })`,
			strokeWidth,
			stroke,
		)
	}
	return
}

func Arrowheads(r *Runner, connection d2target.Connection, srcAdj, dstAdj *geo.Point) (string, error) {
	arrowPaths := []string{}

	if connection.SrcArrow != d2target.NoArrowhead {
		arrowJS, extraJS := ArrowheadJS(r, connection.SrcArrow, connection.Stroke, connection.StrokeWidth)
		if arrowJS == "" {
			return "", nil
		}

		startingSegment := geo.NewSegment(connection.Route[0], connection.Route[1])
		startingVector := startingSegment.ToVector().Reverse()
		angle := startingVector.Degrees()

		transform := fmt.Sprintf(`transform="translate(%f %f) rotate(%v)"`,
			startingSegment.Start.X+srcAdj.X, startingSegment.Start.Y+srcAdj.Y, angle,
		)

		roughPaths, err := computeRoughPaths(r, arrowJS)
		if err != nil {
			return "", err
		}
		if extraJS != "" {
			extraPaths, err := computeRoughPaths(r, extraJS)
			if err != nil {
				return "", err
			}
			roughPaths = append(roughPaths, extraPaths...)
		}

		for _, rp := range roughPaths {
			pathStr := fmt.Sprintf(`<path class="connection" d="%s" style="%s" %s/>`,
				rp.Attrs.D,
				rp.StyleCSS(),
				transform,
			)
			arrowPaths = append(arrowPaths, pathStr)
		}
	}

	if connection.DstArrow != d2target.NoArrowhead {
		arrowJS, extraJS := ArrowheadJS(r, connection.DstArrow, connection.Stroke, connection.StrokeWidth)
		if arrowJS == "" {
			return "", nil
		}

		length := len(connection.Route)
		endingSegment := geo.NewSegment(connection.Route[length-2], connection.Route[length-1])
		endingVector := endingSegment.ToVector()
		angle := endingVector.Degrees()

		transform := fmt.Sprintf(`transform="translate(%f %f) rotate(%v)"`,
			endingSegment.End.X+dstAdj.X, endingSegment.End.Y+dstAdj.Y, angle,
		)

		roughPaths, err := computeRoughPaths(r, arrowJS)
		if err != nil {
			return "", err
		}
		if extraJS != "" {
			extraPaths, err := computeRoughPaths(r, extraJS)
			if err != nil {
				return "", err
			}
			roughPaths = append(roughPaths, extraPaths...)
		}

		for _, rp := range roughPaths {
			pathStr := fmt.Sprintf(`<path class="connection" d="%s" style="%s" %s/>`,
				rp.Attrs.D,
				rp.StyleCSS(),
				transform,
			)
			arrowPaths = append(arrowPaths, pathStr)
		}
	}

	return strings.Join(arrowPaths, " "), nil
}
