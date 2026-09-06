package d2scenebuild

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"unicode/utf8"

	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func (b *builder) compileLinkRegions() error {
	hasMetadata := false
	for _, targetShape := range b.diagram.Shapes {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if targetShape.Link != "" || targetShape.Tooltip != "" || targetShape.PrettyLink != "" {
			hasMetadata = true
			break
		}
	}
	if !hasMetadata {
		for _, connection := range b.diagram.Connections {
			if err := b.ctx.Err(); err != nil {
				return err
			}
			if connection.Link != "" || connection.Tooltip != "" || connection.PrettyLink != "" {
				hasMetadata = true
				break
			}
		}
	}
	if !hasMetadata {
		return b.ctx.Err()
	}
	budget := b.options.LinkBudget
	if budget.MaxRegions <= 0 || budget.MaxStringBytes <= 0 {
		return invalidField("options", "linkBudget", budget, "must provide positive MaxRegions and MaxStringBytes for link/tooltip metadata")
	}

	startLinks, startBytes := len(b.links), b.linkBytes
	startAppendixItems, startAppendixBytes := len(b.appendixItems), b.appendixStringBytes
	complete := false
	defer func() {
		if !complete {
			b.links = b.links[:startLinks]
			b.linkBytes = startBytes
			b.appendixItems = b.appendixItems[:startAppendixItems]
			b.appendixStringBytes = startAppendixBytes
		}
	}()
	for index, targetShape := range b.diagram.Shapes {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if targetShape.Link == "" && targetShape.Tooltip == "" && targetShape.PrettyLink == "" {
			continue
		}
		object := fmt.Sprintf("shape %q", targetShape.ID)
		region, err := b.shapeLinkRegion(object, targetShape)
		if err != nil {
			return err
		}
		if err := b.addLinkRegion(object, region); err != nil {
			return err
		}
		if err := b.addAppendixItem(fmt.Sprintf("shape[%d] %q", index, targetShape.ID), "tooltip", targetShape.Tooltip); err != nil {
			return err
		}
		if err := b.addAppendixItem(fmt.Sprintf("shape[%d] %q", index, targetShape.ID), "prettyLink", targetShape.PrettyLink); err != nil {
			return err
		}
	}
	for index, connection := range b.diagram.Connections {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if connection.Link == "" && connection.Tooltip == "" && connection.PrettyLink == "" {
			continue
		}
		object := fmt.Sprintf("connection %q", connection.ID)
		region, err := b.connectionLinkRegion(object, connection)
		if err != nil {
			return err
		}
		if err := b.addLinkRegion(object, region); err != nil {
			return err
		}
		if err := b.addAppendixItem(fmt.Sprintf("connection[%d] %q", index, connection.ID), "tooltip", connection.Tooltip); err != nil {
			return err
		}
		if err := b.addAppendixItem(fmt.Sprintf("connection[%d] %q", index, connection.ID), "prettyLink", connection.PrettyLink); err != nil {
			return err
		}
	}
	complete = true
	return nil
}

func (b *builder) shapeLinkRegion(object string, targetShape d2target.Shape) (d2scene.LinkRegion, error) {
	if targetShape.PrettyLink != "" && targetShape.Link == "" {
		return d2scene.LinkRegion{}, invalidField(object, "prettyLink", targetShape.PrettyLink, "requires a non-empty link")
	}
	if err := b.validateLinkFields(object, targetShape.Link, targetShape.Tooltip); err != nil {
		return d2scene.LinkRegion{}, err
	}
	if err := validateLinkTooltipSecurity(object, targetShape.Link, targetShape.Tooltip); err != nil {
		return d2scene.LinkRegion{}, err
	}
	urlValue, target := linkDestination(targetShape.Link)
	strokeWidth := float64(targetShape.StrokeWidth)
	region := d2scene.LinkRegion{
		// PDF/PPTX annotations expand a shape by its full stroke width on every
		// side.
		Box: d2scene.Box{
			X:      float64(targetShape.Pos.X) - strokeWidth,
			Y:      float64(targetShape.Pos.Y) - strokeWidth,
			Width:  float64(targetShape.Width) + 2*strokeWidth,
			Height: float64(targetShape.Height) + 2*strokeWidth,
		},
		URL: urlValue, Tooltip: targetShape.Tooltip, Target: target,
	}
	return region, nil
}

func (b *builder) connectionLinkRegion(object string, connection d2target.Connection) (d2scene.LinkRegion, error) {
	if connection.PrettyLink != "" && connection.Link == "" {
		return d2scene.LinkRegion{}, invalidField(object, "prettyLink", connection.PrettyLink, "requires a non-empty link")
	}
	if err := b.validateLinkFields(object, connection.Link, connection.Tooltip); err != nil {
		return d2scene.LinkRegion{}, err
	}
	if err := validateLinkTooltipSecurity(object, connection.Link, connection.Tooltip); err != nil {
		return d2scene.LinkRegion{}, err
	}
	topLeft := connection.GetLabelTopLeft()
	if topLeft == nil {
		return d2scene.LinkRegion{}, invalidField(object, "labelPosition", connection.LabelPosition, "does not resolve to a finite metadata hit region")
	}
	urlValue, target := linkDestination(connection.Link)
	return d2scene.LinkRegion{
		// Only the rounded connection-label box is interactive; the route, arrows,
		// endpoint labels, and icon are not.
		Box: d2scene.Box{
			X: math.Round(topLeft.X), Y: math.Round(topLeft.Y),
			Width: float64(connection.LabelWidth), Height: float64(connection.LabelHeight),
		},
		URL: urlValue, Tooltip: connection.Tooltip, Target: target,
	}, nil
}

