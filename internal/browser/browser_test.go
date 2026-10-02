// SPDX-License-Identifier: GPL-3.0-or-later

package browser

import (
	"context"
	"fmt"
	"github.com/SeraphinaDX/Silk/internal/pageview"
	"github.com/chromedp/chromedp"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAddress(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"example.org", "https://example.org"}, {"http://localhost:1234", "http://localhost:1234"}, {"about:blank", "about:blank"}} {
		got, err := Address(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%q -> %q %v", tc.in, got, err)
		}
	}
	for _, s := range []string{"", "javascript://alert(1)", "chrome://settings", "http://", "about:config"} {
		if _, err := Address(s); err == nil {
			t.Fatal("accepted", s)
		}
	}
	got, err := Address("./space name.html")
	if err != nil || !strings.HasSuffix(got, "space%20name.html") {
		t.Fatal(got, err)
	}
}
func TestTextBrowserIntegration(t *testing.T) {
	if os.Getenv("SILK_INTEGRATION") == "" {
		t.Skip("set SILK_INTEGRATION=1")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/photo.svg" {
			w.Header().Set("Content-Type", "image/svg+xml")
			fmt.Fprint(w, `<svg xmlns="http://www.w3.org/2000/svg" width="90" height="45"><rect width="90" height="45" fill="red"/></svg>`)
			return
		}
		if r.URL.Path == "/missing.svg" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/second" {
			fmt.Fprint(w, `<title>Second</title><h1>Second page text</h1>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><meta charset=utf-8><title>Ready</title><style>body{height:1800px}#hidden{display:none}canvas{display:block}</style><h1>Real terminal text</h1><p>Café and Unicode 😀.</p><p id=hidden>Secret hidden text</p><button id=b>Click me</button><input id=i placeholder="Name"><textarea id=t aria-label="Notes"></textarea><input type=password id=p value=secret><input type=checkbox id=check aria-label=agree><select id=s aria-label=Colour><option>Red</option><option>Blue</option></select><canvas id=c width=120 height=60></canvas><img src="/photo.svg" alt="Loaded picture"><img src="/missing.svg" alt="Broken picture" width=50 height=20><svg width=0 height=0></svg><a id=next href=/second target=_blank>Next page</a><script>let n=0;window.keys=0;window.ticks=0;b.onclick=()=>{b.textContent='Clicked '+(++n)};i.onkeydown=()=>keys++;i.oninput=()=>document.title='Typed:'+i.value;setInterval(()=>ticks++,70);const ctx=c.getContext('2d');ctx.fillStyle='#0000ff';ctx.fillRect(0,0,120,60);</script>`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	e, err := New(ctx, Options{Chrome: os.Getenv("SILK_CHROME"), NoSandbox: os.Getenv("SILK_TEST_NO_SANDBOX") == "1", Width: 800, Height: 600, Zoom: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	do := func(c Command) {
		t.Helper()
		if err := e.Do(c); err != nil {
			t.Fatal(err)
		}
	}
	poll := func(expr string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			var ok bool
			err := e.run(chromedp.Evaluate(expr, &ok))
			if err == nil && ok {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(25 * time.Millisecond):
			}
		}
		t.Fatal("condition failed", expr)
	}
	doc := func() pageview.Document {
		t.Helper()
		d, err := e.Document()
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	byRole := func(d pageview.Document, role string) string {
		t.Helper()
		for _, a := range d.Actions {
			if a.Role == role {
				return a.ID
			}
		}
		t.Fatal("missing role", role)
		return ""
	}
	do(Command{Kind: "navigate", Text: srv.URL})
	poll("window.ticks>=2")
	d := doc()
	var content strings.Builder
	var imageID, loadedID string
	for _, tok := range d.Tokens {
		content.WriteString(tok.Text)
		if tok.Kind == "image" && tok.Source == "canvas" {
			imageID = tok.ID
		}
		if tok.Text == "Loaded picture" {
			loadedID = tok.ID
		}
		if tok.Text == "Broken picture" && (tok.Width != 0 || tok.Height != 0) {
			t.Fatal("broken alt text would be rasterized")
		}
	}
	if !strings.Contains(content.String(), "Real terminal text") || !strings.Contains(content.String(), "Café") {
		t.Fatal(content.String())
	}
	if strings.Contains(content.String(), "Secret hidden text") || strings.Contains(content.String(), "secret") {
		t.Fatal("hidden/password text leaked")
	}
	if imageID == "" {
		t.Fatal("canvas image missing")
	}
	picture, err := e.Picture(imageID, 120, 60)
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Dx() != 120 || picture.Bounds().Dy() != 60 {
		t.Fatal(picture.Bounds())
	}
	r, g, b, _ := picture.At(40, 30).RGBA()
	if r != 0 || g != 0 || b != 65535 {
		t.Fatal("canvas pixels", r, g, b)
	}
	loaded, err := e.Picture(loadedID, 90, 45)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ = loaded.At(40, 20).RGBA()
	if r != 65535 || g != 0 || b != 0 {
		t.Fatal("HTML img pixels", r, g, b)
	}
	// Only this 120x60 image is captured. All the page text came from Document().
	button := byRole(d, "button")
	do(Command{Kind: "activate", ID: button})
	poll("b.textContent==='Clicked 1'")
	d2 := doc()
	found := false
	for _, tok := range d2.Tokens {
		if strings.Contains(tok.Text, "Clicked 1") {
			found = true
		}
	}
	if !found {
		t.Fatal("JS text update missing")
	}
	if byRole(d2, "button") != button {
		t.Fatal("unstable DOM identities")
	}
	field := byRole(d, "text")
	do(Command{Kind: "focus", ID: field})
	do(Command{Kind: "key", ID: field, Text: "Hello"})
	poll("i.value==='Hello' && keys===5")
	do(Command{Kind: "key", ID: field, Text: "ctrl-a"})
	do(Command{Kind: "paste", ID: field, Text: "café 😀"})
	poll("i.value==='café 😀'")
	do(Command{Kind: "paste", ID: byRole(d, "textarea"), Text: strings.Repeat("line\n", 80)})
	poll("t.value.length===400")
	do(Command{Kind: "activate", ID: byRole(d, "checkbox")})
	poll("check.checked")
	do(Command{Kind: "key", ID: byRole(d, "select"), Text: "down"})
	poll("s.value==='Blue'")
	do(Command{Kind: "page-scroll", Y: 1})
	poll("scrollY>0")
	do(Command{Kind: "activate", ID: byRole(d, "link")})
	poll("document.title==='Second'")
	newDoc := doc()
	if newDoc.Epoch == d.Epoch {
		t.Fatal("navigation did not change document identity")
	}
	if err := e.Do(Command{Kind: "activate", ID: button}); err == nil {
		t.Fatal("stale action accepted")
	}
	do(Command{Kind: "back"})
	poll("document.title==='Ready'")
	do(Command{Kind: "forward"})
	poll("document.title==='Second'")
	do(Command{Kind: "reload"})
	poll("document.title==='Second'")
	do(Command{Kind: "resize", Width: 640, Height: 480, Zoom: 1})
	d = doc()
	if len(d.Tokens) == 0 {
		t.Fatal("text missing after resize")
	}
	t.Log("Verified live native-text extraction, hidden/password filtering, bounded image capture, JS updates, trusted activation, form keys, Unicode/multiline paste, checkbox/select, lazy scroll, history and stale-target rejection")
}
