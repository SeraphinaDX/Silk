// SPDX-License-Identifier: GPL-3.0-or-later

// Package pageview reflows a live DOM into terminal cells. It never rasterizes
// text. Images occupy separate, bounded rectangles between text lines.
package pageview

import (
	"github.com/mattn/go-runewidth"
	"math"
	"strings"
	"unicode"
)

type Token struct {
	Kind   string  `json:"kind"`
	Text   string  `json:"text"`
	ID     string  `json:"id"`
	Action string  `json:"action"`
	Style  string  `json:"style"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Source string  `json:"source"`
}
type Action struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Label    string `json:"label"`
	Href     string `json:"href"`
	Editable bool   `json:"editable"`
}
type Document struct {
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Epoch     string   `json:"epoch"`
	Active    string   `json:"active"`
	Tokens    []Token  `json:"tokens"`
	Actions   []Action `json:"actions"`
	Truncated bool     `json:"truncated"`
}
type Span struct {
	Text, Action, Style string
	Start, End          int
}
type Line struct {
	Spans           []Span
	ImageID, Action string
}
type Picture struct {
	ID, Source               string
	Row, Rows, Width, Height int
}
type Layout struct {
	Lines    []Line
	Pictures []Picture
}
type Options struct {
	Cols, CellWidth, CellHeight, ImageRows int
	Images                                 bool
	Zoom                                   float64
}

func Safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return ' '
		}
		return r
	}, s)
}

func Build(d Document, o Options) Layout {
	o.Cols = max(1, o.Cols)
	o.CellWidth = max(1, o.CellWidth)
	o.CellHeight = max(1, o.CellHeight)
	o.ImageRows = max(1, o.ImageRows)
	if o.Zoom <= 0 {
		o.Zoom = 1
	}
	l := Layout{Lines: []Line{{}}}
	col := 0
	newline := func() {
		if col > 0 || len(l.Lines[len(l.Lines)-1].Spans) > 0 {
			l.Lines = append(l.Lines, Line{})
			col = 0
		}
	}
	appendText := func(text, action, style string) {
		line := &l.Lines[len(l.Lines)-1]
		width := runewidth.StringWidth(text)
		if len(line.Spans) > 0 {
			last := &line.Spans[len(line.Spans)-1]
			if last.Action == action && last.Style == style {
				last.Text += text
				last.End += width
				col += width
				return
			}
		}
		line.Spans = append(line.Spans, Span{text, action, style, col, col + width})
		col += width
	}
	write := func(text, action, style string, pre bool) {
		text = Safe(text)
		if pre {
			text = strings.ReplaceAll(text, "\t", "    ")
		}
		// Retain word boundaries across tokens; split only words wider than a line.
		chunks := []string{}
		start := 0
		space := false
		for i, r := range text {
			isSpace := unicode.IsSpace(r)
			if i > start && isSpace != space {
				chunks = append(chunks, text[start:i])
				start = i
			}
			space = isSpace
		}
		if start < len(text) {
			chunks = append(chunks, text[start:])
		}
		for _, chunk := range chunks {
			if !pre && strings.TrimSpace(chunk) == "" {
				if col > 0 && col < o.Cols {
					appendText(" ", action, style)
				}
				continue
			}
			if !pre && col > 0 && col+runewidth.StringWidth(chunk) > o.Cols {
				newline()
			}
			for _, r := range chunk {
				if r == '\n' {
					if pre {
						l.Lines = append(l.Lines, Line{})
						col = 0
					} else {
						newline()
					}
					continue
				}
				w := runewidth.RuneWidth(r)
				if w > o.Cols {
					continue
				}
				if col+w > o.Cols {
					newline()
				}
				appendText(string(r), action, style)
			}
		}
	}
	for _, t := range d.Tokens {
		switch t.Kind {
		case "break":
			newline()
		case "text", "control":
			write(t.Text, t.Action, t.Style, t.Style == "pre")
		case "image":
			newline()
			label := "[image"
			if t.Text != "" {
				label += ": " + t.Text
			}
			label += "]"
			write(label, t.Action, "image", false)
			newline()
			if o.Images && t.Width > 0 && t.Height > 0 {
				w, h := t.Width*o.Zoom, t.Height*o.Zoom
				scale := math.Min(1, math.Min(float64(o.Cols*o.CellWidth)/w, float64(o.ImageRows*o.CellHeight)/h))
				pw, ph := max(1, int(w*scale)), max(1, int(h*scale))
				rows := (ph + o.CellHeight - 1) / o.CellHeight
				row := len(l.Lines) - 1
				l.Pictures = append(l.Pictures, Picture{t.ID, t.Source, row, rows, pw, ph})
				l.Lines[row] = Line{ImageID: t.ID, Action: t.Action}
				for j := 1; j < rows; j++ {
					l.Lines = append(l.Lines, Line{ImageID: t.ID, Action: t.Action})
				}
				l.Lines = append(l.Lines, Line{})
				col = 0
			}
		}
	}
	if d.Truncated {
		newline()
		write("[Large page truncated after the extraction limit]", "", "muted", false)
	}
	if len(l.Lines) > 1 && len(l.Lines[len(l.Lines)-1].Spans) == 0 && l.Lines[len(l.Lines)-1].ImageID == "" {
		l.Lines = l.Lines[:len(l.Lines)-1]
	}
	return l
}

func (l Layout) Hit(row, col int) string {
	if row < 0 || row >= len(l.Lines) {
		return ""
	}
	line := l.Lines[row]
	if line.ImageID != "" {
		return line.Action
	}
	for _, s := range line.Spans {
		if col >= s.Start && col < s.End {
			return s.Action
		}
	}
	return ""
}
func (l Layout) ActionRow(id string) int {
	for row, line := range l.Lines {
		if line.Action == id && id != "" {
			return row
		}
		for _, s := range line.Spans {
			if s.Action == id && id != "" {
				return row
			}
		}
	}
	return -1
}
