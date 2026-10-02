// SPDX-License-Identifier: GPL-3.0-or-later

// Silk renders live web text in terminal cells and images in separate sixel slots.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SeraphinaDX/Silk/internal/browser"
	"github.com/SeraphinaDX/Silk/internal/config"
	"github.com/SeraphinaDX/Silk/internal/graphics"
	"github.com/SeraphinaDX/Silk/internal/pageview"
	"github.com/SeraphinaDX/Silk/internal/terminal"
)

const version = "0.2.0"

type request struct{ command browser.Command }
type update struct {
	doc     *pageview.Document
	picture *image.RGBA
	key     string
	hash    [32]byte
	err     error
	fatal   bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Silk:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("silk", flag.ContinueOnError)
	path := fs.String("config", config.DefaultPath(), "TOML configuration file")
	chrome := fs.String("chrome", "", "Chromium executable path")
	mode := fs.String("graphics", "", "image graphics: auto, sixel, halfblock or none")
	cell := fs.String("cell", "", "override terminal cell size, e.g. 8x16")
	profile := fs.String("profile", "", "optional persistent Chromium profile directory")
	noSandbox := fs.Bool("no-sandbox", false, "disable Chromium sandbox (only for isolated root/container testing)")
	showVersion := fs.Bool("version", false, "print version")
	fs.Usage = func() { fmt.Fprintln(fs.Output(), "Usage: silk [options] [URL or ./file.html]"); fs.PrintDefaults() }
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Println("Silk", version)
		return nil
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicit = true
		}
	})
	c, err := config.Load(*path, explicit)
	if err != nil {
		return err
	}
	if *chrome != "" {
		c.Chrome = *chrome
	}
	if *mode != "" {
		c.Graphics = *mode
	}
	if *profile != "" {
		c.Profile = *profile
	}
	if *noSandbox {
		c.NoSandbox = true
	}
	if *cell != "" {
		parts := strings.Split(*cell, "x")
		if len(parts) != 2 {
			return fmt.Errorf("-cell needs WIDTHxHEIGHT")
		}
		c.CellWidth, err = strconv.Atoi(parts[0])
		if err != nil {
			return err
		}
		c.CellHeight, err = strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
	}
	if err = c.Validate(); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("only one initial URL is supported")
	}
	if fs.NArg() == 1 {
		c.Home = fs.Arg(0)
	}
	c.Home, err = browser.Address(c.Home)
	if err != nil {
		return err
	}
	if strings.HasPrefix(c.Profile, "~/") {
		home, _ := os.UserHomeDir()
		c.Profile = home + c.Profile[1:]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	s, err := terminal.Open(c.CellWidth, c.CellHeight)
	if err != nil {
		return err
	}
	defer s.Close()
	a := &app{screen: s, config: c, url: c.Home, mode: c.Graphics, status: "Starting Chromium…", zoom: c.Zoom, manualCell: *cell != "", commands: make(chan request, 256), assets: map[string]asset{}, pending: map[string]bool{}}
	if a.mode == "auto" {
		a.mode = "none"
	}
	updates := make(chan update, 4)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan struct{})
	w, h := s.Pixels()
	go func() { defer close(finished); worker(workerCtx, c, w, h, a.commands, updates) }()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
		}
	}()
	chunks := make(chan []byte, 16)
	go terminal.ReadChunks(os.Stdin, chunks, ctx.Done())
	var decoder terminal.Decoder
	var lastInput time.Time
	inputTimer := time.NewTicker(40 * time.Millisecond)
	defer inputTimer.Stop()
	sizeTimer := time.NewTicker(400 * time.Millisecond)
	defer sizeTimer.Stop()
	a.render()
	for {
		select {
		case <-ctx.Done():
			return nil
		case chunk := <-chunks:
			lastInput = time.Now()
			if chunk == nil {
				return nil
			}
			events := decoder.Feed(chunk)
			for i := 0; i < len(events); i++ {
				e := events[i]
				if !a.editing && e.Key == "text" {
					for i+1 < len(events) && events[i+1].Key == "text" {
						i++
						e.Text += events[i].Text
					}
				}
				if a.event(e) {
					return nil
				}
			}
			a.render()
		case <-inputTimer.C:
			if time.Since(lastInput) < 60*time.Millisecond {
				continue
			}
			for _, e := range decoder.Expire() {
				if a.event(e) {
					return nil
				}
				a.render()
			}
		case <-sizeTimer.C:
			oldW, oldH := s.Cols, s.Rows
			s.Resize()
			if oldW != s.Cols || oldH != s.Rows {
				io.WriteString(s.Out, terminal.Probe)
				a.resize()
				a.render()
			}
			a.images()
		case u := <-updates:
			if u.fatal {
				return u.err
			}
			if u.key != "" {
				delete(a.pending, u.key)
				old := a.assets[u.key]
				a.assets[u.key] = asset{u.picture, u.hash, time.Now()}
				if u.err != nil {
					a.status = "Image unavailable"
				}
				if old.hash != u.hash || old.picture == nil {
					a.render()
				}
				continue
			}
			if u.err != nil {
				a.status = terminal.Clean(u.err.Error())
				a.chrome()
				continue
			}
			if u.doc != nil {
				if a.doc.Epoch != u.doc.Epoch {
					a.offset = 0
					a.selected = ""
					a.formEditing = false
					a.assets = map[string]asset{}
					a.pending = map[string]bool{}
				}
				a.doc = *u.doc
				a.url = a.doc.URL
				a.title = a.doc.Title
				if a.selected != "" && a.action(a.selected) == nil {
					a.selected = ""
					a.formEditing = false
				}
				a.reflow()
				a.status = a.title
				a.render()
			}
		}
	}
}

