package svg

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"oss.terrastruct.com/d2/lib/geo"
)

type SvgPathContext struct {
	Path     []geo.Intersectable
	Commands []string
	Start    *geo.Point
	Current  *geo.Point
	TopLeft  *geo.Point
	ScaleX   float64
	ScaleY   float64
}

// TODO probably use math.Big
func chopPrecision(f float64) float64 {
	// bring down to float32 precision before rounding for consistency across architectures
	result := math.Round(float64(float32(f*10000)) / 10000)
	// Ensure negative zero becomes positive zero
	if result == 0 {
		return 0
	}
	return result
}

func NewSVGPathContext(tl *geo.Point, sx, sy float64) *SvgPathContext {
	return &SvgPathContext{TopLeft: tl.Copy(), ScaleX: sx, ScaleY: sy}
}

func (c *SvgPathContext) Relative(base *geo.Point, dx, dy float64) *geo.Point {
	return geo.NewPoint(chopPrecision(base.X+c.ScaleX*dx), chopPrecision(base.Y+c.ScaleY*dy))
}
func (c *SvgPathContext) Absolute(x, y float64) *geo.Point {
	return c.Relative(c.TopLeft, x, y)
}

func (c *SvgPathContext) StartAt(p *geo.Point) {
	c.Start = p
	c.Commands = append(c.Commands, fmt.Sprintf("M %v %v", p.X, p.Y))
	c.Current = p.Copy()
}

func (c *SvgPathContext) Z() {
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: c.Start.Copy()})
	c.Commands = append(c.Commands, "Z")
	c.Current = c.Start.Copy()
}

func (c *SvgPathContext) L(isLowerCase bool, x, y float64) {
	var endPoint *geo.Point
	if isLowerCase {
		endPoint = c.Relative(c.Current, x, y)
	} else {
		endPoint = c.Absolute(x, y)
	}
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: endPoint})
	c.Commands = append(c.Commands, fmt.Sprintf("L %v %v", endPoint.X, endPoint.Y))
	c.Current = endPoint.Copy()
}

func (c *SvgPathContext) C(isLowerCase bool, x1, y1, x2, y2, x3, y3 float64) {
	p := func(x, y float64) *geo.Point {
		if isLowerCase {
			return c.Relative(c.Current, x, y)
		}
		return c.Absolute(x, y)
	}
	points := []*geo.Point{c.Current.Copy(), p(x1, y1), p(x2, y2), p(x3, y3)}
	c.Path = append(c.Path, geo.NewBezierCurve(points))
	c.Commands = append(c.Commands, fmt.Sprintf(
		"C %v %v %v %v %v %v",
		points[1].X, points[1].Y,
		points[2].X, points[2].Y,
		points[3].X, points[3].Y,
	))
	c.Current = points[3].Copy()
}

func (c *SvgPathContext) H(isLowerCase bool, x float64) {
	var endPoint *geo.Point
	if isLowerCase {
		endPoint = c.Relative(c.Current, x, 0)
	} else {
		endPoint = c.Absolute(x, 0)
		endPoint.Y = c.Current.Y
	}
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: endPoint.Copy()})
	c.Commands = append(c.Commands, fmt.Sprintf("H %v", endPoint.X))
	c.Current = endPoint.Copy()
}

func (c *SvgPathContext) V(isLowerCase bool, y float64) {
	var endPoint *geo.Point
	if isLowerCase {
		endPoint = c.Relative(c.Current, 0, y)
	} else {
		endPoint = c.Absolute(0, y)
		endPoint.X = c.Current.X
	}
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: endPoint})
	c.Commands = append(c.Commands, fmt.Sprintf("V %v", endPoint.Y))
	c.Current = endPoint.Copy()
}

func (c *SvgPathContext) PathData() string {
	return strings.Join(c.Commands, " ")
}

func GetStrokeDashAttributes(strokeWidth, dashGapSize float64) (float64, float64) {
	// as the stroke width gets thicker, the dash gap gets smaller
	scale := math.Log10(-0.6*strokeWidth+10.6)*0.5 + 0.5
	scaledDashSize := strokeWidth * dashGapSize
	scaledGapSize := scale * scaledDashSize
	return scaledDashSize, scaledGapSize
}

