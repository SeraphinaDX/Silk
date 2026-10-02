# Silk 0.2.1 verification

Verified on 2026-10-02 on Linux x86-64 with Go 1.25.1 and Chrome for Testing
headless shell 134.0.6998.35. Only the isolated root test container disables
Chromium's sandbox; normal Silk startup keeps it enabled.

Validation covers:

- Unit tests for native text wrapping, Unicode cell widths, action hit ranges,
  preformatted lines, bounded image rectangles, scrolling/focus routing,
  fragmented terminal input, config validation and terminal text sanitization.
- Independent decoding of emitted sixel pixels, including palette data, repeated
  runs, nonzero image origins and partial six-pixel bands.
- Real Chromium integration against a local HTTP fixture: live DOM text,
  hidden/password filtering, JavaScript timers and updates, individual image
  capture, stable DOM identities, trusted activation, printable key events,
  Ctrl+A, Unicode/emoji and multiline paste, checkboxes/select fields,
  approximate lazy scrolling, history/reload and stale-target rejection.
- Full CLI pseudo-terminal tests with sixel, no graphics, half-block images and
  automatic detection. Native page text is asserted outside graphics blocks;
  a text-only page in forced sixel mode must emit no sixel blocks. Every image
  block is checked against the fixture image dimensions rather than the viewport.
  Tests also exercise mouse links/buttons/fields, form paste, terminal scrolling,
  resizing, history, startup failure, Ctrl+Q/SIGTERM and termios restoration.
- JPEG and AVIF pixel checks through real Chromium, including XHTML, smooth
  scrolling CSS and native lazy images; image capture must preserve scrollY.
- Renderer regression checks: motion/release reports preserve address editing;
  hover emits no terminal output; selection/status updates preserve cached sixels;
  changed images repaint only their slot; scrolling/removal erase stale graphics.
- Static Linux amd64 build with CGO_ENABLED=0 and go vet.

Additional manual verification used a local copy of the reported Cerberus Games
page and its actual scientists.avif / canada.jpg files (not included in this
repository). The full CLI PTY output was replayed into xterm.js 5.5.0 with the
image addon 0.8.0 in Chromium. Both images were visually inspected in their
inline slots with native text. Replaying the incorrect DECSDM setting reproduced
top-left graphics placement; resetting it restored inline placement. A burst of
100 mouse-motion reports produced zero output bytes, clears or image blocks.

Desktop Konsole/Alacritty, other Chromium versions and other operating systems
were not available for testing. Standard Alacritty lacks sixel support; forced
output does not change that. Commands are in the README.
