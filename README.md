# Silk 0.2.0

A text-based terminal browser in Go, with sixel **for images only**.
Headings, paragraphs, links, lists, tables, and form fields are real terminal
characters. Chromium runs JavaScript in the background; Silk reads the live DOM
and reflows it to your terminal width. Images, SVGs and canvases appear in
separate inline sixel rectangles.

This replaces the whole-page screenshot renderer from 0.1.0.

## Build and run

Install Go 1.25 or newer and Chromium/Chrome. Then:

```sh
git clone https://github.com/SeraphinaDX/Silk.git
cd Silk
make build
./silk -graphics=sixel https://example.org
```

Without Make:

```sh
go build -buildvcs=false -trimpath -o silk ./cmd/silk
```

The Go frontend builds with `CGO_ENABLED=0`. Chromium is a separate runtime
dependency; no Chromium binary is bundled. Linux x86-64 is the tested platform.

Try the offline demonstration:

```sh
./silk -graphics=sixel ./examples/demo.html
```

Choose Chromium explicitly if automatic discovery fails:

```sh
./silk -chrome=/usr/bin/chromium -graphics=sixel https://example.org
```

## Text and image rendering

Page text always uses your terminal's font. It is never encoded as sixel or
half-block graphics. Links and controls are underlined; the selected control is
highlighted. Paragraphs and headings reflow to the terminal width, with basic
list, table and preformatted-text handling. Use your terminal's selection
modifier (often Shift) to select native text while mouse reporting is active.

`-graphics` controls **only images**:

| Value | Image behaviour |
| --- | --- |
| `sixel` | Render inline images using sixel; requires a terminal with sixel enabled |
| `auto` | Probe for sixel; otherwise show native alt-text placeholders |
| `none` | Show alt-text placeholders, with no graphics |
| `halfblock` | Optional low-resolution truecolour image fallback |

Images keep their aspect ratio, fit the terminal width, and occupy at most
`image_max_rows` rows (10 by default, also bounded by the viewport). The toolbar
minus/plus buttons change image size, not the terminal font. Partially visible
images are cropped to the content area so they cannot cover the toolbar/footer.
Only visible pictures are requested, cached, and refreshed roughly every two
seconds. Unchanged pictures do not trigger another redraw.

Text extraction updates every `refresh_ms` (400 ms by default), including
JavaScript changes. HTML image elements, inline SVG and canvas are supported;
CSS background images are not currently extracted. Broken images keep their
native alt-text label.

## Controls

| Control | Action |
| --- | --- |
| Click a link or button | Activate the original page element |
| Click a field | Focus it for typing |
| Tab / Shift+Tab | Select the next/previous link or form control |
| Enter | Activate the selected link/button; when editing, send Enter to the field |
| Escape | Leave field editing or cancel address editing |
| Wheel / Up / Down / j / k | Scroll the terminal document while browsing |
| PageUp / PageDown / Space | Scroll one page while browsing |
| Home / End | Jump to the start/end while browsing |
| Ctrl+L or click address row | Edit the URL; typing replaces its current contents |
| Enter in address | Navigate |
| Ctrl+U in address | Clear the URL |
| Backspace in address | Remove the last character |
| Ctrl+B / Alt+Left | Back |
| Ctrl+F / Alt+Right | Forward |
| Ctrl+R / F5 | Reload |
| Ctrl+Q / Ctrl+C | Quit |
| Toolbar minus / plus | Adjust inline image size |

When a field is active, text, arrow keys, Home/End and other supported keys go
to Chromium. Real key events reach JavaScript handlers. Terminal bracketed paste
is inserted in one operation, preserving Unicode and multiline textarea input.
Checkboxes and radio buttons can be clicked or selected and activated with Enter.
Select fields support keyboard selection with Up/Down.

The address editor supports replacement, append, backspace and clear; a movable
address insertion cursor and URL suggestions are not implemented.

## Configuration

Optional Linux config: `~/.config/silk/config.toml`, based on Go's
`os.UserConfigDir()` and respecting `XDG_CONFIG_HOME`. Start with
`config.example.toml`, or choose a file explicitly:

```sh
./silk -config=./config.example.toml -graphics=sixel https://example.org
```

CLI flags override corresponding TOML settings. Bare hostnames receive HTTPS.
For local files use `./page.html`, an absolute path, or `file:///path/page.html`.

