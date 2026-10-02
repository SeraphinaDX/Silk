// SPDX-License-Identifier: GPL-3.0-or-later

// Package terminal owns the raw terminal, escape sequence decoding and chrome.
package terminal

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// DECSDM must be reset: inline sixels start at the current text cursor.
// Setting mode 80 instead pins graphics to the upper-left screen corner.
const Enter = "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[?1002h\x1b[?1006h\x1b[?2004h\x1b[?80l\x1b[2J"
const Leave = "\x1b[0m\x1b[?1002l\x1b[?1006l\x1b[?2004l\x1b[?80l\x1b[?7h\x1b[?25h\x1b[?1049l"
const Probe = "\x1b[c\x1b[16t\x1b[14t"

type Screen struct {
	Out                               io.Writer
	FD                                int
	State                             *term.State
	Cols, Rows, CellWidth, CellHeight int
}

func Open(cw, ch int) (*Screen, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, fmt.Errorf("run Silk in an interactive terminal")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	s := &Screen{Out: os.Stdout, FD: fd, State: state, CellWidth: cw, CellHeight: ch}
	s.Resize()
	if _, err = io.WriteString(s.Out, Enter+Probe); err != nil {
		term.Restore(fd, state)
		return nil, err
	}
	return s, nil
}

func (s *Screen) Close() { io.WriteString(s.Out, Leave); term.Restore(s.FD, s.State) }
func (s *Screen) Resize() {
	w, h, err := term.GetSize(s.FD)
	if err == nil {
		s.Cols = max(20, w)
		s.Rows = max(6, h)
	} else {
		s.Cols = 80
		s.Rows = 24
	}
}
func (s *Screen) Pixels() (int, int) { return s.Cols * s.CellWidth, (s.Rows - 3) * s.CellHeight }

// Clean prevents page titles, URLs and browser errors from injecting terminal
// control codes. Only Silk-generated output may contain escape sequences.
func Clean(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 {
			return ' '
		}
		return r
	}, text)
}
func Fit(text string, width int) string { return runewidth.Truncate(Clean(text), max(0, width), "") }

func (s *Screen) Line(row int, text string, style string) {
	fmt.Fprintf(s.Out, "\x1b[%d;1H\x1b[0m%s\x1b[2K%s\x1b[0m", row, style, Fit(text, s.Cols))
}

func (s *Screen) Chrome(url, title, status, edit string, editing bool) {
	s.Line(1, "[Back] [Next] [Reload] [URL] [ - ] [ + ]  Silk 0.2.1", "\x1b[48;2;42;27;48m\x1b[38;2;255;197;225m")
	address := "  " + url
	if editing {
		edit = Clean(edit)
		if width := runewidth.StringWidth(edit); width > s.Cols-5 {
			edit = runewidth.TruncateLeft(edit, width-(s.Cols-5), "")
		}
		address = "  > " + edit
	}
	s.Line(2, address, "\x1b[48;2;28;25;34m\x1b[38;2;244;237;248m")
	if status == "" {
		status = title
	}
	s.Line(s.Rows, "^L URL  Tab links  Enter open  Esc browse  ^Q quit | "+status, "\x1b[48;2;42;27;48m\x1b[38;2;232;207;236m")
	if editing {
		col := min(s.Cols, 5+runewidth.StringWidth(Fit(edit, s.Cols-5)))
		fmt.Fprintf(s.Out, "\x1b[2;%dH\x1b[?25h", col)
	} else {
		io.WriteString(s.Out, "\x1b[?25l")
	}
}
