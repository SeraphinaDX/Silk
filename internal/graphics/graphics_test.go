// SPDX-License-Identifier: GPL-3.0-or-later

package graphics

import (
	"bytes"
	"image"
	"image/color"
	"strconv"
	"strings"
	"testing"
)

// Decode the emitted protocol independently to detect misplaced bands, empty
// runs and errors in the final partial six-pixel row.
func TestSixelPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(5, 7, 42, 20))
	for y := 7; y < 20; y++ {
		for x := 5; x < 42; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x < 12 {
				c = color.RGBA{255, 0, 0, 255}
			}
			if y >= 13 && x > 30 {
				c = color.RGBA{0, 0, 255, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	encoded := Sixel(img)
	if !bytes.HasPrefix(encoded, []byte("\x1bP0;1;0q\"1;1;37;13")) || !bytes.HasSuffix(encoded, []byte("\x1b\\")) {
		t.Fatal("invalid DCS frame")
	}
	s := string(encoded)
	s = s[strings.Index(s, "q")+1 : len(s)-2]
	result := image.NewRGBA(image.Rect(0, 0, 37, 13))
	var palette [256]color.RGBA
	x, y, index := 0, 0, 0
	for i := 0; i < len(s); {
		readNumber := func() int {
			start := i
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			n, _ := strconv.Atoi(s[start:i])
			return n
		}
		switch s[i] {
		case '"':
			i++
			for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == ';') {
				i++
			}
		case '#':
			i++
			index = readNumber()
			if i < len(s) && s[i] == ';' {
				var vals []int
				for i < len(s) && s[i] == ';' {
					i++
					vals = append(vals, readNumber())
				}
				if len(vals) != 4 || vals[0] != 2 {
					t.Fatal("invalid RGB palette")
				}
				palette[index] = color.RGBA{uint8(vals[1] * 255 / 100), uint8(vals[2] * 255 / 100), uint8(vals[3] * 255 / 100), 255}
			}
		case '$':
			x = 0
			i++
		case '-':
			x = 0
			y += 6
			i++
		default:
			repeat := 1
			if s[i] == '!' {
				i++
				repeat = readNumber()
			}
			if i >= len(s) || s[i] < '?' || s[i] > '~' {
				t.Fatal("invalid sixel data")
			}
			bits := s[i] - 63
			i++
			for j := 0; j < repeat; j++ {
				for dy := 0; dy < 6; dy++ {
					if bits&(1<<dy) != 0 {
						result.SetRGBA(x, y+dy, palette[index])
					}
				}
				x++
			}
		}
	}
	for yy := 0; yy < 13; yy++ {
		for xx := 0; xx < 37; xx++ {
			if got, want := result.RGBAAt(xx, yy), img.RGBAAt(xx+5, yy+7); got != want {
				t.Fatalf("pixel (%d,%d): %v want %v", xx, yy, got, want)
			}
		}
	}
}
func TestSixelRunsAndHalfblock(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 100; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	if !bytes.Contains(Sixel(img), []byte("!100~")) {
		t.Fatal("solid band was not run-length encoded")
	}
	s := string(Halfblock(img, 5, 2))
	if strings.Count(s, "▀") != 10 || !strings.Contains(s, "\x1b[3;1H") || !strings.Contains(s, "\x1b[4;1H") {
		t.Fatal("fallback geometry")
	}
}
