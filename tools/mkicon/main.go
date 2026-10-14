// mkicon draws the Velo icon (a white lightning bolt on a rounded blue
// square) at every size Windows and the browser extension need, and writes
// the .ico and .png files. Run from the repository root:
//
//	go run ./tools/mkicon
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/vector"
)

var (
	top    = color.NRGBA{0x2B, 0x86, 0xFF, 0xFF} // gradient, top-left
	bottom = color.NRGBA{0x0E, 0x5B, 0xE0, 0xFF} // gradient, bottom-right
)

// bolt is the lightning bolt in a 24x24 design grid (same shape as the UI logo).
var bolt = [][2]float32{{13.5, 2}, {4, 13.5}, {10.5, 13.5}, {9, 22}, {20, 9.5}, {13.2, 9.5}}

func render(size int) *image.NRGBA {
	s := float32(size)
	// Rounded square with a small margin at large sizes (like other Windows app icons).
	margin := float32(0)
	if size >= 48 {
		margin = s * 0.04
	}
	radius := (s - 2*margin) * 0.24

	mask := vector.NewRasterizer(size, size)
	roundRect(mask, margin, margin, s-margin, s-margin, radius)
	bg := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			t := (float64(x) + float64(y)) / float64(2*size-2)
			bg.SetNRGBA(x, y, lerp(top, bottom, t))
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	mask.Draw(out, out.Bounds(), bg, image.Point{})

	// The bolt fills about 62% of the tile, nudged to look optically centred.
	fill := float32(0.62)
	if size <= 24 {
		fill = 0.74 // bigger bolt so it stays legible at tray sizes
	}
	scale := (s - 2*margin) * fill / 20
	ox := s/2 - 12*scale
	oy := s/2 - 12*scale
	b := vector.NewRasterizer(size, size)
	b.MoveTo(ox+bolt[0][0]*scale, oy+bolt[0][1]*scale)
	for _, p := range bolt[1:] {
		b.LineTo(ox+p[0]*scale, oy+p[1]*scale)
	}
	b.ClosePath()
	b.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{})
	return out
}

func roundRect(r *vector.Rasterizer, x0, y0, x1, y1, rad float32) {
	const k = 0.5523 // cubic Bézier circle approximation
	r.MoveTo(x0+rad, y0)
	r.LineTo(x1-rad, y0)
	r.CubeTo(x1-rad+rad*k, y0, x1, y0+rad-rad*k, x1, y0+rad)
	r.LineTo(x1, y1-rad)
	r.CubeTo(x1, y1-rad+rad*k, x1-rad+rad*k, y1, x1-rad, y1)
	r.LineTo(x0+rad, y1)
	r.CubeTo(x0+rad-rad*k, y1, x0, y1-rad+rad*k, x0, y1-rad)
	r.LineTo(x0, y0+rad)
	r.CubeTo(x0, y0+rad-rad*k, x0+rad-rad*k, y0, x0+rad, y0)
	r.ClosePath()
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	t = math.Max(0, math.Min(1, t))
	m := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.NRGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 255}
}

func pngBytes(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

// writeICO packs PNG images into one .ico (PNG entries work on Windows Vista+).
func writeICO(path string, sizes []int) {
	var imgs [][]byte
	for _, s := range sizes {
		imgs = append(imgs, pngBytes(render(s)))
	}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0 // 0 means 256
		}
		buf.Write([]byte{dim, dim, 0, 0})
		binary.Write(&buf, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&buf, binary.LittleEndian, []uint32{uint32(len(imgs[i])), uint32(offset)})
		offset += len(imgs[i])
	}
	for _, b := range imgs {
		buf.Write(b)
	}
	write(path, buf.Bytes())
}

func write(path string, b []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d bytes)", path, len(b))
}

func main() {
	all := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	writeICO("cmd/velo/assets/velo.ico", all)
	write("cmd/velo/assets/velo-32.png", pngBytes(render(32)))
	for _, s := range []int{16, 32, 48, 128} {
		write(filepath.Join("extension", "icons", "icon"+itoa(s)+".png"), pngBytes(render(s)))
	}
	write("docs/velo-256.png", pngBytes(render(256)))

	// A preview sheet of every size, for checking by eye.
	sheet := image.NewNRGBA(image.Rect(0, 0, 900, 300))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.NRGBA{0xF5, 0xF8, 0xFC, 255}), image.Point{}, draw.Src)
	x := 16
	for _, s := range all {
		img := render(s)
		draw.Draw(sheet, image.Rect(x, 150-s/2, x+s, 150+s/2), img, image.Point{}, draw.Over)
		x += s + 16
	}
	write(filepath.Join(os.TempDir(), "velo-icon-preview.png"), pngBytes(sheet))
}

func itoa(n int) string {
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
