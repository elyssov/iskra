// gen_icons.go generates Android launcher icons for Iskra.
// Draws a stylized flame shape (indigo #6366F1) on a dark background (#0F0F14).
// Uses only Go standard library.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

var (
	bgColor    = color.RGBA{0x0F, 0x0F, 0x14, 0xFF}
	flameColor = color.RGBA{0x63, 0x66, 0xF1, 0xFF}
	glowColor  = color.RGBA{0x81, 0x84, 0xF5, 0xFF}
)

// sizes maps density bucket to pixel size.
var sizes = []struct {
	size int
	dir  string
}{
	{48, "mipmap-mdpi"},
	{72, "mipmap-hdpi"},
	{96, "mipmap-xhdpi"},
	{144, "mipmap-xxhdpi"},
	{192, "mipmap-xxxhdpi"},
}

func main() {
	base := filepath.Join("android", "app", "src", "main", "res")

	for _, s := range sizes {
		dir := filepath.Join(base, s.dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", dir, err)
			os.Exit(1)
		}

		img := generateIcon(s.size)

		path := filepath.Join(dir, "ic_launcher.png")
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create %s: %v\n", path, err)
			os.Exit(1)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "encode %s: %v\n", path, err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("wrote %s (%dx%d)\n", path, s.size, s.size)
	}
}

// generateIcon creates a square icon of the given size with a flame shape.
func generateIcon(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	sz := float64(size)
	cx, cy := sz/2, sz/2

	// Fill background with rounded-rect (circular) mask
	cornerR := sz * 0.18
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if inRoundedRect(float64(x), float64(y), sz, sz, cornerR) {
				img.SetRGBA(x, y, bgColor)
			}
		}
	}

	// Draw flame shape using parametric curves.
	// The flame is composed of a main teardrop and a smaller inner highlight.
	// We sample points and check if (x,y) is inside the flame.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !inRoundedRect(float64(x), float64(y), sz, sz, cornerR) {
				continue
			}
			// Normalize to [-1, 1] centered
			nx := (float64(x) - cx) / (sz * 0.4)
			ny := (float64(y) - cy) / (sz * 0.4)

			if inFlame(nx, ny) {
				// Inner glow near the center bottom
				if inInnerFlame(nx, ny) {
					img.SetRGBA(x, y, glowColor)
				} else {
					img.SetRGBA(x, y, flameColor)
				}
			}
		}
	}

	return img
}

// inFlame checks if normalized coordinates (x,y) are inside a flame shape.
// The flame is pointed at top, wide at bottom, with a slight S-curve.
func inFlame(x, y float64) bool {
	// Flame occupies roughly y in [-1.1, 0.9] (top to bottom)
	// Width narrows toward the top.

	// Shift so flame center-bottom is at (0, 0.8)
	y = y - 0.0

	// Outer flame boundary: teardrop shape
	// At a given height y, the half-width of the flame:
	// Bottom (y=0.8): wide ~0.55
	// Middle (y=0): medium ~0.5
	// Top (y=-1.0): zero (tip)

	if y > 0.85 || y < -1.05 {
		return false
	}

	// Map y to [0,1] where 0=tip, 1=bottom
	t := (y + 1.05) / 1.9
	if t < 0 || t > 1 {
		return false
	}

	// Half-width as function of t (0=tip, 1=base)
	// Use sqrt for a teardrop look
	halfW := 0.55 * math.Pow(t, 0.6)

	// Add a slight leftward lean at mid-height for flame character
	lean := 0.08 * math.Sin(t*math.Pi)
	cx := lean

	dist := math.Abs(x - cx)

	return dist < halfW
}

// inInnerFlame draws a smaller, brighter inner flame.
func inInnerFlame(x, y float64) bool {
	y = y + 0.15

	if y > 0.75 || y < -0.5 {
		return false
	}

	t := (y + 0.5) / 1.25
	if t < 0 || t > 1 {
		return false
	}

	halfW := 0.22 * math.Pow(t, 0.7)
	lean := 0.04 * math.Sin(t*math.Pi)
	cx := lean

	dist := math.Abs(x - cx)
	return dist < halfW
}

// inRoundedRect checks if point is inside a rounded rectangle.
func inRoundedRect(px, py, w, h, r float64) bool {
	// Corners at (r,r), (w-r,r), (r,h-r), (w-r,h-r)
	if px < 0 || py < 0 || px >= w || py >= h {
		return false
	}

	// Check if in corner regions
	corners := [][2]float64{
		{r, r},
		{w - r, r},
		{r, h - r},
		{w - r, h - r},
	}

	for _, c := range corners {
		// Determine which quadrant
		inCornerX := (px < r && c[0] == r) || (px > w-r && c[0] == w-r)
		inCornerY := (py < r && c[1] == r) || (py > h-r && c[1] == h-r)
		if inCornerX && inCornerY {
			dx := px - c[0]
			dy := py - c[1]
			if dx*dx+dy*dy > r*r {
				return false
			}
		}
	}

	return true
}
