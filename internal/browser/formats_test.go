// SPDX-License-Identifier: GPL-3.0-or-later
package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/chromedp/chromedp"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestJPEGAndAVIF(t *testing.T) {
	if os.Getenv("SILK_INTEGRATION") == "" {
		t.Skip("set SILK_INTEGRATION=1")
	}
	fixtures := map[string][]byte{}
	for _, format := range []string{"jpeg", "avif"} {
		b, err := os.ReadFile("testdata/" + format + ".b64")
		if err != nil {
			t.Fatal(err)
		}
		fixtures[format], err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil {
			t.Fatal(err)
		}
	}
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fixtures["jpeg"])
	}))
	defer foreign.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format := strings.TrimPrefix(r.URL.Path, "/")
		if b, ok := fixtures[format]; ok {
			w.Header().Set("Content-Type", "image/"+format)
			w.Write(b)
			return
		}
		switch r.URL.Path {
		case "/xhtml":
			w.Header().Set("Content-Type", "application/xhtml+xml")
			fmt.Fprint(w, `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Formats</title><style>html{scroll-behavior:smooth}img{display:block;max-width:100%;height:auto}p{margin:300px 0}</style></head><body><p>Native text above</p><img src="/jpeg" alt="JPEG"/><img src="/avif" alt="AVIF"/><p>Native text below</p></body></html>`)
		case "/cross-origin":
			fmt.Fprintf(w, `<!doctype html><title>Cross origin</title><div style="height:10000px">Native text above</div><img src="%s" width=96 height=48 alt=Foreign>`, foreign.URL)
		case "/lazy":
			fmt.Fprint(w, `<!doctype html><title>Lazy</title><div style="height:10000px">Text</div><img loading=lazy src=/avif width=96 height=48 alt=Lazy>`)
		default:
			fmt.Fprint(w, `<!doctype html><title>Formats</title><style>html{scroll-behavior:smooth}img{display:block;max-width:100%;height:auto}p{margin:300px 0}</style><p>Native text above</p><img src=/jpeg alt=JPEG><img src=/avif alt=AVIF><p>Native text below</p>`)
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	e, err := New(ctx, Options{Chrome: os.Getenv("SILK_CHROME"), NoSandbox: os.Getenv("SILK_TEST_NO_SANDBOX") == "1", Width: 800, Height: 600, Zoom: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	for _, path := range []string{"/", "/xhtml", "/lazy", "/cross-origin"} {
		t.Run(path, func(t *testing.T) {
			if err = e.Do(Command{Kind: "navigate", Text: srv.URL + path}); err != nil {
				t.Fatal(err)
			}
			d, err := e.Document()
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, tok := range d.Tokens {
				if tok.Kind != "image" {
					continue
				}
				t.Run(tok.Text, func(t *testing.T) {
					if tok.Width != 96 || tok.Height != 48 {
						t.Fatalf("no image slot: %+v", tok)
					}
					img, err := e.Picture(tok.ID, 96, 48)
					if err != nil {
						t.Fatal(err)
					}
					r, g, b, _ := img.At(40, 20).RGBA()
					if r < 180*257 || g > 60*257 || b < 50*257 || b > 110*257 {
						t.Fatalf("undecoded image: %d %d %d", r>>8, g>>8, b>>8)
					}
					var scroll float64
					if err = e.run(chromedp.Evaluate(`scrollY`, &scroll)); err != nil {
						t.Fatal(err)
					}
					if scroll != 0 {
						t.Fatalf("capture scrolled the original page: %v", scroll)
					}
				})
				found++
			}
			expected := 2
			if path == "/lazy" || path == "/cross-origin" {
				expected = 1
			}
			if found != expected {
				t.Fatalf("found %d images, expected %d", found, expected)
			}
		})
	}
}
