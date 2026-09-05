package d2raster

import "math"

// COLRv1 gradient stops use byte channels, so their sRGB decoding has only
// 256 possible inputs. Decode each value once instead of evaluating the power
// function for both endpoints of every painted pixel.
var linearSRGBByte = func() [256]float64 {
	var table [256]float64
	for value := range table {
		table[value] = srgbToLinear(float64(value) / 255)
	}
	return table
}()

func srgbToLinear(value float64) float64 {
	value = math.Max(0, math.Min(1, value))
	if value <= 0.04045 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func premultipliedSRGBByteToLinear(channel, alpha uint8) float64 {
	if alpha == 0 {
		return 0
	}
	if alpha == 0xff {
		return linearSRGBByte[channel]
	}
	return srgbToLinear(float64(channel) / float64(alpha))
}

func linearToSRGB(value float64) float64 {
	value = math.Max(0, math.Min(1, value))
	if value <= 0.0031308 {
		return 12.92 * value
	}
	return 1.055*math.Pow(value, 1/2.4) - 0.055
}
