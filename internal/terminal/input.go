// SPDX-License-Identifier: GPL-3.0-or-later

package terminal

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Mouse struct {
	Button, X, Y int
	Release      bool
}
type Event struct {
	Key, Text string
	Mouse     *Mouse
	Report    string
	Err       error
}

// Decoder is incremental: terminal sequences and UTF-8 may span many reads.
// Expire resolves an isolated Escape after the caller's short timeout.
type Decoder struct {
	buf     []byte
	paste   []byte
	pasting bool
}

func (d *Decoder) Feed(b []byte) []Event {
	d.buf = append(d.buf, b...)
	var events []Event
	for len(d.buf) > 0 {
		if d.pasting {
			end := bytes.Index(d.buf, []byte("\x1b[201~"))
			if end < 0 {
				// Leave enough bytes to recognize a split end marker.
				n := max(0, len(d.buf)-5)
				d.paste = append(d.paste, d.buf[:n]...)
				d.buf = d.buf[n:]
				if len(d.paste) > 1024*1024 {
					d.paste = d.paste[:1024*1024]
				}
				break
			}
			d.paste = append(d.paste, d.buf[:end]...)
			events = append(events, Event{Key: "paste", Text: string(d.paste[:min(len(d.paste), 1024*1024)])})
			d.paste = nil
			d.pasting = false
			d.buf = d.buf[end+6:]
			continue
		}
		c := d.buf[0]
		if c == 27 {
			if len(d.buf) == 1 {
				break
			}
			if d.buf[1] == '[' || d.buf[1] == 'O' {
				end := 2
				for end < len(d.buf) && (d.buf[end] < 0x40 || d.buf[end] > 0x7e) {
					end++
				}
				if end == len(d.buf) {
					if len(d.buf) > 128 {
						d.buf = d.buf[1:]
					}
					break
				}
				s := string(d.buf[2 : end+1])
				d.buf = d.buf[end+1:]
				if s == "200~" {
					d.pasting = true
					continue
				}
				e := sequence(s)
				if e.Key != "" || e.Mouse != nil || e.Report != "" {
					events = append(events, e)
				}
				continue
			}
			d.buf = d.buf[1:]
			events = append(events, Event{Key: "escape"})
			continue
		}
		if c < 32 || c == 127 {
			d.buf = d.buf[1:]
			key := fmt.Sprintf("ctrl-%c", c+'a'-1)
			switch c {
			case 9:
				key = "tab"
			case 13, 10:
				key = "enter"
			case 127, 8:
				key = "backspace"
			}
			events = append(events, Event{Key: key})
			continue
		}
		if !utf8.FullRune(d.buf) {
			break
		}
		r, n := utf8.DecodeRune(d.buf)
		d.buf = d.buf[n:]
		if r != utf8.RuneError || n > 1 {
			events = append(events, Event{Key: "text", Text: string(r)})
		}
	}
	return events
}

func (d *Decoder) Expire() []Event {
	if len(d.buf) == 1 && d.buf[0] == 27 && !d.pasting {
		d.buf = nil
		return []Event{{Key: "escape"}}
	}
	return nil
}

func sequence(s string) Event {
	if strings.HasPrefix(s, "<") && (strings.HasSuffix(s, "M") || strings.HasSuffix(s, "m")) {
		parts := strings.Split(s[1:len(s)-1], ";")
		if len(parts) != 3 {
			return Event{}
		}
		var a [3]int
		for i, p := range parts {
			v, err := strconv.Atoi(p)
			if err != nil || v < 0 {
				return Event{}
			}
			a[i] = v
		}
		return Event{Mouse: &Mouse{a[0], a[1], a[2], strings.HasSuffix(s, "m")}}
	}
	if strings.HasSuffix(s, "t") || strings.HasSuffix(s, "c") {
		return Event{Report: s}
	}
	keys := map[string]string{"A": "up", "B": "down", "C": "right", "D": "left", "H": "home", "F": "end", "Z": "shift-tab", "1~": "home", "2~": "insert", "3~": "delete", "4~": "end", "5~": "pageup", "6~": "pagedown", "7~": "home", "8~": "end", "15~": "f5", "P": "f1", "Q": "f2", "R": "f3", "S": "f4", "1;3D": "alt-left", "1;3C": "alt-right", "1;5D": "ctrl-left", "1;5C": "ctrl-right"}
	return Event{Key: keys[s]}
}

// ReadChunks keeps blocking reads out of the UI. Only the UI owns output.
func ReadChunks(r io.Reader, out chan<- []byte, done <-chan struct{}) {
	for {
		b := make([]byte, 4096)
		n, err := r.Read(b)
		if n > 0 {
			select {
			case out <- b[:n]:
			case <-done:
				return
			}
		}
		if err != nil {
			select {
			case out <- nil:
			case <-done:
			}
			return
		}
	}
}
