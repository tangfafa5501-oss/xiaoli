import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
const root=fileURLToPath(new URL('../',import.meta.url));
const source=path.resolve(root,'../experiments/companion-motion');
const destination=path.join(root,'web/motion');
const three=path.join(root,'node_modules/three');
const version=JSON.parse(fs.readFileSync(path.join(three,'package.json'),'utf8')).version;
if(version!=='0.180.0')throw new Error('Expected pinned Three.js 0.180.0');
const files=[
 [path.join(source,'motion.mjs'),'motion.mjs'],[path.join(source,'rig.mjs'),'rig.mjs'],
 [path.join(source,'LICENSE'),'LICENSE'],
 [path.join(three,'build/three.module.min.js'),'vendor/three.module.min.js'],
 [path.join(three,'build/three.core.min.js'),'vendor/three.core.min.js'],
 [path.join(three,'LICENSE'),'vendor/LICENSE']
];
const manifest={threeVersion:version,source:'experiments/companion-motion',files:[]};
for(const [src,name] of files){const bytes=fs.readFileSync(src),target=path.join(destination,name);fs.mkdirSync(path.dirname(target),{recursive:true});fs.writeFileSync(target,bytes);manifest.files.push({path:name,bytes:bytes.length,sha256:createHash('sha256').update(bytes).digest('hex')});}
fs.writeFileSync(path.join(destination,'manifest.json'),JSON.stringify(manifest,null,2)+'\n');
console.log(`Built ${files.length} local motion assets; runtime needs no npm or CDN.`);
