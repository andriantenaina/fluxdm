// Command genicons writes the FluxDM PNG icons used by the extension and installers.
//
//	go run ./tools/genicons <outdir> [sizes...]
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

func icon(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	fn := float64(n)
	r := fn * 0.22
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			dx := math.Max(math.Max(r-fx, fx-(fn-r)), 0)
			dy := math.Max(math.Max(r-fy, fy-(fn-r)), 0)
			d := math.Hypot(dx, dy) - r
			a := math.Min(1, math.Max(0, 0.5-d))
			if a <= 0 {
				continue
			}
			t := (fx + fy) / (2 * fn)
			c := color.RGBA{uint8(37 + (14-37)*t), uint8(140 + (84-140)*t), uint8(255 + (200-255)*t), 255}
			cx := fn / 2
			if (math.Abs(fx-cx) < fn*0.07 && fy > fn*0.2 && fy < fn*0.58) ||
				(fy-fn*0.42 >= 0 && fy-fn*0.42 < fn*0.24 && math.Abs(fx-cx) < fn*0.24-(fy-fn*0.42)) ||
				(fy > fn*0.74 && fy < fn*0.82 && fx > fn*0.24 && fx < fn*0.76) {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.SetRGBA(x, y, color.RGBA{c.R, c.G, c.B, uint8(255 * a)})
		}
	}
	return img
}

// writeICO writes a Windows .ico made of PNG images (valid since Windows Vista).
func writeICO(path string, sizes []int) {
	var imgs [][]byte
	for _, n := range sizes {
		var b bytes.Buffer
		_ = png.Encode(&b, icon(n))
		imgs = append(imgs, b.Bytes())
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, n := range sizes {
		dim := byte(n)
		if n >= 256 {
			dim = 0
		}
		out.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))
		_ = binary.Write(&out, binary.LittleEndian, uint16(32))
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(imgs[i])))
		_ = binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(imgs[i])
	}
	for _, b := range imgs {
		out.Write(b)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: genicons <outdir> [sizes...]")
		os.Exit(2)
	}
	out := os.Args[1]
	sizes := []int{16, 32, 48, 128}
	if len(os.Args) > 2 && os.Args[2] == "ico" {
		writeICO(filepath.Join(out, "fluxdm.ico"), []int{16, 24, 32, 48, 64, 128, 256})
		return
	}
	if len(os.Args) > 2 {
		sizes = nil
		for _, s := range os.Args[2:] {
			n, _ := strconv.Atoi(s)
			sizes = append(sizes, n)
		}
	}
	_ = os.MkdirAll(out, 0o755)
	for _, n := range sizes {
		f, err := os.Create(filepath.Join(out, fmt.Sprintf("icon-%d.png", n)))
		if err != nil {
			panic(err)
		}
		_ = png.Encode(f, icon(n))
		f.Close()
	}
}
