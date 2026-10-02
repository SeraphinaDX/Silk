// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/SeraphinaDX/Silk/internal/graphics"
	"github.com/SeraphinaDX/Silk/internal/terminal"
)

type chromeState struct {
	url, title, status, edit string
	editing                  bool
	cols, rows               int
}
type paintedPicture struct {
	key                     string
	row, end, y0, y1, width int
}
type paintState struct {
	valid       bool
	cols, rows  int
	mode, epoch string
	lines       []string
	slots       []paintedPicture
	hashes      map[paintedPicture][32]byte
	chrome      chromeState
	chromeValid bool
}

func (a *app) chrome() {
	status := a.status
	if a.config.Graphics == "auto" && a.mode == "none" {
		status = "Images: alt text (no sixel reply); try -graphics=halfblock | " + status
	}
	state := chromeState{a.url, a.title, status, a.edit, a.editing, a.screen.Cols, a.screen.Rows}
	if a.paint.chromeValid && a.paint.chrome == state {
		return
	}
	a.screen.Chrome(a.url, a.title, status, a.edit, a.editing)
	a.paint.chrome, a.paint.chromeValid = state, true
}

// Paint changed text rows and changed pictures independently. Motion reports,
// link selection and status updates must not clear or retransmit page graphics.
func (a *app) render() {
	s := a.screen
	slots := []paintedPicture{}
	if a.mode != "none" {
		for _, p := range a.layout.Pictures {
			start, end := max(p.Row, a.offset), min(p.Row+p.Rows, a.offset+s.Rows-3)
			if start >= end {
				continue
			}
			slots = append(slots, paintedPicture{pictureKey(p), start - a.offset + 3, end - a.offset + 3, (start - p.Row) * s.CellHeight, min(p.Height, (end-p.Row)*s.CellHeight), p.Width})
		}
	}
	old := &a.paint
	clear := !old.valid || old.cols != s.Cols || old.rows != s.Rows || old.mode != a.mode || old.epoch != a.doc.Epoch || len(old.slots) != len(slots)
	if !clear {
		for i, p := range slots {
			if old.slots[i] != p {
				clear = true
				break
			}
		}
	}
	wrote := clear
	if clear {
		io.WriteString(s.Out, "\x1b[?25l\x1b[0m\x1b[2J")
		old.lines = nil
		old.hashes = map[paintedPicture][32]byte{}
		old.chromeValid = false
	}
	lines := make([]string, s.Rows-3)
	for row := range lines {
		var text strings.Builder
		index := a.offset + row
		if index < len(a.layout.Lines) {
			for _, span := range a.layout.Lines[index].Spans {
				style := "\x1b[0m\x1b[38;2;239;228;243m"
				switch span.Style {
				case "heading", "bold":
					style += "\x1b[1m"
				case "button", "input":
					style += "\x1b[38;2;255;186;221m"
				case "image", "muted":
					style += "\x1b[38;2;161;146;174m"
				}
				if span.Action != "" {
					style += "\x1b[4m\x1b[38;2;154;209;255m"
				}
				if span.Action != "" && span.Action == a.selected {
					style += "\x1b[7m"
				}
				text.WriteString(style + terminal.Clean(span.Text))
			}
		}
		lines[row] = text.String()
		if !clear && row < len(old.lines) && old.lines[row] == lines[row] {
			continue
		}
		fmt.Fprintf(s.Out, "\x1b[%d;1H\x1b[0m\x1b[2K%s", row+3, lines[row])
		wrote = true
	}
	for _, p := range slots {
		asset := a.assets[p.key]
		if asset.picture == nil {
			continue
		}
		hash, exists := old.hashes[p]
		if exists && hash == asset.hash {
			continue
		}
		img := asset.picture
		y1 := min(p.y1, img.Bounds().Dy())
		if p.y0 >= y1 {
			continue
		}
		crop := img.SubImage(image.Rect(0, p.y0, img.Bounds().Dx(), y1))
		fmt.Fprintf(s.Out, "\x1b[%d;1H\x1b[0m", p.row)
		if a.mode == "sixel" {
			s.Out.Write(graphics.Sixel(crop))
		} else {
			s.Out.Write(graphics.HalfblockAt(crop, (p.width+s.CellWidth-1)/s.CellWidth, p.end-p.row, p.row))
		}
		old.hashes[p] = asset.hash
		wrote = true
	}
	if wrote && a.editing {
		old.chromeValid = false
	}
	a.chrome()
	old.valid, old.cols, old.rows, old.mode, old.epoch = true, s.Cols, s.Rows, a.mode, a.doc.Epoch
	old.lines, old.slots = lines, slots
	a.images()
}
