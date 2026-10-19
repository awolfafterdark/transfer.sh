'use strict';
(() => {
 const $ = s => document.querySelector(s);
 const base = document.body.dataset.base || '';
 const copyStatus = $('#copy-status');
 function fallbackCopy(text) {
  const previous=document.activeElement;
  const box=document.createElement('textarea');box.value=text;box.readOnly=true;box.setAttribute('aria-label','Copy link');document.body.append(box);
  box.select();box.setSelectionRange(0,text.length);
  try { if(!document.execCommand('copy')) throw new Error('Copy unavailable'); }
  finally { box.remove();if(previous && previous.focus)previous.focus(); }
 }
 async function copy(text) {
  try {
   if(navigator.clipboard && window.isSecureContext) {
    try { await navigator.clipboard.writeText(text); } catch (_) { fallbackCopy(text); }
   } else fallbackCopy(text);
   if(copyStatus)copyStatus.textContent='Link copied.';
  } catch (_) { if(copyStatus)copyStatus.textContent='Copy unavailable. Select and copy the link manually.'; }
 }
 document.addEventListener('click', e => { const button = e.target.closest('[data-copy]'); if (button) copy(button.dataset.copy); });
 // Tokenize original source once; create only text nodes and known spans. Uploaded HTML is never parsed.
 document.querySelectorAll('code[data-language]').forEach(code => {
  if (!code.dataset.language) return;
  const source = code.textContent;
  const tokens = /("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`|#[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\/|\b(?:def|class|import|from|return|if|else|elif|for|while|try|except|with|as|in|True|False|None|const|let|var|function|async|await|export|default|new|throw|true|false|null)\b)/g;
  const fragment = document.createDocumentFragment(); let at=0;
  for (const match of source.matchAll(tokens)) {
   fragment.append(document.createTextNode(source.slice(at, match.index)));
   const span = document.createElement('span'); span.className=/^["'`]/.test(match[0])?'syntax-string':/^(#|\/\/|\/\*)/.test(match[0])?'syntax-comment':'syntax-keyword'; span.textContent=match[0]; fragment.append(span); at=match.index+match[0].length;
  }
  fragment.append(document.createTextNode(source.slice(at))); code.replaceChildren(fragment);
 });
 const form=$('#upload-form'); if (!form) return;
 let files=[], busy=false;
 const input=$('#files'), button=$('#upload-button'), progress=$('#progress'), error=$('#error'), status=$('#status');
 function select(selected) {
  if (busy) return;
  files=Array.from(selected); $('#file-list').replaceChildren();
  for (const file of files) { const li=document.createElement('li'); li.textContent=`${file.name} · ${file.size.toLocaleString()} bytes`; $('#file-list').append(li); }
  button.disabled=!files.length; error.textContent='';
 }
 input.addEventListener('change',()=>select(input.files));
 const drop=$('#drop-zone');
 for (const event of ['dragenter','dragover']) drop.addEventListener(event,e=>{e.preventDefault();drop.classList.add('dragging');});
 for (const event of ['dragleave','drop']) drop.addEventListener(event,e=>{e.preventDefault();drop.classList.remove('dragging');});
 drop.addEventListener('drop',e=>select(e.dataTransfer.files));
 function links(raw) {
  const uploaded=new URL(raw.trim());
  // Keep the public browser origin even if the reverse proxy returns an internal origin.
  if (!['https:','http:'].includes(uploaded.protocol) || uploaded.username || uploaded.password || uploaded.search || uploaded.hash) throw new Error('Invalid upload response URL');
  if (!uploaded.pathname.startsWith(base+'/')) throw new Error('Unexpected upload path');
  const parts=uploaded.pathname.slice(base.length+1).split('/');
  if (parts.length!==2 || !/^[A-Za-z0-9_-]+$/.test(parts[0]) || !parts[1]) throw new Error('Unexpected upload path');
  const name=decodeURIComponent(parts[1]);
  if (/[\/\\\x00-\x1f]/.test(name) || name==='.' || name==='..') throw new Error('Invalid uploaded filename');
  const path='/'+parts.join('/');
  return {view:location.origin+base+'/view'+path,direct:location.origin+base+'/get'+path};
 }
 function result(file, urls) {
  const card=document.createElement('article');card.className='result';
  const heading=document.createElement('h2');heading.textContent=file.name;card.append(heading);
  for (const [title,value] of [['VIEW FILE',urls.view],['DIRECT DOWNLOAD',urls.direct]]) {
   const label=document.createElement('label');label.className='link-label';label.textContent=title;
   const row=document.createElement('div');row.className='link-row'; const field=document.createElement('input');field.readOnly=true;field.value=value;field.setAttribute('aria-label',title);field.addEventListener('click',()=>field.select());
   const copyButton=document.createElement('button');copyButton.type='button';copyButton.className='secondary';copyButton.textContent='Copy Link';copyButton.dataset.copy=value;label.append(row);row.append(field,copyButton);card.append(label);
  }
  const actions=document.createElement('div');actions.className='actions';
  for (const [title,href] of [['View File',urls.view],['Download File',urls.direct]]) { const a=document.createElement('a');a.className='button';a.textContent=title;a.href=href;actions.append(a); }
  card.append(actions);$('#results').append(card);
 }
 function upload(file,index) {
  return new Promise((resolve,reject)=>{
   const xhr=new XMLHttpRequest();xhr.open('PUT',base+'/'+encodeURIComponent(file.name));xhr.timeout=30*60*1000;
   xhr.setRequestHeader('Content-Type',file.type || 'application/octet-stream');
   xhr.upload.onprogress=e=>{if(e.lengthComputable){progress.value=((index+e.loaded/e.total)/files.length)*100;status.textContent=`Uploading ${index+1} of ${files.length} · ${Math.round(progress.value)}%`;}};
   xhr.onerror=()=>reject(new Error('Network error. Check your connection and try again.'));
   xhr.ontimeout=()=>reject(new Error('Upload timed out. Try again.'));
   xhr.onload=()=>{if(xhr.status!==200){reject(new Error(`Upload failed (${xhr.status}).`));return;}try{resolve(links(xhr.responseText));}catch(e){reject(e);}};
   xhr.send(file);
  });
 }
 form.addEventListener('submit',async e=>{
  e.preventDefault();if(busy || !files.length)return;busy=true;button.disabled=true;input.disabled=true;error.textContent='';progress.hidden=false;$('#results').replaceChildren();
  let completed=0;
  try {for(let i=0;i<files.length;i++){const urls=await upload(files[i],i);result(files[i],urls);completed++;}}
  catch(e){error.textContent=e.message+(completed?' Previously completed uploads are shown below.':'');}
  finally{busy=false;input.disabled=false;button.disabled=!files.length;progress.hidden=true;status.textContent='';if(completed){$('#upload-screen').hidden=true;$('#results-screen').hidden=false;$('#results-screen h1').textContent=completed===files.length?'✓ Upload Complete':'Some files uploaded';}}
 });
 $('#upload-another').addEventListener('click',()=>{input.value='';select([]);$('#results').replaceChildren();$('#results-screen').hidden=true;$('#upload-screen').hidden=false;error.textContent='';copyStatus.textContent='';});
})();