// Given control points p1, p2, p3, p4, calculate the segment of this bezier curve from t0 -> t1 where {0 <= t0 < t1 <= 1}.
// Uses De Casteljau's algorithm, referenced: https://stackoverflow.com/questions/11703283/cubic-bezier-curve-segment/11704152#11704152
func BezierCurveSegment(p1, p2, p3, p4 *geo.Point, t0, t1 float64) (geo.Point, geo.Point, geo.Point, geo.Point) {
	u0, u1 := 1-t0, 1-t1

	q1 := geo.Point{
		X: (u0*u0*u0)*p1.X + (3*t0*u0*u0)*p2.X + (3*t0*t0*u0)*p3.X + t0*t0*t0*p4.X,
		Y: (u0*u0*u0)*p1.Y + (3*t0*u0*u0)*p2.Y + (3*t0*t0*u0)*p3.Y + t0*t0*t0*p4.Y,
	}
	q2 := geo.Point{
		X: (u0*u0*u1)*p1.X + (2*t0*u0*u1+u0*u0*t1)*p2.X + (t0*t0*u1+2*u0*t0*t1)*p3.X + t0*t0*t1*p4.X,
		Y: (u0*u0*u1)*p1.Y + (2*t0*u0*u1+u0*u0*t1)*p2.Y + (t0*t0*u1+2*u0*t0*t1)*p3.Y + t0*t0*t1*p4.Y,
	}
	q3 := geo.Point{
		X: (u0*u1*u1)*p1.X + (t0*u1*u1+2*u0*t1*u1)*p2.X + (2*t0*t1*u1+u0*t1*t1)*p3.X + t0*t1*t1*p4.X,
		Y: (u0*u1*u1)*p1.Y + (t0*u1*u1+2*u0*t1*u1)*p2.Y + (2*t0*t1*u1+u0*t1*t1)*p3.Y + t0*t1*t1*p4.Y,
	}
	q4 := geo.Point{
		X: (u1*u1*u1)*p1.X + (3*t1*u1*u1)*p2.X + (3*t1*t1*u1)*p3.X + t1*t1*t1*p4.X,
		Y: (u1*u1*u1)*p1.Y + (3*t1*u1*u1)*p2.Y + (3*t1*t1*u1)*p3.Y + t1*t1*t1*p4.Y,
	}

	return q1, q2, q3, q4
}

// Gets a certain line/curve's SVG path string. offsetIdx and pathData provides the points needed
func getSVGPathString(pathType string, offsetIdx int, pathData []string) (string, error) {
	switch pathType {
	case "M":
		return fmt.Sprintf("M %s %s ", pathData[offsetIdx+1], pathData[offsetIdx+2]), nil
	case "L":
		return fmt.Sprintf("L %s %s ", pathData[offsetIdx+1], pathData[offsetIdx+2]), nil
	case "C":
		return fmt.Sprintf("C %s %s %s %s %s %s ", pathData[offsetIdx+1], pathData[offsetIdx+2], pathData[offsetIdx+3], pathData[offsetIdx+4], pathData[offsetIdx+5], pathData[offsetIdx+6]), nil
	case "S":
		return fmt.Sprintf("S %s %s %s %s ", pathData[offsetIdx+1], pathData[offsetIdx+2], pathData[offsetIdx+3], pathData[offsetIdx+4]), nil
	default:
		return "", fmt.Errorf("unknown svg path command \"%s\"", pathData[offsetIdx])
	}
}

// Gets how much to increment by on an SVG string to get to the next path command
func getPathStringIncrement(pathType string) (int, error) {
	switch pathType {
	case "M":
		return 3, nil
	case "L":
		return 3, nil
	case "C":
		return 7, nil
	case "S":
		return 5, nil
	default:
		return 0, fmt.Errorf("unknown svg path command \"%s\"", pathType)
	}
}

// This function finds the length of a path in SVG notation
func pathLength(pathData []string) (float64, error) {
	var x, y, pathLength float64
	var prevPosition geo.Point
	var increment int

	for i := 0; i < len(pathData); i += increment {
		switch pathData[i] {
		case "M":
			x, _ = strconv.ParseFloat(pathData[i+1], 64)
			y, _ = strconv.ParseFloat(pathData[i+2], 64)
		case "L":
			x, _ = strconv.ParseFloat(pathData[i+1], 64)
			y, _ = strconv.ParseFloat(pathData[i+2], 64)

			pathLength += geo.EuclideanDistance(prevPosition.X, prevPosition.Y, x, y)
		case "C":
			x, _ = strconv.ParseFloat(pathData[i+5], 64)
			y, _ = strconv.ParseFloat(pathData[i+6], 64)

			pathLength += geo.EuclideanDistance(prevPosition.X, prevPosition.Y, x, y)
		case "S":
			x, _ = strconv.ParseFloat(pathData[i+3], 64)
			y, _ = strconv.ParseFloat(pathData[i+4], 64)

			pathLength += geo.EuclideanDistance(prevPosition.X, prevPosition.Y, x, y)
		default:
			return 0, fmt.Errorf("unknown svg path command \"%s\"", pathData[i])
		}

		prevPosition = geo.Point{X: x, Y: y}

		incr, err := getPathStringIncrement(pathData[i])

		if err != nil {
			return 0, err
		}

		increment = incr
	}

	return pathLength, nil
}

