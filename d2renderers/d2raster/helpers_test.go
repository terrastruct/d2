package d2raster

import (
	"context"
	"image/color"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func prepare(ctx context.Context, document *d2scene.Document, options FrameOptions) (*preparedDocument, error) {
	return prepareWithSession(ctx, document, options, nil)
}

func prepareAnimatedPaint(paint d2scene.Paint, animatedColor *color.NRGBA, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (*preparedPaint, error) {
	return prepareAnimatedPaintWithPattern(paint, animatedColor, objectBounds, objectToDevice, nil)
}

func prepareStroke(stroke *d2scene.Stroke) (*preparedStroke, error) {
	return prepareAnimatedStroke(stroke, nil, nil, d2scene.Box{Width: 1, Height: 1}, d2scene.Identity())
}

func prepareAnimatedStroke(stroke *d2scene.Stroke, animatedColor *color.NRGBA, animatedDashOffset *float64, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (*preparedStroke, error) {
	return prepareAnimatedStrokeWithPaint(stroke, animatedColor, animatedDashOffset, objectBounds, objectToDevice, prepareAnimatedPaint)
}
