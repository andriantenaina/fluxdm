package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// appIcon is generated at startup: a rounded blue gradient square with a
// white "download" arrow (no external asset needed).
var appIcon = makeIcon(256)

func makeIcon(n int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	fn := float64(n)
	r := fn * 0.22
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			// rounded rectangle mask with anti-aliasing
			dx := math.Max(math.Max(r-fx, fx-(fn-r)), 0)
			dy := math.Max(math.Max(r-fy, fy-(fn-r)), 0)
			d := math.Hypot(dx, dy) - r
			a := clamp(0.5 - d)
			if a <= 0 {
				continue
			}
			t := (fx + fy) / (2 * fn)
			c := color.RGBA{
				R: uint8(lerp(37, 14, t)),
				G: uint8(lerp(140, 84, t)),
				B: uint8(lerp(255, 200, t)),
				A: 255,
			}
			// arrow: shaft + head + base line
			cx := fn / 2
			white := 0.0
			if math.Abs(fx-cx) < fn*0.07 && fy > fn*0.2 && fy < fn*0.58 {
				white = 1
			}
			// head triangle
			hy := fy - fn*0.42
			if hy >= 0 && hy < fn*0.24 && math.Abs(fx-cx) < (fn*0.24-hy)*1.0 {
				white = 1
			}
			if fy > fn*0.74 && fy < fn*0.82 && fx > fn*0.24 && fx < fn*0.76 {
				white = 1
			}
			if white > 0 {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.SetRGBA(x, y, color.RGBA{c.R, c.G, c.B, uint8(255 * a)})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func clamp(v float64) float64      { return math.Min(1, math.Max(0, v)) }
func lerp(a, b, t float64) float64 { return a + (b-a)*t }