Relevant settings:

```toml
home = "https://example.org"
chrome = ""
graphics = "auto"
refresh_ms = 400
image_max_rows = 10
zoom = 1.0
cell_width = 8
cell_height = 16
profile = ""
no_sandbox = false
```

`zoom` scales images. Terminal pixel/cell-size reports override the fallback cell
settings. If pictures overlap text or look incorrectly sized, lock the actual
font cell dimensions, for example:

```sh
./silk -graphics=sixel -cell=10x20 https://example.org
```

Text layout and clicks use terminal cells, so they do not depend on the font's
pixel dimensions. Correct pixel dimensions matter only for inline graphics.

Chromium uses a temporary profile by default. To keep cookies and localStorage:

```sh
./silk -profile=~/.local/share/silk/profile https://example.org
```

Use a dedicated Silk profile, rather than one in use by desktop Chrome.
Chromium's sandbox remains enabled by default. `-no-sandbox` is provided for
isolated root test containers.

## Current limits

- One tab. New-window links and `window.open()` are redirected to the current tab.
- This is semantic text reflow, not a reproduction of a page's CSS layout.
  Complex visual widgets, custom drag interfaces and canvas-only applications
  may not be usable through their text representation.
- Open shadow DOM and accessible same-origin iframe contents are extracted.
  Cross-origin iframe contents get a placeholder. Closed shadow DOM is not read.
- Browser-controlled downloads are disabled. File-upload pickers, native menus,
  printing, permission UI, bookmarks and a JavaScript console are not implemented.
  Alerts are acknowledged; confirms/prompts are canceled.
- Scrolling mirrors approximate document progress to Chromium to trigger lazy
  content. Virtualized/infinite-scroll applications can still behave differently.
- Large pages are bounded to 50,000 visited nodes, 24,000 tokens and about two
  million text characters. A truncation marker is displayed if a limit is reached.
- Image output uses a fixed 256-colour sixel palette. Animation refresh is modest.
  Some terminals or multiplexers require sixel configuration. `-graphics=none`
  provides a readable native-text view without graphics support.
- Browser operations have an eight-second timeout. Terminal input remains
  responsive while the browser worker handles requests.
- Terminal attributes are restored on normal exit, Ctrl+C, Ctrl+Q, SIGTERM,
  SIGHUP and startup failure. SIGKILL cannot be handled.

## Tests

```sh
make test
SILK_CHROME=/usr/bin/chromium make integration
SILK_CHROME=/usr/bin/chromium python3 tools/ui_smoke.py
go vet -buildvcs=false ./...
```

For an isolated root test container, add `SILK_TEST_NO_SANDBOX=1` to the
integration/smoke commands. The tests use Chromium and a pseudo-terminal, not a
mock browser. They assert that page text appears outside graphics escape blocks,
that text-only pages emit no sixel data, and that image captures are bounded to
individual image rectangles. See `TESTING.md` for the verification details.

## Source guide

| Directory | Responsibility |
| --- | --- |
| `cmd/silk` | CLI, native page renderer, scrolling, focus and browser worker |
| `internal/browser` | Chromium, live DOM extraction, trusted activation, image-only capture |
| `internal/pageview` | Word wrapping, native spans, clickable cell ranges and image slots |
| `internal/terminal` | Raw mode, safe terminal UI, fragmented UTF-8/input decoding |
| `internal/graphics` | Image resizing, sixel and image-only half-block encoding |
| `internal/config` | Optional TOML configuration |
| `examples` | Offline JavaScript/text/image demo |
| `tools` | Full CLI pseudo-terminal smoke test |

One goroutine owns browser operations; the UI is the only terminal output writer.
Stable DOM identities connect reflowed links/fields to their real Chromium nodes.
Trusted mouse events activate page elements after resolving their original
positions. Image captures never provide the page's terminal text.

## License

Silk is licensed under **GPLv3 or later** (`GPL-3.0-or-later`).

Silk is free software: you can redistribute it and/or modify it under the terms
of the GNU General Public License as published by the Free Software Foundation,
either version 3 of the License, or (at your option) any later version.

Silk is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR
PURPOSE. See [LICENSE](LICENSE) for the complete GNU General Public License.

Third-party dependencies retain their original licenses and notices in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Chromium is installed separately
and retains its own licenses.
