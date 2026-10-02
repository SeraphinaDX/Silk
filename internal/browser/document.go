// SPDX-License-Identifier: GPL-3.0-or-later

package browser

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/SeraphinaDX/Silk/internal/pageview"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"image"
	_ "image/png"
	"math"
)

//go:embed document.js
var documentScript string

func (e *Engine) Document() (pageview.Document, error) {
	var d pageview.Document
	if err := e.dismissDialogs(); err != nil {
		return d, err
	}
	err := e.run(chromedp.Evaluate(documentScript, &d))
	return d, err
}

func targetScript(id, body string) string {
	b, _ := json.Marshal(id)
	return fmt.Sprintf(`(()=>{const el=window.__silkText?.nodes.get(%s);if(!el||!el.isConnected)throw new Error('Page control is no longer available');%s})()`, b, body)
}

// elementRect resolves an element in the real document, even though the Go
// terminal has reflowed it into different screen coordinates.
func (e *Engine) elementRect(id string) (struct{ X, Y, Width, Height float64 }, error) {
	return e.rect(id, true)
}
func (e *Engine) rect(id string, scroll bool) (struct{ X, Y, Width, Height float64 }, error) {
	var r struct{ X, Y, Width, Height float64 }
	prefix := ""
	if scroll {
		prefix = `el.scrollIntoView({block:'center',inline:'center',behavior:'instant'});`
	}
	script := targetScript(id, prefix+`const r=el.getBoundingClientRect();let x=r.x,y=r.y,w=el.ownerDocument.defaultView;while(w!==window){const f=w.frameElement.getBoundingClientRect();x+=f.x;y+=f.y;w=w.parent;}return {X:x,Y:y,Width:r.width,Height:r.height};`)
	err := e.run(chromedp.Evaluate(script, &r))
	return r, err
}
func (e *Engine) focus(id string) error {
	return e.run(chromedp.Evaluate(targetScript(id, `if(el.ownerDocument.activeElement!==el){el.focus();if(el.setSelectionRange&&['text','search','url','email','tel','password'].includes(el.type)){try{el.setSelectionRange(el.value.length,el.value.length);}catch(_){}}}return true;`), nil))
}

// Picture reads decoded HTML image pixels directly through Chromium. For a
// tainted canvas (cross-origin image) or SVG, capture only its bounding rectangle.
// Captures do not scroll the page or rasterize terminal text.
func (e *Engine) Picture(id string, maxW, maxH int) (image.Image, error) {
	if maxW < 1 || maxH < 1 || maxW > 4096 || maxH > 4096 {
		return nil, fmt.Errorf("invalid image bounds")
	}
	var result struct{ Data, Error string }
	script := targetScript(id, fmt.Sprintf(`return (async()=>{
  if(el.tagName.toLowerCase()!=='img')return {Data:''};
  if(!el.complete)el.loading='eager';
  let timer;
  try {
   await Promise.race([el.decode(),new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('Image loading timed out')),2500)})]);
  } catch(error) {return {Error:'Cannot decode image '+(el.currentSrc||el.src)+': '+error.message};}
  finally {clearTimeout(timer);}
  const canvas=el.ownerDocument.createElementNS('http://www.w3.org/1999/xhtml','canvas');
  const scale=Math.min(1,%d/el.naturalWidth,%d/el.naturalHeight);
  canvas.width=Math.max(1,Math.round(el.naturalWidth*scale));
  canvas.height=Math.max(1,Math.round(el.naturalHeight*scale));
  const ctx=canvas.getContext('2d');
  ctx.fillStyle='white';ctx.fillRect(0,0,canvas.width,canvas.height);
  ctx.drawImage(el,0,0,canvas.width,canvas.height);
  try {return {Data:canvas.toDataURL('image/png').split(',')[1]};}
  catch(error) {if(error.name==='SecurityError')return {Data:''};throw error;}
 })();`, maxW, maxH))
	err := e.run(chromedp.Evaluate(script, &result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }))
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}
	if result.Data != "" {
		data, err := base64.StdEncoding.DecodeString(result.Data)
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		return img, err
	}
	r, err := e.rect(id, false)
	if err != nil {
		return nil, err
	}
	if r.Width <= 0 || r.Height <= 0 {
		return nil, fmt.Errorf("image is not ready")
	}
	var scroll struct{ X, Y float64 }
	if err = e.run(chromedp.Evaluate(`({X:scrollX,Y:scrollY})`, &scroll)); err != nil {
		return nil, err
	}
	scale := math.Min(1, math.Min(float64(maxW)/r.Width, float64(maxH)/r.Height))
	var data []byte
	err = e.run(chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		data, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).WithCaptureBeyondViewport(true).WithClip(&page.Viewport{X: r.X + scroll.X, Y: r.Y + scroll.Y, Width: r.Width, Height: r.Height, Scale: scale}).Do(ctx)
		return err
	}))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
