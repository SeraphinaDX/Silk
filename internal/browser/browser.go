// SPDX-License-Identifier: GPL-3.0-or-later

// Package browser is the Chromium adapter. The Go terminal frontend never
// implements a partial DOM or JavaScript sandbox; Chromium runs the page.
package browser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

type Options struct {
	Chrome, Profile string
	NoSandbox       bool
	Width, Height   int
	Zoom            float64
}
type Command struct {
	Kind, Text    string
	ID            string
	X, Y, DX, DY  float64
	Button        int
	Release, Move bool
	Modifiers     int64
	Width, Height int
	Zoom          float64
	Clicks        int64
}
type Engine struct {
	ctx           context.Context
	cancel        context.CancelFunc
	stopAllocator context.CancelFunc
	dialogs       chan *page.EventJavascriptDialogOpening
	zoom          float64
}

func New(parent context.Context, opts Options) (*Engine, error) {
	flags := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	// Explicit false preserves Chromium's sandbox, even when invoked as root.
	flags = append(flags, chromedp.Flag("no-sandbox", opts.NoSandbox), chromedp.Flag("hide-scrollbars", false))
	if opts.Chrome != "" {
		flags = append(flags, chromedp.ExecPath(opts.Chrome))
	}
	if opts.Profile != "" {
		if err := os.MkdirAll(opts.Profile, 0700); err != nil {
			return nil, err
		}
		flags = append(flags, chromedp.UserDataDir(opts.Profile))
	}
	alloc, stop := chromedp.NewExecAllocator(parent, flags...)
	ctx, cancel := chromedp.NewContext(alloc, chromedp.WithLogf(func(string, ...any) {}))
	e := &Engine{ctx: ctx, cancel: cancel, stopAllocator: stop, dialogs: make(chan *page.EventJavascriptDialogOpening, 8), zoom: 1}
	chromedp.ListenTarget(ctx, func(ev any) {
		if d, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			select {
			case e.dialogs <- d:
			default:
			}
		}
	})
	if err := chromedp.Run(ctx); err != nil {
		e.Close()
		return nil, fmt.Errorf("start Chromium: %w (install Chromium or set -chrome)", err)
	}
	// Silk is single-tab. Keep target=_blank and window.open in this tab.
	const sameTab = `(() => { window.open = (url) => { if (url) location.href = url; return window; }; document.addEventListener('click', e => { const a=e.target.closest && e.target.closest('a'); if(a && a.target && a.target !== '_self') a.target='_self'; }, true); })();`
	err := e.run(cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny), chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(sameTab).Do(ctx)
		return err
	}))
	if err == nil {
		err = e.Do(Command{Kind: "resize", Width: opts.Width, Height: opts.Height, Zoom: opts.Zoom})
	}
	if err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

func (e *Engine) Close() { e.cancel(); e.stopAllocator() }
func (e *Engine) run(actions ...chromedp.Action) error {
	ctx, cancel := context.WithTimeout(e.ctx, 8*time.Second)
	defer cancel()
	return chromedp.Run(ctx, actions...)
}

