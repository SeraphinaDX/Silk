# Silk 0.2.0 verification

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
- Static Linux amd64 build with CGO_ENABLED=0 and go vet.

Visual output has not been inspected inside a real sixel terminal emulator in
this environment. Tests verify the terminal protocol and browser behaviour.
Other Chromium versions, operating systems and complex public websites were not
part of the deterministic test run. Commands are in the README.
