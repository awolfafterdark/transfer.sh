const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const base = process.env.TEST_BASE_URL || 'http://127.0.0.1:8080';
(async () => {
 const browser = await chromium.launch({headless:true,args:['--no-sandbox']});
 try {
  for (const mobile of [false,true]) {
   const context = await browser.newContext({acceptDownloads:true,viewport:mobile?{width:390,height:844}:{width:1280,height:900},isMobile:mobile,hasTouch:mobile,extraHTTPHeaders:{'ngrok-skip-browser-warning':'integration-test'}});
   const page=await context.newPage();const errors=[];page.on('pageerror', e=>errors.push(e.message));
   await page.goto(base);
   const payload=Buffer.from('browser upload verification\n192.0.2.10:8080\n');
   await page.locator('#files').setInputFiles({name:'browser file.txt',mimeType:'text/plain',buffer:payload});
   await page.getByRole('button',{name:'Upload Files'}).click();
   await page.locator('#results-screen:not([hidden])').waitFor();
   const view=await page.getByRole('textbox',{name:'VIEW FILE',exact:true}).inputValue();
   const direct=await page.getByRole('textbox',{name:'DIRECT DOWNLOAD',exact:true}).inputValue();
   assert.match(view,/\/view\/[^/]+\/browser%20file.txt$/);
   assert.match(direct,/\/get\/[^/]+\/browser%20file.txt$/);
   assert.equal(await page.getByRole('link',{name:'View File',exact:true}).getAttribute('href'),view);
   assert.equal(await page.getByRole('link',{name:'Download File',exact:true}).getAttribute('href'),direct);
   // Clipboard fallback works even on HTTP; stub only the OS clipboard transport.
   await page.evaluate(()=>{document.execCommand=()=>true;});
   await page.getByRole('button',{name:'Copy Link',exact:true}).first().click();
   await page.waitForFunction(()=>document.querySelector('#copy-status').textContent.includes('copied'));
   const download1=page.waitForEvent('download');await page.getByRole('link',{name:'Download File',exact:true}).click();
   assert.deepEqual(await fs.readFile(await (await download1).path()),payload);
   await page.getByRole('link',{name:'View File',exact:true}).click();
   await page.locator('.text-preview').waitFor();assert.equal(await page.locator('.text-preview').textContent(),payload.toString());
   assert.equal(await page.locator('#download-file').getAttribute('href'),direct);
   const download2=page.waitForEvent('download');await page.locator('#download-file').click();
   assert.deepEqual(await fs.readFile(await (await download2).path()),payload);
   assert.deepEqual(errors,[]);
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
   console.log(`PASS: ${mobile?'mobile':'desktop'} browser upload, dual links, preview, copy, and original downloads`);
   await context.close();
  }
 } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exitCode=1;});
