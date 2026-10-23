// Local fixture transport check; this does not execute Go, WebGL or a model.
import {spawn} from 'node:child_process';
import fs from 'node:fs';
import assert from 'node:assert/strict';
const child=spawn(process.execPath,[new URL('./fixture-server.mjs',import.meta.url).pathname],{stdio:['ignore','pipe','inherit']});
try {
  const base=await new Promise((resolve,reject)=>{
    const timer=setTimeout(()=>reject(Error('Fixture startup timeout')),5000);
    child.once('error',reject);child.once('exit',code=>reject(Error('Fixture exited '+code)));
    child.stdout.on('data',data=>{const match=data.toString().match(/FIXTURE_UI=(\S+)/);if(match){clearTimeout(timer);resolve(match[1]);}});
  });
  const checks=[];
  for(const route of ['/','/chat-ui.mjs','/chat-session.mjs','/avatar.mjs','/motion/rig.mjs','/motion/motion.mjs','/motion/vendor/three.module.min.js','/motion/vendor/three.core.min.js','/__test']) {
    const response=await fetch(base+route);assert.equal(response.status,200,route);
    const bytes=(await response.arrayBuffer()).byteLength;assert.ok(bytes>0);
    if(/\.(mjs|js)$/.test(route))assert.match(response.headers.get('content-type'),/javascript/);
    checks.push({route,status:response.status,bytes});
  }
  const config=async scenario=>{const r=await fetch(base+'/__test/config',{method:'POST',body:JSON.stringify({scenario})});assert.equal(r.status,200);};
  const chat=()=>fetch(base+'/api/chat',{method:'POST',body:JSON.stringify({messages:[{role:'user',content:'fixture check'}]})});
  await config('normal');const normal=await chat();const text=await normal.text();assert.match(text,/模拟接口/);assert.match(text,/\[DONE\]/);
  await config('error');assert.equal((await chat()).status,500);
  const report={checks,sseFixture:true,errorFixture:true,realGoProxy:false,realModel:false,browser:false};
  fs.writeFileSync(new URL('../.test-output/http-validation.json',import.meta.url),JSON.stringify(report,null,2));
  console.log('HTTP fixture smoke passed: 9 routes, SSE completion and HTTP error. Not a browser/model test.');
} finally {child.kill('SIGTERM');}