func (e *Engine) dismissDialogs() error {
	for {
		select {
		case d := <-e.dialogs:
			if err := e.run(page.HandleJavaScriptDialog(d.Type == page.DialogTypeAlert)); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (e *Engine) Do(c Command) error {
	if err := e.dismissDialogs(); err != nil {
		return err
	}
	switch c.Kind {
	case "activate":
		r, err := e.elementRect(c.ID)
		if err != nil {
			return err
		}
		if r.Width <= 0 || r.Height <= 0 {
			return fmt.Errorf("control is no longer visible")
		}
		return e.run(input.DispatchMouseEvent(input.MousePressed, r.X+r.Width/2, r.Y+r.Height/2).WithButton(input.Left).WithClickCount(1), input.DispatchMouseEvent(input.MouseReleased, r.X+r.Width/2, r.Y+r.Height/2).WithButton(input.Left).WithClickCount(1))
	case "focus":
		return e.focus(c.ID)
	case "page-scroll":
		return e.run(chromedp.Evaluate(fmt.Sprintf(`window.scrollTo(0,Math.max(0,document.documentElement.scrollHeight-innerHeight)*%f)`, min(1, max(0, c.Y))), nil))
	case "navigate":
		address, err := Address(c.Text)
		if err != nil {
			return err
		}
		// Wait for this navigation so a following Back cannot run against the
		// previous history index. The worker keeps the terminal responsive.
		return e.run(chromedp.Navigate(address))
	case "resize":
		if c.Width < 1 || c.Height < 1 || c.Width > 8192 || c.Height > 8192 {
			return fmt.Errorf("viewport must be 1..8192 pixels per side")
		}
		err := e.run(emulation.SetDeviceMetricsOverride(int64(c.Width), int64(c.Height), 1, false), emulation.SetPageScaleFactor(c.Zoom))
		if err == nil {
			e.zoom = c.Zoom
		}
		return err
	case "zoom":
		err := e.run(emulation.SetPageScaleFactor(c.Zoom))
		if err == nil {
			e.zoom = c.Zoom
		}
		return err
	case "reload":
		return e.run(page.Reload())
	case "back", "forward":
		return e.run(chromedp.ActionFunc(func(ctx context.Context) error {
			index, entries, err := page.GetNavigationHistory().Do(ctx)
			if err != nil {
				return err
			}
			if c.Kind == "back" {
				index--
			} else {
				index++
			}
			if index < 0 || index >= int64(len(entries)) {
				return nil
			}
			return page.NavigateToHistoryEntry(entries[index].ID).Do(ctx)
		}))
	case "paste":
		if c.ID != "" {
			if err := e.focus(c.ID); err != nil {
				return err
			}
		}
		return e.run(input.InsertText(c.Text))
	case "key":
		if c.ID != "" {
			if err := e.focus(c.ID); err != nil {
				return err
			}
		}
		keys := map[string]string{"enter": kb.Enter, "tab": kb.Tab, "shift-tab": kb.Tab, "backspace": kb.Backspace, "delete": kb.Delete, "escape": kb.Escape, "up": kb.ArrowUp, "down": kb.ArrowDown, "left": kb.ArrowLeft, "right": kb.ArrowRight, "home": kb.Home, "end": kb.End, "pageup": kb.PageUp, "pagedown": kb.PageDown}
		keys["f1"], keys["f2"], keys["f3"], keys["f4"] = kb.F1, kb.F2, kb.F3, kb.F4
		if strings.HasPrefix(c.Text, "ctrl-") && len(c.Text) > 6 {
			c.Text = strings.TrimPrefix(c.Text, "ctrl-")
			c.Modifiers |= int64(input.ModifierCtrl)
		}
		if key, ok := keys[c.Text]; ok {
			mods := input.Modifier(c.Modifiers)
			if c.Text == "shift-tab" {
				mods |= input.ModifierShift
			}
			return e.run(chromedp.KeyEvent(key, chromedp.KeyModifiers(mods)))
		}
		if strings.HasPrefix(c.Text, "ctrl-") && len(c.Text) == 6 {
			return e.run(chromedp.KeyEvent(c.Text[5:], chromedp.KeyModifiers(input.ModifierCtrl)))
		}
		return e.run(chromedp.KeyEvent(c.Text))
	case "scroll":
		return e.run(input.DispatchMouseEvent(input.MouseWheel, c.X/e.zoom, c.Y/e.zoom).WithDeltaX(c.DX / e.zoom).WithDeltaY(c.DY / e.zoom).WithModifiers(input.Modifier(c.Modifiers)))
	case "mouse":
		buttons := []input.MouseButton{input.Left, input.Middle, input.Right}
		button := input.None
		if c.Button >= 0 && c.Button < 3 {
			button = buttons[c.Button]
		}
		kind := input.MousePressed
		mask := int64(0)
		if c.Button == 0 {
			mask = 1
		}
		if c.Button == 1 {
			mask = 4
		}
		if c.Button == 2 {
			mask = 2
		}
		if c.Release {
			kind = input.MouseReleased
			mask = 0
		}
		if c.Move {
			kind = input.MouseMoved
		}
		// CDP expects CSS coordinates before visual-viewport magnification;
		// the terminal reports positions in the displayed screenshot pixels.
		return e.run(input.DispatchMouseEvent(kind, c.X/e.zoom, c.Y/e.zoom).WithButton(button).WithButtons(mask).WithClickCount(max(1, c.Clicks)).WithModifiers(input.Modifier(c.Modifiers)))
	default:
		return fmt.Errorf("unknown browser command %q", c.Kind)
	}
}