// Splits an SVG path into two SVG paths, with the first path being ~{percentage}% of the path
func SplitPath(path string, percentage float64) (string, string, error) {
	var sumPathLens, curPathLen, x, y float64
	var prevPosition geo.Point
	var path1, path2 string
	var increment int

	pastHalf := false
	pathData := strings.Split(path, " ")
	pathLen, err := pathLength(pathData)

	if err != nil {
		return "", "", err
	}

	for i := 0; i < len(pathData); i += increment {
		switch pathData[i] {
		case "M":
			x, _ = strconv.ParseFloat(pathData[i+1], 64)
			y, _ = strconv.ParseFloat(pathData[i+2], 64)

			curPathLen = 0
		case "L":
			x, _ = strconv.ParseFloat(pathData[i+1], 64)
			y, _ = strconv.ParseFloat(pathData[i+2], 64)

			curPathLen = geo.EuclideanDistance(prevPosition.X, prevPosition.Y, x, y)
		case "C":
			x, _ = strconv.ParseFloat(pathData[i+5], 64)
			y, _ = strconv.ParseFloat(pathData[i+6], 64)

			curPathLen = geo.EuclideanDistance(prevPosition.X, prevPosition.Y, x, y)
		case "S":
			x, _ = strconv.ParseFloat(pathData[i+3], 64)
			y, _ = strconv.ParseFloat(pathData[i+4], 64)

			curPathLen = geo.EuclideanDistance(prevPosition.X, prevPosition.Y, x, y)
		default:
			return "", "", fmt.Errorf("unknown svg path command \"%s\"", pathData[i])
		}

		curPath, err := getSVGPathString(pathData[i], i, pathData)
		if err != nil {
			return "", "", err
		}

		sumPathLens += curPathLen

		if pastHalf { // add to path2
			path2 += curPath
		} else if sumPathLens < pathLen*percentage { // add to path1
			path1 += curPath
		} else { // transition from path1 -> path2
			t := (pathLen*percentage - sumPathLens + curPathLen) / curPathLen

			switch pathData[i] {
			case "M":
				path2 += fmt.Sprintf("M %s %s ", pathData[i+3], pathData[i+4])
			case "L":
				path1 += fmt.Sprintf("L %f %f ", (x-prevPosition.X)*t+prevPosition.X, (y-prevPosition.Y)*t+prevPosition.Y)
				path2 += fmt.Sprintf("M %f %f L %f %f ", (x-prevPosition.X)*t+prevPosition.X, (y-prevPosition.Y)*t+prevPosition.Y, x, y)
			case "C":
				h1x, _ := strconv.ParseFloat(pathData[i+1], 64)
				h1y, _ := strconv.ParseFloat(pathData[i+2], 64)
				h2x, _ := strconv.ParseFloat(pathData[i+3], 64)
				h2y, _ := strconv.ParseFloat(pathData[i+4], 64)

				heading1 := geo.Point{X: h1x, Y: h1y}
				heading2 := geo.Point{X: h2x, Y: h2y}
				nextPoint := geo.Point{X: x, Y: y}

				q1, q2, q3, q4 := BezierCurveSegment(&prevPosition, &heading1, &heading2, &nextPoint, 0, 0.5)
				path1 += fmt.Sprintf("C %f %f %f %f %f %f ", q2.X, q2.Y, q3.X, q3.Y, q4.X, q4.Y)

				q1, q2, q3, q4 = BezierCurveSegment(&prevPosition, &heading1, &heading2, &nextPoint, 0.5, 1)
				path2 += fmt.Sprintf("M %f %f C %f %f %f %f %f %f ", q1.X, q1.Y, q2.X, q2.Y, q3.X, q3.Y, q4.X, q4.Y)
			case "S":
				// Skip S curves because they are shorter and we can split along the connection to the next path instead
				path1 += fmt.Sprintf("S %s %s %s %s ", pathData[i+1], pathData[i+2], pathData[i+3], pathData[i+4])
				path2 += fmt.Sprintf("M %s %s ", pathData[i+3], pathData[i+4])
			default:
				return "", "", fmt.Errorf("unknown svg path command \"%s\"", pathData[i])
			}

			pastHalf = true
		}

		incr, err := getPathStringIncrement(pathData[i])

		if err != nil {
			return "", "", err
		}

		increment = incr
		prevPosition = geo.Point{X: x, Y: y}
	}

	return path1, path2, nil
}