type asset struct {
	picture *image.RGBA
	hash    [32]byte
	fetched time.Time
}

// The worker extracts live text and captures individual requested image nodes.
// It never rasterizes a whole page. Only the UI goroutine writes the terminal.
func worker(ctx context.Context, c config.Config, w, h int, commands <-chan request, updates chan<- update) {
	emit := func(u update) {
		select {
		case updates <- u:
		case <-ctx.Done():
		}
	}
	e, err := browser.New(ctx, browser.Options{Chrome: c.Chrome, Profile: c.Profile, NoSandbox: c.NoSandbox, Width: w, Height: h, Zoom: 1})
	if err != nil {
		emit(update{err: err, fatal: true})
		return
	}
	defer e.Close()
	if err = e.Do(browser.Command{Kind: "navigate", Text: c.Home}); err != nil {
		emit(update{err: err})
	}
	var previous [32]byte
	refresh := func() {
		d, err := e.Document()
		if err != nil {
			emit(update{err: err})
			return
		}
		b, _ := json.Marshal(d)
		hash := sha256.Sum256(b)
		if hash != previous {
			previous = hash
			emit(update{doc: &d})
		}
	}
	ticker := time.NewTicker(time.Duration(c.RefreshMS) * time.Millisecond)
	defer ticker.Stop()
	refresh()
	for {
		select {
		case <-ctx.Done():
			return
		case r := <-commands:
			cmd := r.command
			if cmd.Kind == "picture" {
				img, err := e.Picture(cmd.ID, cmd.Width, cmd.Height)
				if err != nil {
					emit(update{key: cmd.Text, err: err})
				} else {
					pixels := graphics.Resize(img, cmd.Width, cmd.Height)
					emit(update{key: cmd.Text, picture: pixels, hash: sha256.Sum256(pixels.Pix)})
				}
			} else if err = e.Do(cmd); err != nil {
				emit(update{err: err})
			}
			if len(commands) == 0 {
				refresh()
			}
		case <-ticker.C:
			if len(commands) == 0 {
				refresh()
			}
		}
	}
}

type app struct {
	screen                                    *terminal.Screen
	config                                    config.Config
	commands                                  chan request
	url, title, status, mode, edit, selected  string
	editing, replace, manualCell, formEditing bool
	zoom                                      float64
	offset                                    int
	doc                                       pageview.Document
	layout                                    pageview.Layout
	assets                                    map[string]asset
	pending                                   map[string]bool
}

