package d2ascii

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
)

type ASCIIartist struct {
	canvas  [][]string
	FW      float64
	FH      float64
	chars   map[string]string
	entr    string
	bcurve  string
	tcurve  string
	SCALE   float64
	diagram d2target.Diagram
}
type RenderOpts struct {
	Scale *float64
}
type Boundary struct {
	TL []int
	BR []int
}

func (b *Boundary) Contains(x int, y int) bool {
	return x >= b.TL[0] && x <= b.BR[0] && y >= b.TL[1] && y <= b.BR[1]
}

func NewBoundary(TL []int, BR []int) *Boundary {
	boundary := &Boundary{
		TL: TL,
		BR: BR,
	}
	return boundary
}

func (a *ASCIIartist) GetBoundary(s d2target.Shape) ([]int, []int) {
	x1 := int(math.Round((float64(s.Pos.X) / a.FW) * a.SCALE))
	y1 := int(math.Round((float64(s.Pos.Y) / a.FH) * a.SCALE))
	x2 := int(math.Round(((float64(s.Pos.X) + float64(s.Width) - 1) / a.FW) * a.SCALE))
	y2 := int(math.Round(((float64(s.Pos.Y) + float64(s.Height) - 1) / a.FH) * a.SCALE))
	return []int{x1, y1}, []int{x2, y2}
}

func (a *ASCIIartist) GetActualBoundary(s d2target.Shape) ([]int, []int) {
	return []int{s.Pos.X, s.Pos.Y}, []int{s.Pos.X + s.Width, s.Pos.Y + s.Height}
}

