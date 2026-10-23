import fs from 'node:fs';
import {GLTFLoader} from './node_modules/three/examples/jsm/loaders/GLTFLoader.js';
import {AnimationMixer,Vector3} from './node_modules/three/build/three.module.js';
const file=new URL('./output/companion-motion.glb',import.meta.url),bytes=fs.readFileSync(file);
if(bytes.readUInt32LE(0)!==0x46546c67||bytes.readUInt32LE(4)!==2||bytes.readUInt32LE(8)!==bytes.length)throw new Error('Invalid GLB header');
const jsonLength=bytes.readUInt32LE(12),json=JSON.parse(bytes.subarray(20,20+jsonLength).toString());
const gltf=await new GLTFLoader().parseAsync(bytes.buffer.slice(bytes.byteOffset,bytes.byteOffset+bytes.byteLength),'');
let skinnedMeshes=0,bones=0;gltf.scene.traverse(o=>{if(o.isSkinnedMesh)skinnedMeshes++;if(o.isBone)bones++;});
if(skinnedMeshes<1||bones!==35||gltf.animations.length!==9)throw new Error('Rig or clip count mismatch');
const mixer=new AnimationMixer(gltf.scene),hand=gltf.scene.getObjectByName('rightHand');
const wave=gltf.animations.find(a=>a.name==='wave');mixer.clipAction(wave).play();mixer.setTime(0);gltf.scene.updateMatrixWorld(true);const start=hand.getWorldPosition(new Vector3());mixer.setTime(1.5);gltf.scene.updateMatrixWorld(true);const moved=hand.getWorldPosition(new Vector3()).distanceTo(start);
if(moved<.35)throw new Error('Wave has no significant 3D wrist displacement');
for(const clip of gltf.animations){mixer.stopAllAction();mixer.clipAction(clip).reset().play();for(let i=0;i<30;i++){mixer.setTime(clip.duration*i/30);gltf.scene.updateMatrixWorld(true);gltf.scene.traverse(o=>{if(!o.matrixWorld.elements.every(Number.isFinite))throw new Error('Non-finite transform in '+clip.name);});}}
const idle=gltf.animations.find(a=>a.name==='idle');let seam=0;for(const track of idle.tracks){const n=track.getValueSize();for(let i=0;i<n;i++)seam=Math.max(seam,Math.abs(track.values[i]-track.values[track.values.length-n+i]));}if(seam>1e-5)throw new Error('Idle loop seam: '+seam);
const report={format:'glTF 2.0 binary',bytes:bytes.length,skinnedMeshes,bones,skins:json.skins?.length,clipCount:gltf.animations.length,waveWristDisplacementMetres:moved,idleLoopMaxEndpointDelta:seam,animationSamplesChecked:270,clips:gltf.animations.map(a=>({name:a.name,duration:a.duration,tracks:a.tracks.length})),passed:true};
fs.mkdirSync(new URL('./generated/',import.meta.url),{recursive:true});
fs.writeFileSync(new URL('./generated/glb-validation.json',import.meta.url),JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
