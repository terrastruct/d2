package geo

import (
	"math"
)

// A N-Dimensional Vector with components (x, y, z, ...) based on the origin
type Vector []float64

// New Vector from components
func NewVector(components ...float64) Vector {
	return components
}

// New Vector of length and pointing in the direction of angle
func NewVectorFromProperties(length float64, angleInRadians float64) Vector {
	return NewVector(
		length*math.Sin(angleInRadians),
		length*math.Cos(angleInRadians),
	)
}

// Creates a Vector by extending the length of the current one by length
func (a Vector) AddLength(length float64) Vector {
	return a.Unit().Multiply(a.Length() + length)
}

func (a Vector) Add(b Vector) Vector {
	c := []float64{}
	for i := 0; i < len(a); i++ {
		c = append(c, a[i]+b[i])
	}
	return c
}

func (a Vector) Minus(b Vector) Vector {
	c := []float64{}
	for i := 0; i < len(a); i++ {
		c = append(c, a[i]-b[i])
	}
	return c
}

func (a Vector) Multiply(v float64) Vector {
	c := []float64{}
	for i := 0; i < len(a); i++ {
		c = append(c, a[i]*v)
	}
	return c
}

func (a Vector) Length() float64 {
	sum := 0.0
	for _, comp := range a {
		sum += comp * comp
	}
	return math.Sqrt(sum)
}

// Creates an unit Vector pointing in the same direction of this Vector
func (a Vector) Unit() Vector {
	return a.Multiply(1 / a.Length())
}

func (a Vector) ToPoint() *Point {
	return &Point{a[0], a[1]}
}

// return the line (x1,y1) -> (x2,y2) rotated 90% counter-clockwise (left)
func getNormalVector(x1, y1, x2, y2 float64) (float64, float64) {
	return y1 - y2, x2 - x1
}

func GetUnitNormalVector(x1, y1, x2, y2 float64) (float64, float64) {
	normalX, normalY := getNormalVector(x1, y1, x2, y2)
	length := EuclideanDistance(x1, y1, x2, y2)
	return normalX / length, normalY / length
}
