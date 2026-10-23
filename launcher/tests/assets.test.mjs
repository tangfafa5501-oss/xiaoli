import test from 'node:test';import assert from 'node:assert/strict';import fs from 'node:fs';import {createHash} from 'node:crypto';
const web=new URL('../web/',import.meta.url);
test('committed offline motion assets match build manifest',()=>{
 const manifest=JSON.parse(fs.readFileSync(new URL('motion/manifest.json',web)));assert.equal(manifest.threeVersion,'0.180.0');assert.equal(manifest.files.length,6);
 for(const entry of manifest.files){const bytes=fs.readFileSync(new URL('motion/'+entry.path,web));assert.equal(bytes.length,entry.bytes);assert.equal(createHash('sha256').update(bytes).digest('hex'),entry.sha256);}
});
test('real launcher page loads chat without a hard dependency on renderer',()=>{
 const html=fs.readFileSync(new URL('index.html',web),'utf8'),ui=fs.readFileSync(new URL('chat-ui.mjs',web),'utf8'),avatar=fs.readFileSync(new URL('avatar.mjs',web),'utf8');
 assert.ok(html.includes('src="./chat-ui.mjs"'));assert.ok(html.includes('./motion/vendor/three.module.min.js'));assert.ok(ui.includes("import('./avatar.mjs')"));assert.ok(ui.includes('.catch(failAvatar)'));assert.ok(avatar.includes('pose.mouth=0'));
 assert.ok(!/https?:\/\//.test(html));assert.ok(!/https?:\/\//.test(ui));assert.ok(fs.readFileSync(new URL('../main.go',import.meta.url),'utf8').includes('//go:embed web/*'));
});
