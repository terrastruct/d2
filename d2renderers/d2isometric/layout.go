package d2isometric

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/d2lang/d2/d2target"
)

// SceneScale maps compiled layout pixels to world-space units.
const SceneScale = .01

func moduleSize(source d2target.Shape) Vec3 {
	height := .7
	switch source.Type {
	case d2target.ShapeCylinder:
		height = 1.15
	case d2target.ShapeQueue:
		height = .85
	case d2target.ShapePerson:
		height = 1.35
	}
	return Vec3{X: float64(max(1, source.Width)) * SceneScale, Y: height, Z: float64(max(1, source.Height)) * SceneScale}
}

func boardLabelMetrics(board Board, nodes []Node, index map[string]int) (float64, float64) {
	fontSize := 16
	labelWidth, labelHeight := 0, 0
	if i, ok := index[board.SourceID]; ok && board.SourceID != "" {
		s := nodes[i].Metadata.Original
		labelWidth, labelHeight = s.LabelWidth, s.LabelHeight
		if s.FontSize > 0 {
			fontSize = s.FontSize
		}
	}
	if labelWidth <= 0 {
		for _, line := range strings.Split(board.Label, "\n") {
			labelWidth = max(labelWidth, int(math.Ceil(float64(utf8.RuneCountInString(line)*fontSize)*.65)))
		}
	}
	if labelHeight <= 0 {
		labelHeight = int(math.Ceil(float64(fontSize)*1.3)) * max(1, len(strings.Split(board.Label, "\n")))
	}
	return float64(labelWidth) * SceneScale, float64(labelHeight) * SceneScale
}

const surfaceHeight = .08
