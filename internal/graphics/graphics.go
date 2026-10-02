// SPDX-License-Identifier: GPL-3.0-or-later

// Package graphics converts browser screenshots into terminal graphics.
package graphics

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
)

// Resize fits a captured image to its terminal slot, using bilinear sampling.
// Native page text is never passed to this function or either image encoder.
func Resize(img image.Image, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	b := img.Bounds()
	for y := 0; y < h; y++ {
		fy := (float64(y)+.5)*float64(b.Dy())/float64(h) - .5
		y0 := max(0, int(fy))
		y1 := min(b.Dy()-1, y0+1)
		ty := max(0, fy-float64(y0))
		for x := 0; x < w; x++ {
			fx := (float64(x)+.5)*float64(b.Dx())/float64(w) - .5
			x0 := max(0, int(fx))
			x1 := min(b.Dx()-1, x0+1)
			tx := max(0, fx-float64(x0))
			r0, g0, b0, _ := img.At(b.Min.X+x0, b.Min.Y+y0).RGBA()
			r1, g1, b1, _ := img.At(b.Min.X+x1, b.Min.Y+y0).RGBA()
			r2, g2, b2, _ := img.At(b.Min.X+x0, b.Min.Y+y1).RGBA()
			r3, g3, b3, _ := img.At(b.Min.X+x1, b.Min.Y+y1).RGBA()
			mix := func(a, b, c, d uint32) uint8 {
				return uint8(((float64(a)*(1-tx)+float64(b)*tx)*(1-ty) + (float64(c)*(1-tx)+float64(d)*tx)*ty) / 257)
			}
			out.SetRGBA(x, y, color.RGBA{mix(r0, r1, r2, r3), mix(g0, g1, g2, g3), mix(b0, b1, b2, b3), 255})
		}
	}
	return out
}

// Sixel uses an opaque, fixed 256-colour 3:3:2 palette. Each six-pixel band
// contains one bit-plane per used colour, with run-length encoded empty areas.
// This intentionally avoids external image converters and native libraries.
func Sixel(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var out bytes.Buffer
	fmt.Fprintf(&out, "\x1bP0;1;0q\"1;1;%d;%d", w, h)
	for c := 0; c < 256; c++ {
		fmt.Fprintf(&out, "#%d;2;%d;%d;%d", c, ((c>>5)&7)*100/7, ((c>>2)&7)*100/7, (c&3)*100/3)
	}
	pixels := make([]byte, w*6)
	planes := make([]byte, w)
	for y := 0; y < h; y += 6 {
		var used [256]bool
		band := min(6, h-y)
		for dy := 0; dy < band; dy++ {
			for x := 0; x < w; x++ {
				r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y+dy).RGBA()
				c := byte(((r >> 13) << 5) | ((g >> 13) << 2) | (bl >> 14))
				pixels[dy*w+x] = c
				used[c] = true
			}
		}
		first := true
		for c := 0; c < 256; c++ {
			if !used[c] {
				continue
			}
			if !first {
				out.WriteByte('$')
			}
			first = false
			fmt.Fprintf(&out, "#%d", c)
			last := -1
			for x := 0; x < w; x++ {
				var bits byte
				for dy := 0; dy < band; dy++ {
					if pixels[dy*w+x] == byte(c) {
						bits |= 1 << dy
					}
				}
				planes[x] = bits + 63
				if bits != 0 {
					last = x
				}
			}
			for x := 0; x <= last; {
				end := x + 1
				for end <= last && planes[end] == planes[x] {
					end++
				}
				if end-x >= 4 {
					fmt.Fprintf(&out, "!%d%c", end-x, planes[x])
				} else {
					for j := x; j < end; j++ {
						out.WriteByte(planes[x])
					}
				}
				x = end
			}
		}
		if y+6 < h {
			out.WriteByte('-')
		}
	}
	out.WriteString("\x1b\\")
	return out.Bytes()
}

// Halfblock is an image-only truecolour fallback: two sampled pixels per cell.
// Page text always uses native terminal characters, regardless of image mode.
func Halfblock(img image.Image, cols, rows int) []byte {
	return HalfblockAt(img, cols, rows, 3)
}
func HalfblockAt(img image.Image, cols, rows, startRow int) []byte {
	b := img.Bounds()
	var out strings.Builder
	for y := 0; y < rows; y++ {
		fmt.Fprintf(&out, "\x1b[%d;1H", y+startRow)
		for x := 0; x < cols; x++ {
			sx := b.Min.X + (2*x+1)*b.Dx()/(2*cols)
			sy := b.Min.Y + (4*y+1)*b.Dy()/(4*rows)
			sy2 := b.Min.Y + (4*y+3)*b.Dy()/(4*rows)
			r, g, bl, _ := img.At(sx, sy).RGBA()
			r2, g2, b2, _ := img.At(sx, sy2).RGBA()
			fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r>>8, g>>8, bl>>8, r2>>8, g2>>8, b2>>8)
		}
	}
	out.WriteString("\x1b[0m")
	return []byte(out.String())
}
