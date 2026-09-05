package d2svgimport

import (
	"math"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

type svgViewport struct {
	viewBox   d2scene.Box
	width     float64
	height    float64
	aspect    d2scene.AspectRatio
	transform d2scene.Matrix
}

func (i *svgImporter) viewport(root *svgElement) (svgViewport, error) {
	viewBox, hasViewBox, err := i.parseViewBoxFor(root, "root")
	if err != nil {
		return svgViewport{}, err
	}
	width, hasWidth, err := i.viewportLength(root, "width")
	if err != nil {
		return svgViewport{}, err
	}
	height, hasHeight, err := i.viewportLength(root, "height")
	if err != nil {
		return svgViewport{}, err
	}
	if !hasWidth || !hasHeight {
		if !hasViewBox {
			if !hasWidth {
				return svgViewport{}, i.errorf("root <svg> requires width or viewBox")
			}
			return svgViewport{}, i.errorf("root <svg> requires height or viewBox")
		}
		switch {
		case !hasWidth && !hasHeight:
			width, height = viewBox.Width, viewBox.Height
		case !hasWidth:
			width = height * viewBox.Width / viewBox.Height
		case !hasHeight:
			height = width * viewBox.Height / viewBox.Width
		}
	}
	if width <= 0 || height <= 0 {
		return svgViewport{}, i.errorf("root <svg> viewport width and height must be positive")
	}
	if !hasViewBox {
		viewBox = d2scene.Box{Width: width, Height: height}
	}
	scaleX := width / viewBox.Width
	scaleY := height / viewBox.Height
	if scaleX <= 0 || scaleY <= 0 || math.IsInf(scaleX, 0) || math.IsInf(scaleY, 0) {
		return svgViewport{}, i.errorf("root <svg> viewport scale is zero or non-finite")
	}
	aspect, err := i.parseAspectRatioFor(root.attrs["preserveAspectRatio"], "root")
	if err != nil {
		return svgViewport{}, err
	}
	transform, err := d2scene.AspectRatioMatrix(viewBox, d2scene.Box{Width: width, Height: height}, aspect)
	if err != nil {
		return svgViewport{}, i.errorf("root <svg> viewport transform is non-finite")
	}
	return svgViewport{viewBox: viewBox, width: width, height: height, aspect: aspect, transform: transform}, nil
}

func (i *svgImporter) parseViewBoxFor(element *svgElement, scope string) (d2scene.Box, bool, error) {
	raw, ok := element.attrs["viewBox"]
	if !ok {
		return d2scene.Box{}, false, nil
	}
	parts, err := splitSVGNumberList(i.ctx, raw, 4)
	if err != nil || len(parts) != 4 {
		if contextErr := i.ctx.Err(); contextErr != nil {
			return d2scene.Box{}, false, contextErr
		}
		return d2scene.Box{}, false, i.errorf("%s <svg> viewBox must contain four finite unitless numbers", scope)
	}
	values := make([]float64, 4)
	for index, part := range parts {
		values[index], err = parseSVGNumber(part)
		if err != nil {
			return d2scene.Box{}, false, i.errorf("%s <svg> viewBox must contain four finite unitless numbers", scope)
		}
	}
	if values[2] <= 0 || values[3] <= 0 {
		return d2scene.Box{}, false, i.errorf("%s <svg> viewBox width and height must be positive", scope)
	}
	return d2scene.Box{X: values[0], Y: values[1], Width: values[2], Height: values[3]}, true, nil
}

func (i *svgImporter) viewportLength(root *svgElement, name string) (float64, bool, error) {
	raw, ok := root.attrs[name]
	if !ok {
		return 0, false, nil
	}
	value, err := parseSVGLength(raw, true)
	if i.mathJaxRoot == root {
		if mathJaxValue, mathJaxErr := parseMathJaxEXLength(raw); mathJaxErr == nil {
			value, err = mathJaxValue, nil
		}
	}
	if err != nil {
		return 0, false, i.propertyError(root, name, err)
	}
	return value, true, nil
}

func (i *svgImporter) parseAspectRatioFor(raw, scope string) (d2scene.AspectRatio, error) {
	defaultAspect := d2scene.AspectRatio{Align: d2scene.AlignXMidYMid, Fit: d2scene.AspectMeet}
	if len(raw) > 128 {
		return d2scene.AspectRatio{}, i.errorf("%s <svg> preserveAspectRatio is too long", scope)
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return defaultAspect, nil
	}
	if fields[0] == "defer" {
		return d2scene.AspectRatio{}, i.errorf("%s <svg> preserveAspectRatio defer is unsupported", scope)
	}
	alignments := map[string]d2scene.AspectAlign{
		"none":     d2scene.AlignNone,
		"xMinYMin": d2scene.AlignXMinYMin,
		"xMidYMin": d2scene.AlignXMidYMin,
		"xMaxYMin": d2scene.AlignXMaxYMin,
		"xMinYMid": d2scene.AlignXMinYMid,
		"xMidYMid": d2scene.AlignXMidYMid,
		"xMaxYMid": d2scene.AlignXMaxYMid,
		"xMinYMax": d2scene.AlignXMinYMax,
		"xMidYMax": d2scene.AlignXMidYMax,
		"xMaxYMax": d2scene.AlignXMaxYMax,
	}
	align, ok := alignments[fields[0]]
	if !ok {
		return d2scene.AspectRatio{}, i.errorf("%s <svg> has unsupported preserveAspectRatio alignment", scope)
	}
	if len(fields) > 2 {
		return d2scene.AspectRatio{}, i.errorf("%s <svg> has malformed preserveAspectRatio", scope)
	}
	fit := d2scene.AspectMeet
	if len(fields) == 2 {
		switch fields[1] {
		case "meet":
			fit = d2scene.AspectMeet
		case "slice":
			fit = d2scene.AspectSlice
		default:
			return d2scene.AspectRatio{}, i.errorf("%s <svg> has unsupported preserveAspectRatio fit", scope)
		}
	}
	return d2scene.AspectRatio{Align: align, Fit: fit}, nil
}

func (i *svgImporter) nestedViewport(element *svgElement) (svgViewport, float64, float64, error) {
	viewBox, hasViewBox, err := i.parseViewBoxFor(element, "nested")
	if err != nil {
		return svgViewport{}, 0, 0, err
	}
	if !hasViewBox {
		return svgViewport{}, 0, 0, i.errorf("nested <svg> requires an explicit viewBox")
	}
	width, hasWidth, err := i.viewportLength(element, "width")
	if err != nil {
		return svgViewport{}, 0, 0, err
	}
	height, hasHeight, err := i.viewportLength(element, "height")
	if err != nil {
		return svgViewport{}, 0, 0, err
	}
	if !hasWidth || !hasHeight {
		return svgViewport{}, 0, 0, i.errorf("nested <svg> requires explicit width and height")
	}
	if width <= 0 || height <= 0 {
		return svgViewport{}, 0, 0, i.errorf("nested <svg> viewport width and height must be positive")
	}
	x, err := i.lengthAttribute(element, "x", 0, false)
	if err != nil {
		return svgViewport{}, 0, 0, err
	}
	y, err := i.lengthAttribute(element, "y", 0, false)
	if err != nil {
		return svgViewport{}, 0, 0, err
	}
	aspect, err := i.parseAspectRatioFor(element.attrs["preserveAspectRatio"], "nested")
	if err != nil {
		return svgViewport{}, 0, 0, err
	}
	transform, err := d2scene.AspectRatioMatrix(viewBox, d2scene.Box{Width: width, Height: height}, aspect)
	if err != nil {
		return svgViewport{}, 0, 0, i.errorf("nested <svg> viewport transform is non-finite")
	}
	return svgViewport{
		viewBox: viewBox, width: width, height: height, aspect: aspect, transform: transform,
	}, x, y, nil
}
