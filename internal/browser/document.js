// SPDX-License-Identifier: GPL-3.0-or-later
(() => {
  let state = window.__silkText;
  if (!state) state = window.__silkText = {epoch: Math.random().toString(36).slice(2), next: 0, ids: new WeakMap(), nodes: new Map()};
  state.nodes.clear();
  const result = {url: location.href, title: document.title, epoch: state.epoch, active: '', tokens: [], actions: [], truncated: false};
  const actions = new Set(), seen = new Set();
  let count = 0, chars = 0;
  function id(el) { let key = state.ids.get(el); if (!key) { key = state.epoch + ':' + (++state.next); state.ids.set(el,key); } state.nodes.set(key,el); return key; }
  function emit(token) { chars += (token.text || '').length; if (result.tokens.length < 24000 && chars < 2000000) result.tokens.push(token); else result.truncated=true; }
  function br() { if (result.tokens.length && result.tokens[result.tokens.length-1].kind!=='break') emit({kind:'break'}); }
  function label(el) { return el.getAttribute('aria-label') || Array.from(el.labels || []).map(l=>l.innerText).join(' ') || el.getAttribute('placeholder') || el.name || ''; }
  function action(el,role,editable) { const key=id(el); if (!actions.has(key)) { actions.add(key);result.actions.push({id:key,role,editable,label:label(el)||el.innerText||'',href:el.href||''}); } return key; }
  function walk(node,parentAction='',style='',pre=false) {
    if (++count>50000 || chars>2000000) { result.truncated=true;return; }
    if (node.nodeType===3) { const text=pre?node.nodeValue:node.nodeValue.replace(/\s+/g,' ');if(text)emit({kind:'text',text,action:parentAction,style:pre?'pre':style});return; }
    if(node.nodeType!==1 || seen.has(node))return;seen.add(node);
    const el=node,tag=el.tagName.toLowerCase();
    if(['script','style','noscript','head','template'].includes(tag))return;
    const css=el.ownerDocument.defaultView.getComputedStyle(el);
    if(css.display==='none'||css.visibility==='hidden'||css.visibility==='collapse'||el.hidden||el.getAttribute('aria-hidden')==='true')return;
    const block=['block','flex','grid','list-item','table','table-row'].includes(css.display)||/^h[1-6]$/.test(tag);
    if(block)br();
    if(tag==='br'){br();return;}
    if(tag==='hr'){br();emit({kind:'text',text:'────────────────',style:'muted'});br();return;}
    if(tag==='li')emit({kind:'text',text:'• ',style:'muted'});
    if(/^h[1-6]$/.test(tag))style='heading';else if(tag==='strong'||tag==='b')style='bold';
    pre=pre||tag==='pre';
    let target=parentAction;
    if(tag==='a'&&el.href)target=action(el,'link',false);
    if((tag==='button'||el.getAttribute('role')==='button')&&!el.disabled){target=action(el,'button',false);emit({kind:'control',text:'[ '+(el.innerText||label(el)||'button').replace(/\s+/g,' ')+' ]',action:target,style:'button'});if(block)br();return;}
    if(tag==='input'||tag==='textarea'||tag==='select'||el.isContentEditable){
      if(el.type==='hidden')return;
      const role=tag==='textarea'?'textarea':tag==='select'?'select':el.isContentEditable?'contenteditable':el.type||'text';
      const editable=!el.disabled&&!el.readOnly&&!['checkbox','radio','submit','button','reset','file','image'].includes(role);
      target=el.disabled?'':action(el,role,editable);
      let value=el.value||'';
      if(el.isContentEditable)value=el.innerText;
      if(role==='password')value='•'.repeat(value.length);
      if(role==='checkbox'||role==='radio')value=el.checked?'x':' ';
      if(tag==='select')value=Array.from(el.selectedOptions).map(o=>o.text).join(', ');
      if(['submit','button','reset'].includes(role))value=el.value||role;
      emit({kind:'control',text:'['+(label(el)?label(el)+': ':'')+value.replace(/\s+/g,' ')+(el.disabled?' (disabled)':'')+']',action:target,style:editable?'input':'button'});
      if(block||tag==='textarea')br();return;
    }
    if(['img','canvas','svg'].includes(tag)){
      const r=el.getBoundingClientRect(),key=id(el);
      const broken=tag==='img'&&el.complete&&el.naturalWidth===0;
      emit({kind:'image',id:key,text:el.alt||el.getAttribute('aria-label')||el.querySelector('title')?.textContent||tag,action:target,width:broken?0:r.width||Number(el.naturalWidth)||Number(el.width)||0,height:broken?0:r.height||Number(el.naturalHeight)||Number(el.height)||0,source:el.currentSrc||el.src||tag});br();return;
    }
    if(tag==='iframe') { try {if(el.contentDocument?.body){walk(el.contentDocument.body,target,style,pre);}else{emit({kind:'text',text:'[embedded frame: '+(el.title||el.src||'unavailable')+']',style:'muted'});}}catch(_){emit({kind:'text',text:'[cross-origin embedded frame]',style:'muted'});}br();return; }
    const children=el.shadowRoot?el.shadowRoot.childNodes:el.childNodes;
    for(const child of children){if(child.nodeType===1&&child.tagName.toLowerCase()==='slot'){for(const assigned of child.assignedNodes({flatten:true}))walk(assigned,target,style,pre);}else walk(child,target,style,pre);}
    if(tag==='td'||tag==='th')emit({kind:'text',text:' | ',action:target});
    if(block)br();
  }
  walk(document.body||document.documentElement);
  let active=document.activeElement;while(active?.shadowRoot?.activeElement)active=active.shadowRoot.activeElement;
  if(active&&state.ids.has(active))result.active=state.ids.get(active);
  return result;
})()
