// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"github.com/SeraphinaDX/Silk/internal/config"
	"github.com/SeraphinaDX/Silk/internal/pageview"
	"github.com/SeraphinaDX/Silk/internal/terminal"
	"image"
	"image/color"
	"strings"
	"testing"
)

func testApp() *app {
	return &app{screen: &terminal.Screen{Out: &bytes.Buffer{}, Cols: 80, Rows: 24, CellWidth: 8, CellHeight: 16}, config: config.Config{Graphics: "auto", ImageRows: 10}, mode: "none", zoom: 1, url: "https://example.org", commands: make(chan request, 32), assets: map[string]asset{}, pending: map[string]bool{}}
}
func TestNativeTextAndImageSeparation(t *testing.T) {
	a := testApp()
	a.doc = pageview.Document{Tokens: []pageview.Token{{Kind: "text", Text: "Readable native text café"}}}
	a.reflow()
	a.render()
	out := a.screen.Out.(*bytes.Buffer).String()
	if !strings.Contains(out, "Readable native text café") || strings.Contains(out, "\x1bP") {
		t.Fatal("text was rasterized", out)
	}
	a.mode = "sixel"
	a.doc.Tokens = append(a.doc.Tokens, pageview.Token{Kind: "image", ID: "pic", Text: "photo", Width: 80, Height: 32, Source: "photo.png"})
	a.reflow()
	p := a.layout.Pictures[0]
	img := image.NewRGBA(image.Rect(0, 0, 80, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 80; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	a.assets[pictureKey(p)] = asset{picture: img}
	a.screen.Out.(*bytes.Buffer).Reset()
	a.render()
	out = a.screen.Out.(*bytes.Buffer).String()
	if !strings.Contains(out, "Readable native text café") || !strings.Contains(out, "\x1bP0;1;0q\"1;1;80;32") {
		t.Fatal("image/text separation failed")
	}
	if strings.Contains(out, "\"1;1;640;336") {
		t.Fatal("whole viewport rasterized")
	}
}
func TestMouseAndKeyboardTargets(t *testing.T) {
	a := testApp()
	a.doc = pageview.Document{Tokens: []pageview.Token{{Kind: "text", Text: "Open link", Action: "link"}, {Kind: "break"}, {Kind: "control", Text: "[Name: ]", Action: "field"}}, Actions: []pageview.Action{{ID: "link", Role: "link"}, {ID: "field", Role: "text", Editable: true}}}
	a.reflow()
	a.mouse(terminal.Mouse{Button: 0, X: 3, Y: 3})
	c := (<-a.commands).command
	if c.Kind != "activate" || c.ID != "link" {
		t.Fatal(c)
	}
	a.mouse(terminal.Mouse{Button: 0, X: 3, Y: 4})
	c = (<-a.commands).command
	if c.Kind != "focus" || c.ID != "field" {
		t.Fatal(c)
	}
	a.event(terminal.Event{Key: "text", Text: "hello"})
	c = (<-a.commands).command
	if c.Kind != "key" || c.ID != "field" || c.Text != "hello" {
		t.Fatal(c)
	}
	a.event(terminal.Event{Key: "escape"})
	if a.formEditing {
		t.Fatal("field remains active")
	}
	a.event(terminal.Event{Key: "shift-tab"})
	if a.selected != "link" {
		t.Fatal(a.selected)
	}
}
func TestAddressAndCapabilityReports(t *testing.T) {
	a := testApp()
	a.event(terminal.Event{Key: "ctrl-l"})
	a.event(terminal.Event{Key: "text", Text: "example.com"})
	a.event(terminal.Event{Key: "enter"})
	c := (<-a.commands).command
	if c.Kind != "navigate" || c.Text != "https://example.com" {
		t.Fatal(c)
	}
	a.report("?1;2;4c")
	if a.mode != "sixel" {
		t.Fatal(a.mode)
	}
	a.report("6;20;10t")
	c = (<-a.commands).command
	if c.Width != 800 || c.Height != 420 {
		t.Fatal(c)
	}
	a.manualCell = true
	a.report("6;30;15t")
	if len(a.commands) != 0 {
		t.Fatal("manual cell override ignored")
	}
}

func TestHoverAndSelectionPreserveGraphics(t *testing.T) {
	a := testApp()
	a.mode = "sixel"
	a.doc = pageview.Document{Epoch: "page", Tokens: []pageview.Token{{Kind: "text", Text: "Open", Action: "link"}, {Kind: "break"}, {Kind: "image", ID: "pic", Source: "image.jpg", Width: 80, Height: 32}}, Actions: []pageview.Action{{ID: "link", Role: "link"}}}
	a.reflow()
	p := a.layout.Pictures[0]
	img := image.NewRGBA(image.Rect(0, 0, 80, 32))
	a.assets[pictureKey(p)] = asset{picture: img}
	a.render()
	out := a.screen.Out.(*bytes.Buffer)
	out.Reset()
	a.event(terminal.Event{Mouse: &terminal.Mouse{Button: 32, X: 2, Y: 3}})
	a.render()
	if out.Len() != 0 {
		t.Fatalf("hover repainted page: %q", out.String())
	}
	a.event(terminal.Event{Key: "tab"})
	a.render()
	if !strings.Contains(out.String(), "Open") || strings.Contains(out.String(), "\x1bP") || strings.Contains(out.String(), "\x1b[2J") {
		t.Fatalf("selection disturbed graphics: %q", out.String())
	}
	out.Reset()
	a.status = "Link target"
	a.render()
	if strings.Contains(out.String(), "Open") || strings.Contains(out.String(), "\x1bP") || strings.Contains(out.String(), "\x1b[2J") {
		t.Fatal("status update repainted page")
	}
	a.address()
	a.render()
	out.Reset()
	a.event(terminal.Event{Mouse: &terminal.Mouse{Button: 32, X: 2, Y: 3}})
	a.event(terminal.Event{Mouse: &terminal.Mouse{Button: 0, X: 2, Y: 3, Release: true}})
	a.render()
	if !a.editing || out.Len() != 0 {
		t.Fatal("hover/release interrupted address editing")
	}
}

func TestChangedPictureAndScroll(t *testing.T) {
	a := testApp()
	a.mode = "sixel"
	a.doc = pageview.Document{Tokens: []pageview.Token{{Kind: "image", ID: "pic", Source: "image.jpg", Width: 80, Height: 32}, {Kind: "text", Text: strings.Repeat("other text ", 300)}}}
	a.reflow()
	p := a.layout.Pictures[0]
	a.assets[pictureKey(p)] = asset{picture: image.NewRGBA(image.Rect(0, 0, 80, 32))}
	a.render()
	out := a.screen.Out.(*bytes.Buffer)
	out.Reset()
	picture := a.assets[pictureKey(p)]
	picture.hash[0] = 1
	a.assets[pictureKey(p)] = picture
	a.render()
	if !strings.Contains(out.String(), "\x1bP") || strings.Contains(out.String(), "\x1b[2J") {
		t.Fatal("new picture should update only its slot")
	}
	out.Reset()
	a.offset = 2
	a.render()
	if !strings.Contains(out.String(), "\x1b[2J") || !strings.Contains(out.String(), "\"1;1;80;16") {
		t.Fatal("scroll did not clear/move/crop old graphics")
	}
	out.Reset()
	a.doc.Tokens = nil
	a.reflow()
	a.render()
	if !strings.Contains(out.String(), "\x1b[2J") || strings.Contains(out.String(), "\x1bP") {
		t.Fatal("removed picture not erased")
	}
}
