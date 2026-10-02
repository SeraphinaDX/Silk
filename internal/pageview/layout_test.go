// SPDX-License-Identifier: GPL-3.0-or-later

package pageview

import (
	"strings"
	"testing"
)

func TestReflowAndHit(t *testing.T) {
	d := Document{Tokens: []Token{{Kind: "text", Text: "hello world", Action: "a"}, {Kind: "break"}, {Kind: "text", Text: "界 café"}}}
	l := Build(d, Options{Cols: 6})
	if len(l.Lines) != 4 {
		t.Fatalf("lines: %+v", l.Lines)
	}
	if l.Hit(0, 1) != "a" || l.Hit(1, 1) != "a" || l.Hit(2, 0) != "" {
		t.Fatal("bad hits")
	}
	for _, line := range l.Lines {
		for _, s := range line.Spans {
			if s.End > 6 {
				t.Fatal("line overflow")
			}
		}
	}
}
func TestBoundedInlineImagesAndPre(t *testing.T) {
	d := Document{Tokens: []Token{{Kind: "text", Text: "above"}, {Kind: "image", ID: "img", Width: 800, Height: 400, Text: "photo", Action: "link"}, {Kind: "text", Text: "below"}, {Kind: "break"}, {Kind: "text", Text: "a\n\nb", Style: "pre"}}}
	l := Build(d, Options{Cols: 40, CellWidth: 8, CellHeight: 16, ImageRows: 5, Images: true, Zoom: 1})
	if len(l.Pictures) != 1 {
		t.Fatal(l)
	}
	p := l.Pictures[0]
	if p.Width != 160 || p.Height != 80 || p.Rows != 5 || l.Hit(p.Row+2, 3) != "link" {
		t.Fatal(p)
	}
	text := ""
	for _, line := range l.Lines {
		for _, s := range line.Spans {
			text += s.Text
		}
		text += "\n"
	}
	if !strings.Contains(text, "below\na\n\nb") {
		t.Fatal(text)
	}
	plain := Build(d, Options{Cols: 40, Images: false})
	if len(plain.Pictures) != 0 {
		t.Fatal("image slots with images disabled")
	}
}
