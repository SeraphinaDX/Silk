// SPDX-License-Identifier: GPL-3.0-or-later

package terminal

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestFragmentedInput(t *testing.T) {
	stream := []byte("é\x1b[<0;12;9M\x1b[<0;12;9m\x1b[<65;12;9M\x1b[A\x1b[200~a\nb😀\x1b[201~\x1b[6;16;8t\x1b[?1;2;4c")
	d := Decoder{}
	var got []Event
	for _, b := range stream {
		got = append(got, d.Feed([]byte{b})...)
	}
	want := []Event{{Key: "text", Text: "é"}, {Mouse: &Mouse{0, 12, 9, false}}, {Mouse: &Mouse{0, 12, 9, true}}, {Mouse: &Mouse{65, 12, 9, false}}, {Key: "up"}, {Key: "paste", Text: "a\nb😀"}, {Report: "6;16;8t"}, {Report: "?1;2;4c"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded %#v", got)
	}
}
func TestEscapeAndUnknownSequences(t *testing.T) {
	d := Decoder{}
	if got := d.Feed([]byte{27}); len(got) != 0 {
		t.Fatal(got)
	}
	if got := d.Expire(); len(got) != 1 || got[0].Key != "escape" {
		t.Fatal(got)
	}
	got := d.Feed([]byte("\x1b[999~\x1b[<0;-1;2M\x11"))
	if len(got) != 1 || got[0].Key != "ctrl-q" {
		t.Fatal(got)
	}
}
func TestSafeTerminalText(t *testing.T) {
	text := Clean("title\x1b[2J\r\n\u009b")
	if strings.ContainsAny(text, "\x1b\r\n\u009b") {
		t.Fatal("unsafe control text")
	}
	if got := Fit("界éabc", 4); got != "界éa" {
		t.Fatalf("cell width: %q", got)
	}
	var out bytes.Buffer
	s := Screen{Out: &out, Cols: 80, Rows: 24}
	s.Chrome("https://example.org", "title", "", "test", true)
	if !bytes.Contains(out.Bytes(), []byte("\x1b[2;9H\x1b[?25h")) {
		t.Fatal("address cursor is misplaced")
	}
	out.Reset()
	s.Chrome("url", "title", "", "", false)
	if !bytes.Contains(out.Bytes(), []byte("\x1b[?25l")) {
		t.Fatal("cursor remains visible outside address editing")
	}
}