func (a *app) chrome() { a.screen.Chrome(a.url, a.title, a.status, a.edit, a.editing) }
func (a *app) send(c browser.Command) bool {
	select {
	case a.commands <- request{command: c}:
		return true
	default:
		a.status = "Input queue full; wait for the page"
		return false
	}
}
func (a *app) reflow() {
	a.layout = pageview.Build(a.doc, pageview.Options{Cols: a.screen.Cols, CellWidth: a.screen.CellWidth, CellHeight: a.screen.CellHeight, ImageRows: min(a.config.ImageRows, max(1, a.screen.Rows-5)), Images: a.mode != "none", Zoom: a.zoom})
	a.offset = min(max(0, a.offset), max(0, len(a.layout.Lines)-(a.screen.Rows-3)))
}
func (a *app) resize() {
	w, h := a.screen.Pixels()
	a.send(browser.Command{Kind: "resize", Width: w, Height: h, Zoom: 1})
	a.reflow()
}
func (a *app) address() { a.editing = true; a.formEditing = false; a.edit = a.url; a.replace = true }
func (a *app) changeZoom(delta float64) {
	a.zoom = min(3, max(.25, a.zoom+delta))
	a.reflow()
	a.status = fmt.Sprintf("Image size %.0f%%", a.zoom*100)
}
func (a *app) action(id string) *pageview.Action {
	for i := range a.doc.Actions {
		if a.doc.Actions[i].ID == id {
			return &a.doc.Actions[i]
		}
	}
	return nil
}
func (a *app) selectAction(id string, activate bool) {
	target := a.action(id)
	if target == nil {
		return
	}
	a.selected = id
	if target.Editable {
		a.formEditing = true
		a.send(browser.Command{Kind: "focus", ID: id})
	} else {
		a.formEditing = false
		if activate {
			a.send(browser.Command{Kind: "activate", ID: id})
		}
	}
	row := a.layout.ActionRow(id)
	if row >= 0 {
		if row < a.offset {
			a.offset = row
		}
		if row >= a.offset+a.screen.Rows-3 {
			a.offset = row - (a.screen.Rows - 4)
		}
	}
	if target.Href != "" {
		a.status = target.Href
	} else if target.Editable {
		a.status = "Editing field — Escape returns to browsing"
	}
}
func (a *app) cycle(delta int) {
	if len(a.doc.Actions) == 0 {
		return
	}
	index := -1
	for i, t := range a.doc.Actions {
		if t.ID == a.selected {
			index = i
			break
		}
	}
	if index < 0 && delta < 0 {
		index = 0
	}
	index = (index + delta + len(a.doc.Actions)) % len(a.doc.Actions)
	a.selectAction(a.doc.Actions[index].ID, false)
}
func (a *app) scroll(delta int) {
	a.formEditing = false
	a.offset = min(max(0, a.offset+delta), max(0, len(a.layout.Lines)-(a.screen.Rows-3)))
	den := max(1, len(a.layout.Lines)-(a.screen.Rows-3))
	a.send(browser.Command{Kind: "page-scroll", Y: float64(a.offset) / float64(den)})
}
func pictureKey(p pageview.Picture) string {
	return fmt.Sprintf("%s|%s|%dx%d", p.ID, p.Source, p.Width, p.Height)
}
func (a *app) images() {
	if a.mode == "none" {
		return
	}
	if a.assets == nil {
		a.assets = map[string]asset{}
	}
	if a.pending == nil {
		a.pending = map[string]bool{}
	}
	for _, p := range a.layout.Pictures {
		if p.Row+p.Rows <= a.offset || p.Row >= a.offset+a.screen.Rows-3 {
			continue
		}
		key := pictureKey(p)
		if a.pending[key] || time.Since(a.assets[key].fetched) < 2*time.Second {
			continue
		}
		if a.send(browser.Command{Kind: "picture", ID: p.ID, Text: key, Width: p.Width, Height: p.Height}) {
			a.pending[key] = true
		}
	}
}

func (a *app) render() {
	s := a.screen
	io.WriteString(s.Out, "\x1b[?25l\x1b[0m\x1b[2J")
	for row := 0; row < s.Rows-3; row++ {
		fmt.Fprintf(s.Out, "\x1b[%d;1H\x1b[38;2;239;228;243m", row+3)
		index := a.offset + row
		if index >= len(a.layout.Lines) {
			continue
		}
		line := a.layout.Lines[index]
		for _, span := range line.Spans {
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
			io.WriteString(s.Out, style+terminal.Clean(span.Text))
		}
	}
	if a.mode != "none" {
		for _, p := range a.layout.Pictures {
			img := a.assets[pictureKey(p)].picture
			if img == nil {
				continue
			}
			start := max(p.Row, a.offset)
			end := min(p.Row+p.Rows, a.offset+s.Rows-3)
			if start >= end {
				continue
			}
			y0 := (start - p.Row) * s.CellHeight
			y1 := min(img.Bounds().Dy(), (end-p.Row)*s.CellHeight)
			if y0 >= y1 {
				continue
			}
			crop := img.SubImage(image.Rect(0, y0, img.Bounds().Dx(), y1))
			screenRow := start - a.offset + 3
			fmt.Fprintf(s.Out, "\x1b[%d;1H\x1b[0m", screenRow)
			if a.mode == "sixel" {
				s.Out.Write(graphics.Sixel(crop))
			} else {
				s.Out.Write(graphics.HalfblockAt(crop, (p.Width+s.CellWidth-1)/s.CellWidth, end-start, screenRow))
			}
		}
	}
	a.chrome()
	a.images()
}

