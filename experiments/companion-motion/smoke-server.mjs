import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import fs from 'node:fs';
import {fileURLToPath} from 'node:url';

const root=fileURLToPath(new URL('./',import.meta.url));
const child=spawn(process.execPath,['server.mjs'],{cwd:root,env:{...process.env,MOTION_PORT:'0'},stdio:['ignore','pipe','pipe']});
let stderr='';child.stderr.on('data',chunk=>stderr+=chunk);
const saved=new URL('./generated/validation.json',import.meta.url);
const original=fs.existsSync(saved)?fs.readFileSync(saved):null;
try {
  const url=await new Promise((resolve,reject)=>{
    let text='';const timer=setTimeout(()=>reject(new Error('Local server startup timed out: '+stderr)),10000);
    child.stdout.on('data',chunk=>{text+=chunk;const match=text.match(/MOTION_DEMO_URL=(http:\/\/127\.0\.0\.1:\d+)/);if(match){clearTimeout(timer);resolve(match[1]);}});
    child.once('error',e=>{clearTimeout(timer);reject(e);});
    child.once('exit',code=>{clearTimeout(timer);reject(new Error('Server exited '+code+': '+stderr));});
  });
  const routes=[['/','text/html'],['/app.mjs','text/javascript'],['/node_modules/three/build/three.module.js','text/javascript'],['/观看动作预览.html','text/html'],['/动作规格与接入说明.md','text/plain'],['/output/companion-motion.glb','model/gltf-binary'],['/output/motion-preview.webm','video/webm']];
  for(const [route,type] of routes){const res=await fetch(url+route);assert.equal(res.status,200,route);assert.ok(res.headers.get('content-type').startsWith(type));const body=await res.arrayBuffer();assert.ok(body.byteLength>0);}
  const head=await fetch(url+'/output/motion-preview.webm',{method:'HEAD'});assert.equal(head.status,200);assert.equal(Number(head.headers.get('content-length')),2191723);assert.equal((await head.arrayBuffer()).byteLength,0);
  assert.equal((await fetch(url+'/not-a-file')).status,404);
  assert.equal((await fetch(url+'/save/validation.json',{method:'POST',headers:{Origin:'https://example.invalid'},body:'{}'})).status,403);
  const sample=JSON.stringify({smokeTest:true});const res=await fetch(url+'/save/validation.json',{method:'POST',headers:{Origin:url,'Content-Type':'application/json'},body:sample});assert.equal(res.status,200);assert.equal(fs.readFileSync(saved,'utf8'),sample);
  assert.equal((await fetch(url+'/save/not-allowed.txt',{method:'POST',headers:{Origin:url},body:'no'})).status,400);
  console.log(JSON.stringify({serverSmoke:'passed',platform:process.platform,routes:routes.length,head:true,localSave:true,crossOriginRejected:true}));
} finally {
  if(original===null){if(fs.existsSync(saved))fs.unlinkSync(saved);}else fs.writeFileSync(saved,original);
  if(child.exitCode===null){const done=once(child,'exit');child.kill();await done;}
}