func validateLinkTooltipSecurity(object, link, tooltip string) error {
	if link == "" || tooltip == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(tooltip)
	if err == nil && parsed.Host != "" {
		return invalidField(object, "tooltip", tooltip, "must not be a URL when link is also set")
	}
	return nil
}

// linkDestination preserves the PDF/PPTX distinction: a parsed D2 key
// rooted at "root" is a board target; every other value, including relative
// paths and opaque application schemes, remains an external URL string.
func linkDestination(value string) (urlValue, target string) {
	if value == "" {
		return "", ""
	}
	key, err := d2parser.ParseKey(value)
	if err == nil && len(key.Path) != 0 && key.Path[0].Unbox().ScalarString() == "root" {
		return "", value
	}
	return value, ""
}

func (b *builder) validateLinkFields(object, link, tooltip string) error {
	nextBytes := b.linkBytes
	for _, field := range []struct {
		name  string
		value string
	}{{"link", link}, {"tooltip", tooltip}} {
		if len(field.value) > b.options.LinkBudget.MaxStringBytes-nextBytes {
			return fmt.Errorf("scene: link metadata string bytes exceed limit %d", b.options.LinkBudget.MaxStringBytes)
		}
		if err := validateLinkString(b.ctx, object, field.name, field.value); err != nil {
			return err
		}
		nextBytes += len(field.value)
	}
	return nil
}

func (b *builder) addLinkRegion(object string, region d2scene.LinkRegion) error {
	budget := b.options.LinkBudget
	if len(b.links) >= budget.MaxRegions {
		return fmt.Errorf("scene: link region count exceeds limit %d", budget.MaxRegions)
	}
	if !finite(region.Box.X) || !finite(region.Box.Y) || !finite(region.Box.Width) || !finite(region.Box.Height) ||
		region.Box.Width <= 0 || region.Box.Height <= 0 {
		return invalidField(object, "linkRegion", region.Box, "must be a finite box with positive dimensions")
	}
	if region.URL != "" && region.Target != "" {
		return fmt.Errorf("scene: %s link metadata has both URL and target destinations", object)
	}
	if region.URL == "" && region.Tooltip == "" && region.Target == "" {
		return fmt.Errorf("scene: %s link metadata is empty", object)
	}

	nextBytes := b.linkBytes
	for _, field := range []struct {
		name  string
		value string
	}{{"URL", region.URL}, {"tooltip", region.Tooltip}, {"target", region.Target}} {
		if len(field.value) > budget.MaxStringBytes-nextBytes {
			return fmt.Errorf("scene: link metadata string bytes exceed limit %d", budget.MaxStringBytes)
		}
		nextBytes += len(field.value)
	}
	if err := b.ctx.Err(); err != nil {
		return err
	}
	b.links = append(b.links, region)
	b.linkBytes = nextBytes
	return nil
}

func validateLinkString(ctx context.Context, object, field, value string) error {
	nextCheck := 0
	for offset := 0; offset < len(value); {
		if offset >= nextCheck {
			if err := ctx.Err(); err != nil {
				return err
			}
			nextCheck = offset + 4096
		}
		r, size := utf8.DecodeRuneInString(value[offset:])
		if r == utf8.RuneError && size == 1 {
			return invalidField(object, field, nil, "must be valid UTF-8")
		}
		if !validXMLMetadataRune(r) {
			return invalidField(object, field, nil, "contains a character forbidden by XML 1.0 metadata consumers")
		}
		offset += size
	}
	return ctx.Err()
}

func validXMLMetadataRune(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' ||
		r >= 0x20 && r <= 0xd7ff ||
		r >= 0xe000 && r <= 0xfffd ||
		r >= 0x10000 && r <= 0x10ffff
}

// visualBoundingBox removes non-pixel metadata from a shallow diagram copy.
// d2target.BoundingBox reserves appendix-icon space for ordinary Link and
// Tooltip metadata, but LinkRegions themselves must not perturb scene pixels
// or leave blank margins. Positioned tooltips are actual scene pixels, so keep
// precisely their tooltip fields and let d2target retain their svg bounds.
func (b *builder) visualBoundingBox() (d2target.Point, d2target.Point) {
	hasShapeMetadata := false
	for _, targetShape := range b.diagram.Shapes {
		if targetShape.Link != "" || targetShape.Tooltip != "" || targetShape.PrettyLink != "" || targetShape.TooltipPosition != "" {
			hasShapeMetadata = true
			break
		}
	}
	if !hasShapeMetadata {
		return b.diagram.BoundingBox()
	}
	copyDiagram := *b.diagram
	copyDiagram.Shapes = append([]d2target.Shape(nil), b.diagram.Shapes...)
	for index := range copyDiagram.Shapes {
		positionedTooltip := copyDiagram.Shapes[index].Tooltip != "" && copyDiagram.Shapes[index].TooltipPosition != ""
		copyDiagram.Shapes[index].Link = ""
		copyDiagram.Shapes[index].PrettyLink = ""
		if !positionedTooltip {
			copyDiagram.Shapes[index].Tooltip = ""
			copyDiagram.Shapes[index].TooltipPosition = ""
		}
	}
	return copyDiagram.BoundingBox()
}
