package d2scenebuild

import (
	"crypto/sha256"
	"fmt"
	"image/color"
	"unicode/utf8"

	"github.com/d2lang/d2/d2renderers/d2latex"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

const maxLatexInputBytes = 4 << 10

type latexAssetKey struct {
	formula string
	color   color.NRGBA
}

type latexAsset struct {
	id     d2scene.AssetID
	width  float64
	height float64
}

func (b *builder) resolveLatexAsset(object, formula, rawColor string) (latexAsset, error) {
	if err := b.ctx.Err(); err != nil {
		return latexAsset{}, err
	}
	if err := b.validateLatexInput(object, formula); err != nil {
		return latexAsset{}, err
	}
	if err := b.validateLatexImportOptions(object); err != nil {
		return latexAsset{}, err
	}
	currentColor, err := b.latexCurrentColor(rawColor, object+" latex currentColor")
	if err != nil {
		return latexAsset{}, err
	}
	key := latexAssetKey{formula: formula, color: currentColor}
	if asset, ok := b.latexAssets[key]; ok {
		return asset, nil
	}

	source, err := d2latex.Render(formula)
	if err != nil {
		return latexAsset{}, fmt.Errorf("scene: %s latex render: %w", object, err)
	}
	if err := b.ctx.Err(); err != nil {
		return latexAsset{}, err
	}
	limits, err := b.remainingSVGImportLimits(object + " latex")
	if err != nil {
		return latexAsset{}, err
	}
	result, err := d2svgimport.ImportNodeWithOptions(b.ctx, object+" latex", []byte(source), limits, d2svgimport.ImportOptions{
		CurrentColor: &currentColor,
	})
	if err != nil {
		return latexAsset{}, fmt.Errorf("scene: %s latex SVG: %w", object, err)
	}
	if err := b.reserveSVGImport(object+" latex", result.Metrics); err != nil {
		return latexAsset{}, err
	}
	if err := b.mergeSVGImportAssets(object+" latex", result.Assets); err != nil {
		return latexAsset{}, err
	}

	digest := sha256.New()
	_, _ = digest.Write([]byte("d2-raster-latex\x00"))
	_, _ = digest.Write([]byte(source))
	_, _ = digest.Write([]byte{currentColor.R, currentColor.G, currentColor.B, currentColor.A})
	id := d2scene.AssetID(fmt.Sprintf("latex:%x", digest.Sum(nil)))
	content := d2scene.NewNode(nil)
	content.ID = string(id) + ":content"
	content.Transform = result.ViewportTransform
	content.Children = []*d2scene.Node{result.Root}
	viewport := d2scene.Box{Width: result.Width, Height: result.Height}
	root := d2scene.NewNode(nil)
	root.ID = string(id) + ":viewport"
	root.Clip = boxClip(viewport)
	root.Children = []*d2scene.Node{content}
	b.assets[id] = d2scene.VectorAsset{ViewBox: viewport, Root: root}
	asset := latexAsset{id: id, width: result.Width, height: result.Height}
	b.latexAssets[key] = asset
	return asset, nil
}

func (b *builder) validateLatexInput(object, formula string) error {
	if !utf8.ValidString(formula) {
		return invalidField(object, "label", nil, "must be valid UTF-8")
	}
	if len(formula) > maxLatexInputBytes {
		return fmt.Errorf("scene: %s latex input is %d bytes, exceeding limit %d", object, len(formula), maxLatexInputBytes)
	}
	return nil
}

func (b *builder) validateLatexImportOptions(object string) error {
	if b.options.Assets == nil {
		return unsupported(object, "label language latex without configured SVG import limits")
	}
	limits := b.options.Assets.SVGImportLimits
	if limits.MaxBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxElements <= 0 || limits.MaxAttributes <= 0 ||
		limits.MaxAttributeBytes <= 0 || limits.MaxPathCommands <= 0 || limits.MaxTransformFunctions <= 0 ||
		limits.MaxUseDepth <= 0 || limits.MaxResources <= 0 {
		return fmt.Errorf("scene: %s latex label requires positive per-import SVG limits", object)
	}
	budget := b.options.Assets.SVGImportBudget
	if budget.MaxSourceBytes <= 0 || budget.MaxElements <= 0 || budget.MaxAttributes <= 0 || budget.MaxAttributeBytes <= 0 ||
		budget.MaxPathCommands <= 0 || budget.MaxTransformFunctions <= 0 || budget.MaxDeclaredResources <= 0 || budget.MaxExpandedUseInstances <= 0 {
		return fmt.Errorf("scene: %s latex label requires positive document-wide SVG import budgets", object)
	}
	return nil
}

func (b *builder) latexCurrentColor(raw, description string) (color.NRGBA, error) {
	if raw == "" {
		return color.NRGBA{A: 255}, nil
	}
	paint, err := b.paint(raw, description)
	if err != nil {
		return color.NRGBA{}, err
	}
	if paint == nil {
		// Invalid CSS colors such as color:none do not override the initial black.
		return color.NRGBA{A: 255}, nil
	}
	solid, ok := paint.(d2scene.SolidPaint)
	if !ok {
		return color.NRGBA{}, fmt.Errorf("scene: %s must resolve to a solid color", description)
	}
	return solid.Color, nil
}

func (b *builder) buildLatexLabelNode(object, nodeID, formula, rawColor string, topLeft *geo.Point) (*d2scene.Node, error) {
	if topLeft == nil || !finite(topLeft.X) || !finite(topLeft.Y) {
		return nil, invalidField(object, "labelPosition", nil, "must resolve to a finite LaTeX label origin")
	}
	asset, err := b.resolveLatexAsset(object, formula, rawColor)
	if err != nil {
		return nil, err
	}
	node := d2scene.NewNode(d2scene.Image{
		Asset: asset.id,
		Box: d2scene.Box{
			X: topLeft.X, Y: topLeft.Y,
			Width: asset.width, Height: asset.height,
		},
		Aspect: defaultImageAspect,
	})
	node.ID = nodeID
	return node, nil
}

// latexShapeLabelTopLeft places border labels against the inner box; 3D and
// multiple effects do not expand it.
func latexShapeLabelTopLeft(targetShape d2target.Shape) *geo.Point {
	position := label.FromString(targetShape.LabelPosition)
	if position == label.Unset {
		position = label.InsideMiddleCenter
	}
	geometry := targetGeometry(targetShape)
	box := geometry.GetInnerBox()
	if position.IsOutside() {
		box = geometry.GetBox()
	}
	return position.GetPointOnBox(
		box, label.PADDING,
		float64(targetShape.LabelWidth), float64(targetShape.LabelHeight),
	)
}
