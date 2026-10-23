// QA only. Serves the unchanged production chat page with a deterministic local SSE fixture.
// This file is outside web/ and is NOT embedded in the Go launcher.
import http from 'node:http';import fs from 'node:fs';import path from 'node:path';import {fileURLToPath} from 'node:url';
const root=path.resolve(fileURLToPath(new URL('../web/',import.meta.url))),tests=fileURLToPath(new URL('./',import.meta.url));
const out=fileURLToPath(new URL('../.test-output/',import.meta.url));fs.mkdirSync(out,{recursive:true});
let scenario='normal',assetFail=false,count=0;const turns=[];
const mime={'.html':'text/html; charset=utf-8','.mjs':'text/javascript','.js':'text/javascript','.json':'application/json','.jpg':'image/jpeg','.png':'image/png','.css':'text/css'};
const server=http.createServer(async(req,res)=>{
 const url=new URL(req.url,'http://localhost');
 if(url.pathname==='/__test/config'&&req.method==='POST'){let raw='';for await(const c of req)raw+=c;const input=JSON.parse(raw);scenario=input.scenario||'normal';assetFail=Boolean(input.assetFail);res.end('{}');return;}
 if(url.pathname==='/__test/stats'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({count,turns}));return;}
 if(url.pathname==='/__test/evidence'&&req.method==='POST'){const name=url.searchParams.get('name');if(!['browser-validation.json','chat-motion-fixture.png'].includes(name)){res.writeHead(400).end();return;}const chunks=[];for await(const c of req)chunks.push(c);fs.writeFileSync(path.join(out,name),Buffer.concat(chunks));res.end('saved');return;}
 if(url.pathname==='/api/status'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({ready:true,failed:false}));return;}
 if(url.pathname==='/api/alive'){res.writeHead(200,{'Content-Type':'text/event-stream'});res.write(': fixture\n\n');return;}
 if(url.pathname==='/api/chat'){
   let raw='';for await(const c of req)raw+=c;const input=JSON.parse(raw);const turn={scenario,messages:input.messages,aborted:false};turns.push(turn);count++;
   const mode=scenario;if(mode==='error'){res.writeHead(500).end('fixture error');return;}
   res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-store'});
   const timers=[];let ended=false;res.on('close',()=>{if(!ended)turn.aborted=true;timers.forEach(clearTimeout);});
   const emit=text=>res.write('data: '+JSON.stringify({choices:[{delta:{content:text}}]})+'\n\n');
   const later=(t,f)=>timers.push(setTimeout(f,t));
   if(mode==='empty'){later(700,()=>{ended=true;res.end('data: [DONE]\n\n');});return;}
   if(mode==='slow'){later(15000,()=>{emit('这条用于取消测试。');ended=true;res.end('data: [DONE]\n\n');});return;}
   later(1200,()=>emit('这是本地模拟接口的回复，'));
   later(2300,()=>emit('用于验证小离原界面的聊天与动作状态。'));
   later(3700,()=>{ended=true;res.end(mode==='truncated'?'':'data: [DONE]\n\n');});return;
 }
 if(assetFail&&url.pathname==='/avatar.mjs'){res.writeHead(404).end('Intentional renderer load failure for QA');return;}
 const file=url.pathname==='/__test'?path.join(tests,'browser-harness.html'):path.resolve(root,'.'+decodeURIComponent(url.pathname==='/'?'/index.html':url.pathname));
 if(file!==path.join(tests,'browser-harness.html')&&!file.startsWith(root+path.sep)){res.writeHead(403).end();return;}
 if(!fs.existsSync(file)||!fs.statSync(file).isFile()){res.writeHead(404).end();return;}
 res.setHeader('Content-Type',mime[path.extname(file)]||'text/plain');res.setHeader('Cache-Control','no-store');
 let bytes=fs.readFileSync(file);
 if(file===path.join(root,'index.html')){
  const notice='<div style="position:fixed;top:0;left:0;z-index:999;background:#422642;color:white;font:11px sans-serif;padding:4px 10px;pointer-events:none">验收环境：模拟 SSE 接口，非真实模型对话</div>';
  bytes=Buffer.from(bytes.toString().replace('<body>','<body>'+notice));
 }
 res.end(bytes);
});
server.listen(0,'127.0.0.1',()=>{const url=`http://127.0.0.1:${server.address().port}`;fs.writeFileSync(path.join(out,'server.json'),JSON.stringify({url,pid:process.pid}));console.log('FIXTURE_UI='+url);console.log('BROWSER_TESTS='+url+'/__test');});
