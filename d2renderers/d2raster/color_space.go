package d2raster

import "math"

func srgbToLinear(value float64) float64 {
	value = math.Max(0, math.Min(1, value))
	if value <= 0.04045 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func linearToSRGB(value float64) float64 {
	value = math.Max(0, math.Min(1, value))
	if value <= 0.0031308 {
		return 12.92 * value
	}
	return 1.055*math.Pow(value, 1/2.4) - 0.055
}
