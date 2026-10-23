import http from 'node:http';import fs from 'node:fs';import path from 'node:path';import {fileURLToPath} from 'node:url';
const root=path.dirname(fileURLToPath(import.meta.url)),output=path.join(root,'generated');fs.mkdirSync(output,{recursive:true});
const types={'.html':'text/html; charset=utf-8','.css':'text/css','.mjs':'text/javascript','.js':'text/javascript','.json':'application/json','.md':'text/plain; charset=utf-8','.glb':'model/gltf-binary','.png':'image/png','.webm':'video/webm'};
const allowed=new Set(['companion-motion.glb','motion-preview.webm','preview.png','validation.json']);
const server=http.createServer(async(req,res)=>{
 try{const url=new URL(req.url,'http://localhost');
 if(req.method==='POST'&&url.pathname.startsWith('/save/')){
   const host=req.headers.host;if(req.headers.origin&&req.headers.origin!==`http://${host}`){res.writeHead(403).end();return;}
   const name=url.pathname.slice(6);if(!allowed.has(name)){res.writeHead(400).end();return;}
   const chunks=[];let size=0;for await(const chunk of req){size+=chunk.length;if(size>80*1024*1024){res.writeHead(413).end();return;}chunks.push(chunk);}
   const target=path.join(output,name);fs.writeFileSync(target,Buffer.concat(chunks));console.log('SAVED',name,size);res.writeHead(200,{'Content-Type':'application/json'}).end(JSON.stringify({path:target}));return;
 }
 if(req.method!=='GET'&&req.method!=='HEAD'){res.writeHead(405).end();return;}
 const target=path.resolve(root,'.'+decodeURIComponent(url.pathname==='/'?'/index.html':url.pathname));
 if(!target.startsWith(root+path.sep)){res.writeHead(403).end();return;}
 if(!fs.existsSync(target)||!fs.statSync(target).isFile()){res.writeHead(404).end('Not found');return;}
 const stat=fs.statSync(target);res.writeHead(200,{'Content-Type':types[path.extname(target)]||'application/octet-stream','Content-Length':stat.size,'Cache-Control':'no-store','X-Content-Type-Options':'nosniff'});if(req.method==='HEAD')res.end();else fs.createReadStream(target).pipe(res);
 }catch(e){res.writeHead(500).end('Local server error');console.error(e.message);}
});
server.listen(Number(process.env.MOTION_PORT||0),'127.0.0.1',()=>{const url=`http://127.0.0.1:${server.address().port}`;fs.writeFileSync(path.join(root,'runtime.json'),JSON.stringify({url,pid:process.pid},null,2));console.log('MOTION_DEMO_URL='+url);});
