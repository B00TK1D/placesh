package main

import (
	"fmt"
	"math"
)

func hexToRGB(hex string) (uint8, uint8, uint8) {
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

func rgbToAnsi(r uint8, g uint8, b uint8) uint8 {

	// Map each channel to 0–5 range
	toCube := func(v uint8) int {
		steps := []uint8{0, 95, 135, 175, 215, 255}
		idx := 0
		minDist := 999
		for i, step := range steps {
			dist := int(math.Abs(float64(v - step)))
			if dist < minDist {
				minDist = dist
				idx = i
			}
		}
		return idx
	}

	// Try cube first
	ri := toCube(r)
	gi := toCube(g)
	bi := toCube(b)
	var cubeIndex uint8 = uint8(16 + (36 * ri) + (6 * gi) + bi)

	// Try grayscale
	var grayIndex uint8 = 0
	if r == g && g == b {
		// Special case: if it's pure gray
		grayLevel := r
		if grayLevel < 8 {
			grayIndex = 16
		} else if grayLevel > 248 {
			grayIndex = 231
		} else {
			grayIndex = uint8(math.Round(float64((grayLevel-8))/10.7)) + 232
		}
	}

	// Pick whichever is closer
	cubeR := []uint8{0, 95, 135, 175, 215, 255}[ri]
	cubeG := []uint8{0, 95, 135, 175, 215, 255}[gi]
	cubeB := []uint8{0, 95, 135, 175, 215, 255}[bi]

	cubeDist := math.Sqrt(float64((r-cubeR)*(r-cubeR) + (g-cubeG)*(g-cubeG) + (b-cubeB)*(b-cubeB)))
	grayDist := math.Sqrt(float64((r-g)*(r-g) + (g-b)*(g-b) + (b-r)*(b-r))) // difference between channels

	if grayIndex != 0 && grayDist < cubeDist {
		return grayIndex
	}

	return cubeIndex
}
