package d2cli

import (
	"bytes"
	"math"
	"math/rand"
	"testing"
)

// Compare the actual scores and scratch writes against the pre-optimization
// scalar filters, including early termination and runs crossing pixel boundaries.
func TestRasterPNGFilterRunsMatchScalar(t *testing.T) {
	random := rand.New(rand.NewSource(2879))
	for _, channels := range []int{3, 4} {
		for _, width := range []int{1, 2, 5, 6, 7, 8, 10, 15, 16, 17, 31, 32, 33, 63, 64, 65, 257} {
			for pattern := 0; pattern < 9; pattern++ {
				raw, prior := make([]byte, width*channels), make([]byte, width*channels)
				random.Read(raw)
				random.Read(prior)
				switch pattern {
				case 1:
					copy(raw, prior)
				case 2, 3:
					for i := range raw {
						raw[i] = byte(i%channels*53 + 1)
						prior[i] = raw[i]
						if pattern == 3 {
							prior[i] += 19
						}
					}
				case 4:
					copy(raw, prior)
					for i := channels - 1; i < len(raw); i += 16 {
						raw[i] ^= 0x81
					}
				case 5:
					for i := range raw {
						raw[i] = 255
						prior[i] = 255
						if i%32 == 15 {
							raw[i] = byte(i)
						}
					}
				case 6:
					for i := 0; i < min(64, len(raw)); i++ {
						raw[i] = 255
						prior[i] = 255
					}
				case 7:
					for i := 16; i+16 <= len(raw); i += 32 {
						copy(raw[i:i+16], prior[i:i+16])
					}
				case 8:
					clear(raw)
					clear(prior)
				}
				thresholds := []int{0, 1, 2, 7, 255, 65535, math.MaxInt}
				scratch := make([]byte, len(raw)+1)
				for _, fullCost := range []int{
					referenceRasterPNGUpFilter(scratch, raw, prior),
					referenceRasterPNGSubFilter(scratch, raw, math.MaxInt, channels),
					referenceRasterPNGPaethFilter(scratch, raw, prior, math.MaxInt, channels),
				} {
					thresholds = append(thresholds, max(0, fullCost-1), fullCost, fullCost+1)
				}
				for _, stopAt := range thresholds {
					for filter := 0; filter < 3; filter++ {
						got, want := bytes.Repeat([]byte{0xa5}, len(raw)+1), bytes.Repeat([]byte{0xa5}, len(raw)+1)
						var gotCost, wantCost int
						switch filter {
						case 0:
							gotCost = rasterPNGUpFilter(got, raw, prior)
							wantCost = referenceRasterPNGUpFilter(want, raw, prior)
						case 1:
							gotCost = rasterPNGSubFilter(got, raw, stopAt, channels)
							wantCost = referenceRasterPNGSubFilter(want, raw, stopAt, channels)
						case 2:
							gotCost = rasterPNGPaethFilter(got, raw, prior, stopAt, channels)
							wantCost = referenceRasterPNGPaethFilter(want, raw, prior, stopAt, channels)
						}
						if gotCost != wantCost || !bytes.Equal(got, want) {
							t.Fatalf("channels=%d width=%d pattern=%d stop=%d filter=%d: cost %d != %d or scratch differs", channels, width, pattern, stopAt, filter, gotCost, wantCost)
						}
					}
				}
			}
		}
	}
}

// Frozen scalar implementations from 70affc0e.
func referenceRasterPNGUpFilter(destination, raw, prior []byte) int {
	_ = prior[len(raw)-1]
	destination[0] = 2
	destination = destination[1:]
	cost := 0
	for index, value := range raw {
		destination[index] = value - prior[index]
		cost += rasterPNGAbs8(destination[index])
	}
	return cost
}

func referenceRasterPNGSubFilter(destination, raw []byte, stopAt, channels int) int {
	destination[0] = 1
	destination = destination[1:]
	cost := 0
	for index := 0; index < channels; index++ {
		destination[index] = raw[index]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := channels; index < len(raw); index++ {
		destination[index] = raw[index] - raw[index-channels]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			break
		}
	}
	return cost
}

func referenceRasterPNGPaethFilter(destination, raw, prior []byte, stopAt, channels int) int {
	_ = prior[len(raw)-1]
	destination[0] = 4
	destination = destination[1:]
	cost := 0
	for index := 0; index < channels; index++ {
		destination[index] = raw[index] - prior[index]
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			return cost
		}
	}
	for index := channels; index < len(raw); index++ {
		left := raw[index-channels]
		upperLeft := prior[index-channels]
		upper := prior[index]
		leftDistance := rasterPNGByteDistance(upper, upperLeft)
		upperDistance := rasterPNGByteDistance(left, upperLeft)
		diagonalDistance := int(left) + int(upper) - 2*int(upperLeft)
		if diagonalDistance < 0 {
			diagonalDistance = -diagonalDistance
		}
		predictor := upperLeft
		if leftDistance <= upperDistance && leftDistance <= diagonalDistance {
			predictor = left
		} else if upperDistance <= diagonalDistance {
			predictor = upper
		}
		destination[index] = raw[index] - predictor
		cost += rasterPNGAbs8(destination[index])
		if cost >= stopAt {
			break
		}
	}
	return cost
}