func (a *app) event(e terminal.Event) bool {
	if e.Report != "" {
		a.report(e.Report)
		return false
	}
	if e.Key == "ctrl-q" || e.Key == "ctrl-c" {
		return true
	}
	if e.Key == "ctrl-l" {
		a.address()
		return false
	}
	if e.Mouse != nil {
		if e.Mouse.Y != 2 {
			a.editing = false
		}
		a.mouse(*e.Mouse)
		return false
	}
	if a.editing {
		switch e.Key {
		case "escape":
			a.editing = false
		case "enter":
			u, err := browser.Address(a.edit)
			if err != nil {
				a.status = err.Error()
			} else {
				a.editing = false
				a.url = u
				a.status = "Loading…"
				a.send(browser.Command{Kind: "navigate", Text: u})
			}
		case "ctrl-u":
			a.edit = ""
			a.replace = false
		case "backspace":
			if a.replace {
				a.edit = ""
				a.replace = false
			} else {
				r := []rune(a.edit)
				if len(r) > 0 {
					a.edit = string(r[:len(r)-1])
				}
			}
		case "text", "paste":
			if a.replace {
				a.edit = ""
				a.replace = false
			}
			a.edit += terminal.Clean(e.Text)
			if len(a.edit) > 8192 {
				a.edit = string([]rune(a.edit)[:min(8192, len([]rune(a.edit)))])
			}
		}
		return false
	}
	switch e.Key {
	case "ctrl-b", "alt-left":
		a.formEditing = false
		a.send(browser.Command{Kind: "back"})
		return false
	case "ctrl-f", "alt-right":
		a.formEditing = false
		a.send(browser.Command{Kind: "forward"})
		return false
	case "ctrl-r", "f5":
		a.formEditing = false
		a.send(browser.Command{Kind: "reload"})
		return false
	case "tab":
		a.cycle(1)
		return false
	case "shift-tab":
		a.cycle(-1)
		return false
	case "escape":
		a.formEditing = false
		a.status = a.title
		return false
	}
	if a.formEditing {
		kind := "key"
		text := e.Key
		if e.Key == "text" {
			text = e.Text
		}
		if e.Key == "paste" {
			kind = "paste"
			text = e.Text
		}
		if e.Key != "" {
			a.send(browser.Command{Kind: kind, Text: text, ID: a.selected})
		}
		return false
	}
	switch e.Key {
	case "enter":
		a.selectAction(a.selected, true)
	case "up":
		a.scroll(-1)
	case "down":
		a.scroll(1)
	case "pageup":
		a.scroll(-(a.screen.Rows - 4))
	case "pagedown", " ":
		a.scroll(a.screen.Rows - 4)
	case "home":
		a.scroll(-len(a.layout.Lines))
	case "end":
		a.scroll(len(a.layout.Lines))
	case "text":
		if e.Text == "j" {
			a.scroll(1)
		} else if e.Text == "k" {
			a.scroll(-1)
		} else if e.Text == " " {
			a.scroll(a.screen.Rows - 4)
		}
	}
	return false
}
func (a *app) report(r string) {
	s := a.screen
	if strings.HasSuffix(r, "c") && a.config.Graphics == "auto" {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(r, "?"), "c"), ";")
		for _, p := range parts {
			if p == "4" && a.mode != "sixel" {
				a.mode = "sixel"
				a.reflow()
				return
			}
		}
	}
	if a.manualCell {
		return
	}
	var kind, h, w int
	if _, err := fmt.Sscanf(r, "%d;%d;%dt", &kind, &h, &w); err != nil {
		return
	}
	if kind == 4 {
		w /= s.Cols
		h /= s.Rows
	} else if kind != 6 {
		return
	}
	if w > 0 && w <= 64 && h > 0 && h <= 128 && (w != s.CellWidth || h != s.CellHeight) {
		s.CellWidth = w
		s.CellHeight = h
		a.resize()
	}
}
func (a *app) mouse(m terminal.Mouse) {
	s := a.screen
	if m.X < 1 || m.X > s.Cols || m.Y < 1 || m.Y > s.Rows {
		return
	}
	if m.Release || m.Button&32 != 0 {
		return
	}
	if m.Button&64 != 0 {
		switch m.Button & 3 {
		case 0:
			a.scroll(-3)
		case 1:
			a.scroll(3)
		}
		return
	}
	if m.Button&3 != 0 {
		return
	}
	if m.Y <= 2 {
		if m.Y == 2 {
			a.address()
			return
		}
		switch {
		case m.X <= 6:
			a.send(browser.Command{Kind: "back"})
		case m.X <= 13:
			a.send(browser.Command{Kind: "forward"})
		case m.X <= 22:
			a.send(browser.Command{Kind: "reload"})
		case m.X <= 28:
			a.address()
		case m.X <= 34:
			a.changeZoom(-.1)
		case m.X <= 40:
			a.changeZoom(.1)
		}
		return
	}
	if m.Y == s.Rows {
		return
	}
	target := a.layout.Hit(a.offset+m.Y-3, m.X-1)
	if target != "" {
		a.selectAction(target, true)
	} else {
		a.formEditing = false
	}
}