func NewASCIIartist() *ASCIIartist {
	artist := &ASCIIartist{
		FW:     9.75,
		FH:     18,
		SCALE:  1,
		entr:   "\n",
		bcurve: "`-._",
		tcurve: ".-`‾",
		chars: map[string]string{
			"TLA": "╭", "TRA": "╮", "BLA": "╰", "BRA": "╯",
			"HOR": "─", "VER": "│", "LVER": "▏", "RVER": "▕",
			"TLC": "┌", "TRC": "┐", "BLC": "└", "BRC": "┘",
			"BS": "╲", "FS": "╱", "X": "╳", "US": "_", "OL": "‾",
			"DOT": ".", "HPN": "-", "TLD": "`", "TDO": "┬", "TLE": "┤", "TRI": "├", "TUP": "┴",
		},
		diagram: *d2target.NewDiagram(),
	}

	return artist
}
func (a *ASCIIartist) Render(diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	if opts == nil {
		opts = &RenderOpts{}
	}
	xOffset := 0
	yOffset := 0
	a.diagram = *diagram
	tl, br := diagram.NestedBoundingBox()
	if tl.X < 0 {
		xOffset = -tl.X
		br.X += -tl.X
		tl.X = 0
	}
	if tl.Y < 0 {
		yOffset = -tl.Y
		br.Y += -tl.Y
		tl.Y = 0
	}
	w := int(math.Ceil(float64(br.X - tl.X)))
	h := int(math.Ceil(float64(br.Y - tl.Y)))

	w = int(math.Round((float64(w) / a.FW) * a.SCALE))
	h = int(math.Round((float64(h) / a.FH) * a.SCALE))

	a.canvas = make([][]string, h+1)
	for i := range a.canvas {
		a.canvas[i] = make([]string, w+1)
		for j := range a.canvas[i] {
			a.canvas[i][j] = " "
		}
	}

	// Draw shapes
	for _, shape := range diagram.Shapes {
		if shape.Classes != nil && slices.Contains(shape.Classes, "NONE") {
			continue
		}
		shape.Pos.X += xOffset
		shape.Pos.Y += yOffset
		switch shape.Type {
		case d2target.ShapeRectangle:
			a.drawRect(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition, "")
		case d2target.ShapeSquare:
			a.drawRect(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition, "")
		case d2target.ShapePage:
			a.drawPage(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeHexagon:
			a.drawHex(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapePerson:
			a.drawPerson(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeStoredData:
			a.drawStoredData(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeCylinder:
			a.drawCylinder(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapePackage:
			a.drawPackage(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeParallelogram:
			a.drawParallelogram(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeQueue:
			a.drawQueue(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeStep:
			a.drawStep(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeCallout:
			a.drawCallout(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeDocument:
			a.drawDocument(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		case d2target.ShapeDiamond:
			a.drawDiamond(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition)
		default:
			symbol := ""
			switch shape.Type {
			case d2target.ShapeCloud:
				symbol = "☁"
			case d2target.ShapeCircle:
				symbol = "●"
			case d2target.ShapeOval:
				symbol = "⬭"
			default:
				symbol = ""
			}
			a.drawRect(float64(shape.Pos.X), float64(shape.Pos.Y), float64(shape.Width), float64(shape.Height), shape.Label, shape.LabelPosition, symbol)
		}
	}
	// Draw connections
	for _, conn := range diagram.Connections {
		for _, r := range conn.Route {
			r.X += float64(xOffset)
			r.Y += float64(yOffset)
		}
		a.drawRoute(conn)
	}
	return a.toByteArray(), nil
}
func (a *ASCIIartist) toByteArray() []byte {
	var buf bytes.Buffer
	startRow := 0
	// Skip empty lines at the beginning
	for i, row := range a.canvas {
		if strings.TrimSpace(strings.Join(row, "")) != "" {
			startRow = i
			break
		}
	}
	for i := startRow; i < len(a.canvas); i++ {
		buf.WriteString(strings.Join(a.canvas[i], ""))
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}
func (a *ASCIIartist) calibrate(x, y, w, h float64) (int, int, int, int) {
	xC := int(math.Round((x / a.FW) * a.SCALE))
	yC := int(math.Round((y / a.FH) * a.SCALE))
	wC := int(math.Round((w / a.FW) * a.SCALE))
	hC := int(math.Round((h / a.FH) * a.SCALE))
	return xC, yC, wC, hC
}

func (a *ASCIIartist) calibrateXY(x, y float64) (float64, float64) {
	xC := float64(math.Round((x / a.FW) * a.SCALE))
	yC := float64(math.Round((y / a.FH) * a.SCALE))
	return xC, yC
}

func (a *ASCIIartist) labelY(y1, y2, h int, label, labelPosition string) int {
	ly := -1
	if strings.Contains(labelPosition, "OUTSIDE") {
		if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 + 1
		} else if strings.Contains(labelPosition, "TOP") {
			ly = y1 - 1
		}
	} else {
		if strings.Contains(labelPosition, "TOP") {
			ly = y1 + 1
		} else if strings.Contains(labelPosition, "MIDDLE") {
			ly = y1 + h/2
		} else if strings.Contains(labelPosition, "BOTTOM") {
			ly = y2 - 1
		}
	}
	return ly
}

func (a *ASCIIartist) drawLabel(x, y int, label string) {
	if y < 0 || y >= len(a.canvas) {
		return
	}
	for i, c := range label {
		if x+i < len(a.canvas[y]) && x+i >= 0 {
			a.canvas[y][x+i] = string(c)
		}
	}
}

func (a *ASCIIartist) drawRect(x, y, w, h float64, label, labelPosition, symbol string) {
	x1, y1, wC, hC := a.calibrate(x, y, w, h)
	if label != "" && hC%2 == 0 {
		if hC > 2 {
			hC--
			y1++
		} else {
			hC++
		}
	}
	x2, y2 := x1+wC, y1+hC
	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1):     a.chars["TLC"],
		fmt.Sprintf("%d_%d", x2-1, y1):   a.chars["TRC"],
		fmt.Sprintf("%d_%d", x1, y2-1):   a.chars["BLC"],
		fmt.Sprintf("%d_%d", x2-1, y2-1): a.chars["BRC"],
	}
	for xi := x1; xi < x2; xi++ {
		for yi := y1; yi < y2; yi++ {
			key := fmt.Sprintf("%d_%d", xi, yi)
			if val, ok := corners[key]; ok {
				a.canvas[yi][xi] = val
			} else if strings.TrimSpace(symbol) != "" && yi == y1 && xi == x1+1 {
				a.canvas[yi][xi] = symbol
			} else if xi == x1 || xi == x2-1 {
				a.canvas[yi][xi] = a.chars["VER"]
			} else if yi == y1 || yi == y2-1 {
				a.canvas[yi][xi] = a.chars["HOR"]
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hC, label, labelPosition)
		lx := x1 + (wC-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawPage(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	x3 := x2 - wi/3
	y3 := y2 - hi/2

	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): a.chars["TLC"],
		fmt.Sprintf("%d_%d", x2, y1): a.chars["TRC"],
		fmt.Sprintf("%d_%d", x1, y2): a.chars["BLC"],
		fmt.Sprintf("%d_%d", x2, y2): a.chars["BRC"],
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			key := fmt.Sprintf("%d_%d", x, y)
			if val, ok := corners[key]; ok && !(x > x3 && y < y3) {
				a.canvas[y][x] = val
			} else if x == x1 || (x == x2 && y > y3) {
				a.canvas[y][x] = a.chars["VER"]
			} else if (y == y1 && x < x3) || y == y2 {
				a.canvas[y][x] = a.chars["HOR"]
			} else if (x == x3 && y == y1) || (x == x2 && y == y3) {
				a.canvas[y][x] = a.chars["TRC"]
			} else if x == x3 && y == y3 {
				a.canvas[y][x] = a.chars["BLC"]
			} else if x == x2 && y == y3 {
				a.canvas[y][x] = a.chars["TRC"]
			} else if x == x3 && y < y3 {
				a.canvas[y][x] = a.chars["VER"]
			} else if x > x3 && y == y3 {
				a.canvas[y][x] = a.chars["HOR"]
			} else if x > x3 && x < x2 && y < y3 && y > y1 {
				a.canvas[y][x] = a.chars["BS"]
			} else {
				a.canvas[y][x] = " "
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawHex(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	hoffset := int(math.Ceil(float64(hi) / 2.0))

	for i := x1; i <= x2; i++ {
		for j := y1; j <= y2; j++ {
			switch {
			case j == y1 && i >= (x1+hoffset) && i <= (x2-hoffset):
				a.canvas[j][i] = a.chars["OL"]
			case j == y2 && i >= (x1+hoffset) && i <= (x2-hoffset):
				a.canvas[j][i] = a.chars["US"]
			case hoffset%2 == 1 && (i == x1 || i == x2) && (y1+hoffset-1) == j:
				a.canvas[j][i] = a.chars["X"]
			case ((j-y1)+(i-x1)+1) == hoffset || ((y2-j)+(x2-i)+1) == hoffset:
				a.canvas[j][i] = a.chars["FS"]
			case ((j-y1)+(x2-i)+1) == hoffset || ((y2-j)+(i-x1)+1) == hoffset:
				a.canvas[j][i] = a.chars["BS"]
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawPerson(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	head := 2
	body := hi - 2
	hw := 2
	if wi%2 == 1 {
		hw = 3
	}
	hoffset := (wi - hw) / 2
	s := body - 1

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			_x, _y := x-x1, y-y1
			_x1, _y1 := _x, _y-head

			switch {
			case y == y2:
				a.canvas[y][x] = a.chars["OL"]
			case y >= y1+head && y < y2:
				if (_x + _y) == body {
					a.canvas[y][x] = a.chars["FS"]
				} else if (float64(_x1 - _y1 - 1)) == math.Abs(float64(wi-(hi-head))) {
					a.canvas[y][x] = a.chars["BS"]
				} else if y == y1+head && x >= x1+s && x <= x2-s {
					a.canvas[y][x] = a.chars["OL"]
				}
			case y < y1+head:
				if y == y1 && x >= x1+hoffset && x <= x2-hoffset {
					a.canvas[y][x] = a.chars["OL"]
				}
				if y == y1+head-1 && x >= x1+hoffset && x <= x2-hoffset {
					a.canvas[y][x] = a.chars["US"]
				}
				if (y == y1 && x == x1+hoffset-1) || (y == y1+head-1 && x == x2-hoffset+1) {
					a.canvas[y][x] = a.chars["FS"]
				}
				if (y == y1+head-1 && x == x1+hoffset-1) || (y == y1 && x == x2-hoffset+1) {
					a.canvas[y][x] = a.chars["BS"]
				}
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawStoredData(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	if hi < 5 {
		hi = 5
	} else if hi%2 == 0 {
		hi++
	}
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	hoffset := (hi + 1) / 2

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			_x, _y := x-x1, y-y1

			switch {
			case y == y1+hoffset-1 && x == x1:
				a.canvas[y][x] = a.chars["VER"]
			case x < x1+hoffset:
				if y < y1+hoffset && (_x+_y) == hoffset-1 {
					a.canvas[y][x] = a.chars["FS"]
				} else if y >= y1+hoffset && int(math.Abs(float64(_x-_y))) == hoffset-1 {
					a.canvas[y][x] = a.chars["BS"]
				}
			case x >= x1+hoffset:
				if y == y1 && x < x2 {
					a.canvas[y][x] = a.chars["OL"]
				} else if y == y2 && x < x2 {
					a.canvas[y][x] = a.chars["US"]
				} else if x > x2-hoffset {
					if y == y1+hoffset-1 && x == x2-(hoffset-1) {
						a.canvas[y][x] = a.chars["VER"]
					} else if (_x + _y) == wi-1 {
						a.canvas[y][x] = a.chars["FS"]
					} else if int(math.Abs(float64(_x-_y))) == int(math.Abs(float64(wi-hi))) {
						a.canvas[y][x] = a.chars["BS"]
					}
				}
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawCylinder(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			switch {
			case iy != y1 && iy != y2 && (ix == x1 || ix == x2):
				a.canvas[iy][ix] = a.chars["VER"]
			case iy == y1 || iy == y2 || iy == y1+1:
				if iy == y1 {
					if ix == x1+1 || ix == x2-1 {
						a.canvas[iy][ix] = a.chars["DOT"]
					} else if ix == x1+2 || ix == x2-2 {
						a.canvas[iy][ix] = a.chars["HPN"]
					} else if ix > x1+2 && ix < x2-2 {
						a.canvas[iy][ix] = a.chars["OL"]
					}
				} else if iy == y2 || iy == y1+1 {
					if ix == x1+1 {
						a.canvas[iy][ix] = a.chars["BS"]
					} else if ix == x2-1 {
						a.canvas[iy][ix] = a.chars["FS"]
					} else if ix == x1+2 || ix == x2-2 {
						a.canvas[iy][ix] = a.chars["HPN"]
					} else if ix > x1+2 && ix < x2-2 {
						a.canvas[iy][ix] = a.chars["US"]
					}
				}
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1+1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawPackage(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1
	x3, y3 := x1+wi/2, y1+1

	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): a.chars["TLC"],
		fmt.Sprintf("%d_%d", x3, y1): a.chars["TRC"],
		fmt.Sprintf("%d_%d", x2, y3): a.chars["TRC"],
		fmt.Sprintf("%d_%d", x3, y3): a.chars["BLC"],
		fmt.Sprintf("%d_%d", x1, y2): a.chars["BLC"],
		fmt.Sprintf("%d_%d", x2, y2): a.chars["BRC"],
	}

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			key := fmt.Sprintf("%d_%d", ix, iy)
			if char, ok := corners[key]; ok {
				a.canvas[iy][ix] = char
			} else if (iy == y1 && ix > x1 && ix < x3) || (iy == y2 && ix > x1 && ix < x2) || (iy == y3 && ix > x3 && ix < x2) {
				a.canvas[iy][ix] = a.chars["HOR"]
			} else if (ix == x1 && iy > y1 && iy < y2) || (ix == x2 && iy > y3 && iy < y2) {
				a.canvas[iy][ix] = a.chars["VER"]
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawParallelogram(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			_x, _y := ix-x1, iy-y1
			if (_x+_y == hi-1) || (_x+_y == wi-1) {
				a.canvas[iy][ix] = a.chars["FS"]
			} else if iy == y1 && ix >= x1+hi && ix < x2 {
				a.canvas[iy][ix] = a.chars["OL"]
			} else if iy == y2 && ix > x1 && ix <= x2-hi {
				a.canvas[iy][ix] = a.chars["US"]
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawQueue(x, y, w, h float64, label, labelPosition string) {
	xi, yi, wi, hi := a.calibrate(x, y, w, h)
	x1, y1 := xi, yi
	x2, y2 := xi+wi-1, yi+hi-1

	for ix := x1; ix <= x2; ix++ {
		for iy := y1; iy <= y2; iy++ {
			switch {
			case (iy == y1 && (ix == x1+1 || ix == x2-2)) || (iy == y2 && ix == x2-1):
				a.canvas[iy][ix] = a.chars["FS"]
			case (iy == y1 && ix == x2-1) || (iy == y2 && (ix == x1+1 || ix == x2-2)):
				a.canvas[iy][ix] = a.chars["BS"]
			case (ix == x1 || ix == x2 || ix == x2-3) && (iy > y1 && iy < y2):
				a.canvas[iy][ix] = a.chars["VER"]
			case iy == y1 && ix > x1+1 && ix < x2-1:
				a.canvas[iy][ix] = a.chars["OL"]
			case iy == y2 && ix > x1+1 && ix < x2-3:
				a.canvas[iy][ix] = a.chars["US"]
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, hi, label, labelPosition)
		lx := x1 + (wi-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawStep(x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := a.calibrate(x, y, w, h)
	if ih%2 == 1 {
		ih++
	}
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			_x, _y := x-x1, y-y1
			if (x < x1+ih/2 && _x-_y == 0) || (x > x2-ih/2 && abs(_x-_y) == iw-ih/2) {
				a.canvas[y][x] = a.chars["BS"]
			} else if (x < x1+ih/2 && _x+_y == ih-1) || (x > x2-ih/2 && _x+_y == iw-1+ih/2) {
				a.canvas[y][x] = a.chars["FS"]
			} else if y == y1 && x > x1 && x < x2-ih/2 {
				a.canvas[y][x] = a.chars["OL"]
			} else if y == y2 && x > x1 && x < x2-ih/2 {
				a.canvas[y][x] = a.chars["US"]
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, ih, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawCallout(x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := a.calibrate(x, y, w, h)
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1
	body := (ih + 1) / 2
	tail := ih / 2

	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1):      a.chars["TLC"],
		fmt.Sprintf("%d_%d", x2, y1):      a.chars["TRC"],
		fmt.Sprintf("%d_%d", x1, y2-tail): a.chars["BLC"],
		fmt.Sprintf("%d_%d", x2, y2-tail): a.chars["BRC"],
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			_x, _y := x-x1, y-y1
			k := fmt.Sprintf("%d_%d", x, y)
			if char, ok := corners[k]; ok {
				a.canvas[y][x] = char
			} else if (y == y1 || y == y2-tail) && x > x1 && x < x2 {
				a.canvas[y][x] = a.chars["HOR"]
			} else if (x == x1 || x == x2) && y > y1 && y < y2-tail {
				a.canvas[y][x] = a.chars["VER"]
			} else if x == x2-(tail+2) && y > y2-tail {
				a.canvas[y][x] = a.chars["VER"]
			} else if y > y2-tail && _x+_y == iw {
				a.canvas[y][x] = a.chars["FS"]
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, body, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (a *ASCIIartist) drawDocument(x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := a.calibrate(x, y, w, h)
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1
	n := (iw - 2) / 2
	j := n / 2
	if j > 3 {
		j = 3
	}
	hcurve := j + 1

	lcurve := make([]rune, n)
	rcurve := make([]rune, n)
	for i := 0; i < n; i++ {
		if i < hcurve {
			lcurve[i] = rune(a.bcurve[i])
			rcurve[i] = rune(a.tcurve[i])
		} else if abs(i-n+1) < hcurve {
			lcurve[i] = rune(a.bcurve[abs(i-n+1)])
			rcurve[i] = rune(a.tcurve[abs(i-n+1)])
		} else {
			lcurve[i] = rune(a.bcurve[3])
			rcurve[i] = rune(a.tcurve[3])
		}
	}
	corners := map[string]string{
		fmt.Sprintf("%d_%d", x1, y1): a.chars["TLC"],
		fmt.Sprintf("%d_%d", x2, y1): a.chars["TRC"],
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			_x := x - x1
			x3 := _x - 1
			k := fmt.Sprintf("%d_%d", x, y)
			if char, ok := corners[k]; ok {
				a.canvas[y][x] = char
			} else if y == y1 && x > x1 && x < x2 {
				a.canvas[y][x] = a.chars["HOR"]
			} else if (x == x1 || x == x2) && y > y1 && y < y2 {
				a.canvas[y][x] = a.chars["VER"]
			} else if y == y2 && x > x1 && _x <= n && x3 >= 0 && x3 < len(lcurve) {
				a.canvas[y][x] = string(lcurve[x3])
			} else if y == y2-1 && _x > n && x < x2 && (_x-int(iw/2)) < len(rcurve) {
				a.canvas[y][x] = string(rcurve[_x-int(iw/2)])
			}
		}
	}

	if label != "" {
		ly := a.labelY(y1, y2, ih-2, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		a.drawLabel(lx, ly, label)
	}
}
func (d *ASCIIartist) drawDiamond(x, y, w, h float64, label, labelPosition string) {
	ix, iy, iw, ih := d.calibrate(x, y, w, h)
	if ih%2 == 0 {
		ih++
	}
	if iw%2 == 0 {
		iw++
	}
	x1, y1, x2, y2 := ix, iy, ix+iw-1, iy+ih-1

	diagPath := [][2]int{
		{x1, y1 + ih/2},
		{x1 + iw/2, y1},
		{x2, y1 + ih/2},
		{x1 + iw/2, y2},
		{x1, y1 + ih/2},
	}

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			_x, _y := x-x1, y-y1
			if (y == y1 || y == y2) && _x == iw/2 {
				d.canvas[y][x] = d.chars["TLD"]
			} else if (x == x1 || x == x2) && _y == ih/2 {
				d.canvas[y][x] = d.chars["HPN"]
			}
		}
	}

	for i := 0; i < len(diagPath)-1; i++ {
		a, c := diagPath[i], diagPath[i+1]
		dx, dy := c[0]-a[0], c[1]-a[1]
		step := max(abs(dx), abs(dy))
		sx, sy := float64(dx)/float64(step), float64(dy)/float64(step)
		fx, fy := float64(a[0]), float64(a[1])
		for j := 0; j < step; j++ {
			fx += sx
			fy += sy
			x := int(math.Round(fx))
			y := int(math.Round(fy))
			d.canvas[y][x] = string('*')
		}
	}

	if label != "" {
		ly := d.labelY(y1, y2, ih, label, labelPosition)
		lx := x1 + (iw-len(label))/2
		d.drawLabel(lx, ly, label)
	}
}

func (aa *ASCIIartist) drawRoute(conn d2target.Connection) { //(routes []*geo.Point, dstArrow d2target.Arrowhead, label string) {
	// conn.Route, conn.DstArrow, conn.Label
	routes := conn.Route
	_label := conn.Label
	re := regexp.MustCompile(` -> | <-> | -- `)
	re1 := regexp.MustCompile(`\(([^}]*)\)`)
	re2 := regexp.MustCompile(`(.*)\(`)
	match1 := re1.FindStringSubmatch(conn.ID)
	match2 := re2.FindStringSubmatch(conn.ID)
	var frmShapeBoundary Boundary
	var toShapeBoundary Boundary
	if len(match1) > 0 {
		_ID := ""
		if len(match2) > 0 {
			_ID = match2[1]
		}
		splitResult := re.Split(match1[1], -1)
		for _, shape := range aa.diagram.Shapes {
			if len(splitResult) > 0 && shape.ID == _ID+splitResult[0] {
				frmShapeBoundary = *NewBoundary(aa.GetBoundary(shape))
				// _frmShapeBoundary = *NewBoundary(aa.GetActualBoundary(shape))
			} else if len(splitResult) > 1 && shape.ID == _ID+splitResult[1] {
				toShapeBoundary = *NewBoundary(aa.GetBoundary(shape))
				// _toShapeBoundary = *NewBoundary(aa.GetActualBoundary(shape))
			}
		}
	}
	routes = mergeRoutes(routes)
	for i := range routes {
		routes[i].X, routes[i].Y = aa.calibrateXY(routes[i].X, routes[i].Y)
		routes[i].X -= 1
	}
	// Determine turn directions
	turnDir := map[string]string{}
	_routes := make([][2]float64, len(routes))
	for i, r := range routes {
		_routes[i] = [2]float64{r.X, r.Y}
	}
	for i := 1; i < len(_routes)-1; i++ {
		r := _routes[i]
		r1 := _routes[i-1]
		r2 := _routes[i+1]
		key := fmt.Sprintf("%d_%d", int(math.Round(r[0])), int(math.Round(r[1])))
		dir := fmt.Sprintf("%d%d%d%d",
			geo.Sign(r[0]-r1[0]), geo.Sign(r[1]-r1[1]),
			geo.Sign(r2[0]-r[0]), geo.Sign(r2[1]-r[1]),
		)
		turnDir[key] = dir
	}

	corners := map[string]string{
		"-100-1": aa.chars["BLC"], "0110": aa.chars["BLC"],
		"-1001": aa.chars["TLC"], "0-110": aa.chars["TLC"],
		"0-1-10": aa.chars["TRC"], "1001": aa.chars["TRC"],
		"01-10": aa.chars["BRC"], "100-1": aa.chars["BRC"],
	}
	arrows := map[string]string{
		"0-1": "▲", "10": "▶", "01": "▼", "-10": "◀",
	}

	x := int(math.Round(routes[0].X))
	y := int(math.Round(routes[0].Y))

	for i := 1; i < len(routes); i++ {
		ax := routes[i-1].X
		ay := routes[i-1].Y
		cx := routes[i].X
		cy := routes[i].Y

		sx := cx - ax
		sy := cy - ay
		step := math.Max(math.Abs(sx), math.Abs(sy))
		if step == 0 {
			continue
		}
		sx /= step
		sy /= step

		fx, fy := ax, ay
		attempt := 0
		for {
			attempt++
			if x == int(math.Round(cx)) && y == int(math.Round(cy)) || attempt == 200 {
				break
			}
			x = int(math.Round(fx))
			y = int(math.Round(fy))

			// Check canvas bounds
			if y < 0 || y >= len(aa.canvas) || x < 0 || x >= len(aa.canvas[y]) {
				fx += sx
				fy += sy
				continue
			}

			isAlphaNumeric := false
			for _, r := range aa.canvas[y][x] {
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					isAlphaNumeric = true
				}
			}
			if isAlphaNumeric {
				fx += sx
				fy += sy
				continue
			}
			key := fmt.Sprintf("%d_%d", x, y)
			if char, ok := corners[turnDir[key]]; ok {
				aa.canvas[y][x] = char
			} else if i == len(routes)-1 && x == int(math.Round(cx)) && y == int(math.Round(cy)) && conn.DstArrow != d2target.NoArrowhead {
				arrowKey := fmt.Sprintf("%d%d", geo.Sign(sx), geo.Sign(sy))
				aa.canvas[y][x] = arrows[arrowKey]
				if conn.DstLabel != nil {
					ly := 0
					lx := 0
					if math.Abs(sx) > 0 {
						ly = int(cy - 1)
						if sx > 0 {
							lx = int(cx) - 1 - len(conn.DstLabel.Label)
						} else {
							lx = int(cx)
						}
					} else if math.Abs(sy) > 0 {
						ly = int(cy - 1)
						lx = int(cx + 1)
					}
					for j, ch := range conn.DstLabel.Label {
						aa.canvas[ly][lx+j+2] = string(ch)
					}
				}
			} else if i == 1 && x == int(math.Round(ax)) && y == int(math.Round(ay)) && conn.SrcArrow != d2target.NoArrowhead {
				arrowKey := fmt.Sprintf("%d%d", geo.Sign(sx)*-1, geo.Sign(sy)*-1)
				aa.canvas[y][x] = arrows[arrowKey]
				if conn.SrcLabel != nil {
					ly := 0
					lx := 0
					if math.Abs(sx) > 0 {
						ly = int(cy - 1)
						if sx > 0 {
							lx = int(ax)
						} else {
							lx = int(ax) - 1 - len(conn.SrcLabel.Label)
						}
					} else if math.Abs(sy) > 0 {
						ly = int(cy - 1)
						lx = int(cx + 1)
					}
					for j, ch := range conn.SrcLabel.Label {
						aa.canvas[ly][lx+j] = string(ch)
					}
				}
			} else {
				overWrite := false
				if y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) && aa.canvas[y][x] != " " {
					overWrite = true
				}
				if sx == 0 {
					if overWrite && y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) && len(frmShapeBoundary.BR) > 1 && len(frmShapeBoundary.TL) > 1 && (y == frmShapeBoundary.BR[1] || y == frmShapeBoundary.TL[1]) && aa.canvas[y][x] == "─" {
						if sy > 0 {
							aa.canvas[y][x] = aa.chars["TDO"]
						} else {
							aa.canvas[y][x] = aa.chars["TUP"]
						}
					} else if overWrite && y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) && len(toShapeBoundary.BR) > 1 && len(toShapeBoundary.TL) > 1 && (y == toShapeBoundary.BR[1] || y == toShapeBoundary.TL[1]) && aa.canvas[y][x] == "─" {
						if sy > 0 {
							aa.canvas[y][x] = aa.chars["TUP"]
						} else {
							aa.canvas[y][x] = aa.chars["TDO"]
						}
					} else if overWrite && y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) && len(frmShapeBoundary.BR) > 1 && len(frmShapeBoundary.TL) > 1 && len(toShapeBoundary.BR) > 1 && len(toShapeBoundary.TL) > 1 && ((aa.canvas[y][x] == "_" && (y == frmShapeBoundary.BR[1] || y == toShapeBoundary.BR[1])) || (aa.canvas[y][x] == "‾" && (y == frmShapeBoundary.TL[1] || y == toShapeBoundary.TL[1]))) {
						// skip
					} else if y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) {
						aa.canvas[y][x] = aa.chars["VER"]
					}
				} else {
					if overWrite {
					}
					if overWrite && y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) && len(frmShapeBoundary.BR) > 0 && len(frmShapeBoundary.TL) > 0 && (x == frmShapeBoundary.BR[0]-1 || x == frmShapeBoundary.TL[0]-1) && aa.canvas[y][x] == "│" {
						if sx > 0 {
							aa.canvas[y][x] = aa.chars["TRI"]
						} else {
							aa.canvas[y][x] = aa.chars["TLE"]
						}
					} else if overWrite && y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) && len(toShapeBoundary.BR) > 0 && len(toShapeBoundary.TL) > 0 && (x == toShapeBoundary.BR[0]-1 || x == toShapeBoundary.TL[0]-1) && aa.canvas[y][x] == "│" {
						if sx > 0 {
							aa.canvas[y][x] = aa.chars["TLE"]
						} else {
							aa.canvas[y][x] = aa.chars["TRI"]
						}
					} else if y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) {
						aa.canvas[y][x] = aa.chars["HOR"]
					}
				}
			}
			if strings.Trim(_label, " ") != "" {
				// Determine best label position
				maxDiff := 0.0
				bestIndex := -1
				bestX := 0.0
				scaleOld := 0.0
				for i := 0; i < len(routes)-1; i++ {
					diffY := math.Abs(routes[i].Y - routes[i+1].Y)
					diffX := math.Abs(routes[i].X - routes[i+1].X)
					diff := math.Max(diffY, diffX)
					scale := (math.Abs(float64(geo.Sign(diffX)))*aa.FW + math.Abs(float64(geo.Sign(diffY)))*aa.FH)
					if diff*scale > maxDiff*scaleOld {
						maxDiff = diff
						bestIndex = i
						bestX = routes[i].X
						if diff == diffX {
							bestX = routes[i].X + (float64(geo.Sign(sx)) * diff / 2)
						}
					}
					scaleOld = scale
				}

				labelPos := struct {
					I int
					X int
					Y int
				}{
					I: bestIndex,
					X: int(math.Round(bestX)) - len(_label)/2,
					Y: int(math.Round(maxDiff / 2)),
				}
				if sy != 0 && labelPos.I == i-1 && int(math.Round(ay))+int(math.Round(maxDiff/2))*geo.Sign(sy) == y {
					if y >= 0 && y < len(aa.canvas) && x >= 0 && x < len(aa.canvas[y]) {
						aa.canvas[y][x] = " "
					}
					for j, ch := range _label {
						if y >= 0 && y < len(aa.canvas) && (labelPos.X+j) >= 0 && (labelPos.X+j) < len(aa.canvas[0]) {
							aa.canvas[y][labelPos.X+j] = string(ch)
						}
					}
				} else if sx != 0 && labelPos.I == i-1 && int(math.Round(ax))+int(math.Round(maxDiff/2))*geo.Sign(sx) == x {
					//aa.canvas[y][x] = " "
					yFactor := 0
					if strings.Contains(conn.LabelPosition, "TOP") {
						yFactor = -1
					} else if strings.Contains(conn.LabelPosition, "BOTTOM") {
						yFactor = 1
					}
					if strings.Contains(conn.LabelPosition, "LEFT") {
						labelPos.X = int(routes[labelPos.I+abs((geo.Sign(sx)-1)/2)].X)
					} else if strings.Contains(conn.LabelPosition, "RIGHT") {
						labelPos.X = int(routes[labelPos.I+((geo.Sign(sx)+1)/2)].X) - len(_label)/2
					}

					for j, ch := range _label {
						if (y+yFactor) >= 0 && (y+yFactor) < len(aa.canvas) && (labelPos.X+j) >= 0 && (labelPos.X+j) < len(aa.canvas[0]) {
							aa.canvas[y+yFactor][labelPos.X+j] = string(ch)
						}
					}
				}
			}
			fx += sx
			fy += sy
		}
	}
}
func mergeRoutes(routes []*geo.Point) []*geo.Point {
	mRoutes := []*geo.Point{routes[0]}
	var pt = routes[0]
	dir := geo.Sign(routes[0].X-routes[1].X)*1 + geo.Sign(routes[0].Y-routes[1].Y)*2
	for j := 1; j < len(routes); j++ {
		if dir != geo.Sign(pt.X-routes[j].X)*1+geo.Sign(pt.Y-routes[j].Y)*2 {
			mRoutes = append(mRoutes, pt)
			dir = geo.Sign(pt.X-routes[j].X)*1 + geo.Sign(pt.Y-routes[j].Y)*2
		}
		pt = routes[j]
	}
	if mRoutes[len(mRoutes)-1].X != pt.X || mRoutes[len(mRoutes)-1].Y != pt.Y {
		mRoutes = append(mRoutes, pt)
	}
	return mRoutes
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func sign(x int) int {
	if x == 0 {
		return 0
	}
	if x < 0 {
		return -1
	}
	return 1
}
func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
