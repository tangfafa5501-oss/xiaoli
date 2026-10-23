import * as THREE from 'three';
import {OrbitControls} from 'three/addons/controls/OrbitControls.js';
import {GLTFExporter} from 'three/addons/exporters/GLTFExporter.js';
import {createCharacter} from './rig.mjs';
import {ACTIONS,DEMO,MotionEngine,actionPose,microPose,neutral,blendPose,smooth} from './motion.mjs';
import {runTests} from './tests.mjs';

const $=id=>document.getElementById(id), error=message=>{$('error').hidden=false;$('error').textContent=message;};
window.addEventListener('error',e=>error(e.message));
window.addEventListener('unhandledrejection',e=>error(String(e.reason)));
let renderer;
try{renderer=new THREE.WebGLRenderer({antialias:true,preserveDrawingBuffer:true});}catch(e){error('无法建立 WebGL 场景。请使用支持 WebGL 的现代浏览器。'+e.message);throw e;}
renderer.setPixelRatio(Math.min(devicePixelRatio,1.75));renderer.shadowMap.enabled=true;renderer.shadowMap.type=THREE.PCFSoftShadowMap;renderer.outputColorSpace=THREE.SRGBColorSpace;renderer.toneMapping=THREE.ACESFilmicToneMapping;renderer.toneMappingExposure=1.15;
$('viewport').appendChild(renderer.domElement);
const scene=new THREE.Scene();scene.background=new THREE.Color('#ece8e2');scene.fog=new THREE.Fog('#ece8e2',6,14);
const camera=new THREE.PerspectiveCamera(31,1,.05,30);camera.position.set(.18,1.65,4.9);
const controls=new OrbitControls(camera,renderer.domElement);controls.target.set(0,.9,0);controls.enableDamping=true;controls.minDistance=2.2;controls.maxDistance=7;controls.maxPolarAngle=Math.PI*.52;controls.enablePan=false;
scene.add(new THREE.HemisphereLight('#fff6ee','#b9aebd',2.5));
const key=new THREE.DirectionalLight('#fff4e5',3.3);key.position.set(-3,5,4);key.castShadow=true;key.shadow.mapSize.set(2048,2048);key.shadow.camera.left=-2;key.shadow.camera.right=2;key.shadow.camera.top=3;key.shadow.camera.bottom=-2;key.shadow.normalBias=.025;key.shadow.bias=-.0002;key.shadow.radius=4;scene.add(key);
const fill=new THREE.DirectionalLight('#e7deff',1.4);fill.position.set(3,2,-2);scene.add(fill);
const floor=new THREE.Mesh(new THREE.PlaneGeometry(100,100),new THREE.MeshStandardMaterial({color:'#e8e2d9',roughness:1}));floor.rotation.x=-Math.PI/2;floor.receiveShadow=true;scene.add(floor);
const platform=new THREE.Mesh(new THREE.CylinderGeometry(.66,.68,.032,80),new THREE.MeshStandardMaterial({color:'#ded4df',roughness:.85}));platform.position.y=-.018;platform.receiveShadow=true;scene.add(platform);
const rig=createCharacter();scene.add(rig.root);
const helper=new THREE.SkeletonHelper(rig.root);helper.visible=false;helper.material.depthTest=false;helper.renderOrder=20;scene.add(helper);
const engine=new MotionEngine();let paused=false,speed=1,demo=false,demoIndex=0,demoTime=0,side=false,recording=false,frames=0,statsTime=0;
let last=performance.now(),elapsed=0;const pointer={x:0,y:0};
function resize(){const r=$('viewport').getBoundingClientRect();renderer.setSize(r.width,r.height);camera.aspect=r.width/r.height;camera.updateProjectionMatrix();}
new ResizeObserver(resize).observe($('viewport'));resize();
function updateState(){const a=ACTIONS[engine.id];$('state-name').textContent=a.label;$('state-note').textContent=a.note;document.querySelectorAll('[data-action]').forEach(b=>{b.classList.toggle('selected',b.dataset.action===engine.id);b.setAttribute('aria-pressed',String(b.dataset.action===engine.id));});}
function choose(id){demo=false;$('demo').classList.remove('active');$('demo').innerHTML='播放完整互动 <span>↗</span>';engine.request(id,{manual:true});paused=false;updatePause();updateState();}
function updatePause(){$('pause').textContent=paused?'▶':'Ⅱ';$('pause').setAttribute('aria-label',paused?'继续播放':'暂停播放');}
function startDemo(){demo=true;demoIndex=0;demoTime=0;paused=false;engine.request(DEMO[0][0],{manual:true});$('demo').classList.add('active');$('demo').innerHTML='互动演示中 <span>↗</span>';updatePause();updateState();}
Object.entries(ACTIONS).filter(([k])=>k!=='idle').forEach(([id,a],i)=>{const b=document.createElement('button');b.dataset.action=id;b.innerHTML=`<span>0${i+1}</span>${a.label}`;b.onclick=()=>choose(id);$('actions').appendChild(b);});
$('demo').onclick=startDemo;$('pause').onclick=()=>{paused=!paused;updatePause();};$('reset').onclick=()=>choose('idle');$('interrupt').onclick=()=>choose('listen');
$('skeleton').onchange=e=>helper.visible=e.target.checked;
$('intensity').oninput=e=>{engine.intensity=Number(e.target.value);$('amount').value=`${Math.round(engine.intensity*100)}%`;};
$('speed').oninput=e=>{speed=Number(e.target.value);$('rate').value=speed.toFixed(1)+'×';};
$('seek').oninput=e=>{demo=false;paused=true;engine.age=Number(e.target.value)/100*ACTIONS[engine.id].duration;engine.pose=actionPose(engine.id,engine.age,engine.variant);engine.from=structuredClone(engine.pose);engine.blend=100;rig.apply(engine.pose,microPose(engine.age));$('phase').value=e.target.value+'%';$('demo').classList.remove('active');$('demo').innerHTML='播放完整互动 <span>↗</span>';updatePause();};
$('view').onclick=()=>{side=!side;camera.position.set(side?3.5:.18,1.65,side?3.4:4.9);$('view').textContent=side?'回到正面':'切换侧面';};
renderer.domElement.addEventListener('pointermove',e=>{const r=renderer.domElement.getBoundingClientRect();pointer.x=((e.clientX-r.left)/r.width-.5)*.09;pointer.y=-((e.clientY-r.top)/r.height-.5)*.04;});
renderer.domElement.addEventListener('pointerleave',()=>{pointer.x=0;pointer.y=0;});
document.addEventListener('keydown',e=>{if(e.target.matches('input,button,summary'))return;if(e.code==='Space'){e.preventDefault();paused=!paused;updatePause();}const ids=['wave','listen','think','speak','happy','confused','nod','bye'];if(ids[Number(e.key)-1])choose(ids[Number(e.key)-1]);});
document.addEventListener('visibilitychange',()=>{last=performance.now();});
async function save(name,blob){const response=await fetch('/save/'+name,{method:'POST',headers:{'Content-Type':blob.type||'application/octet-stream'},body:blob});if(!response.ok)throw new Error('本地保存失败：'+response.status);const data=await response.json();$('result').textContent='已保存：'+data.path;return data;}
$('snapshot').onclick=()=>renderer.domElement.toBlob(blob=>save('preview.png',blob).catch(e=>error(e.message)),'image/png');
$('test').onclick=async()=>{const report=runTests();report.webgl={version:renderer.capabilities.isWebGL2?'WebGL2':'WebGL',triangles:renderer.info.render.triangles,calls:renderer.info.render.calls,bones:Object.keys(rig.bones).length};$('result').textContent=`动作测试：${report.passed}/${report.total} 通过`;await save('validation.json',new Blob([JSON.stringify(report,null,2)],{type:'application/json'}));};

async function bake(){
  const sampleRig=createCharacter(), clips=[],fps=30;
  // Quaternion, position and scale capture includes eye blink, pupils and mouth.
  const nodes=[...sampleRig.animated,sampleRig.body];
  for(const [id,a] of Object.entries(ACTIONS)){
    const times=[],values=nodes.map(()=>({q:[],p:[],s:[]}));
    const n=Math.ceil(a.duration*fps);
    for(let i=0;i<=n;i++){
      const t=Math.min(i/fps,a.duration);times.push(t);
      let pose=actionPose(id,t,0),micro=microPose(t);
      if(id==='idle'){
        const phase=2*Math.PI*t/a.duration;
        micro={breath:.6*Math.sin(phase*2)+.4*Math.cos(phase),blink:micro.blink,yaw:.012*Math.sin(phase),roll:.009*Math.sin(phase*2),gaze:[.025*Math.sin(phase),.016*Math.sin(phase*2)]};
      }else{
        const envelope=smooth(t/a.blend)*(1-smooth((t-(a.duration-.4))/.4));pose=blendPose(neutral(),pose,envelope);
      }
      sampleRig.apply(pose,micro);
      nodes.forEach((node,j)=>{values[j].q.push(...node.quaternion.toArray());values[j].p.push(...node.position.toArray());values[j].s.push(...node.scale.toArray());});
    }
    const tracks=[];nodes.forEach((node,j)=>{tracks.push(new THREE.QuaternionKeyframeTrack(node.name+'.quaternion',times,values[j].q),new THREE.VectorKeyframeTrack(node.name+'.position',times,values[j].p),new THREE.VectorKeyframeTrack(node.name+'.scale',times,values[j].s));});
    clips.push(new THREE.AnimationClip(id,a.duration,tracks).optimize());
  }
  sampleRig.apply(neutral());const exporter=new GLTFExporter();const buffer=await exporter.parseAsync(sampleRig.root,{binary:true,animations:clips,onlyVisible:true});
  sampleRig.root.traverse(o=>{if(o.geometry)o.geometry.dispose();if(o.material)o.material.dispose();});return buffer;
}
$('export').onclick=async()=>{const b=$('export');b.disabled=true;$('result').textContent='正在以 30 fps 烘焙 9 段动作…';try{const buffer=await bake();await save('companion-motion.glb',new Blob([buffer],{type:'model/gltf-binary'}));}catch(e){error(e.message);}finally{b.disabled=false;}};
$('record').onclick=async()=>{
 if(recording)return;
 if(!renderer.domElement.captureStream||!window.MediaRecorder){error('当前浏览器不支持录制，请在 Chrome 打开此本地演示。');return;}
 recording=true;$('record').disabled=true;$('result').textContent='正在录制 24 秒：问候 → 聆听 → 思考 → 说话 → 确认';
 speed=1;$('speed').value=1;$('rate').value='1.0×';engine.intensity=1;$('intensity').value=1;$('amount').value='100%';startDemo();
 const stream=renderer.domElement.captureStream(30),chunks=[];
 const mime=['video/webm;codecs=vp9','video/webm;codecs=vp8','video/webm'].find(x=>MediaRecorder.isTypeSupported(x));
 const recorder=new MediaRecorder(stream,{mimeType:mime,videoBitsPerSecond:4000000});
 recorder.ondataavailable=e=>{if(e.data.size)chunks.push(e.data);};
 recorder.onstop=async()=>{stream.getTracks().forEach(t=>t.stop());try{await save('motion-preview.webm',new Blob(chunks,{type:'video/webm'}));}catch(e){error(e.message);}recording=false;$('record').disabled=false;};
 recorder.start(1000);setTimeout(()=>recorder.stop(),24000);
};
function tick(now){requestAnimationFrame(tick);const raw=Math.min((now-last)/1000,.1);last=now;if(document.hidden&&!recording)return;elapsed+=raw;
 if(!paused){const dt=raw*speed;if(demo){demoTime+=dt;if(demoTime>=DEMO[demoIndex][1]){demoTime=0;demoIndex++;if(demoIndex>=DEMO.length){demo=false;$('demo').classList.remove('active');$('demo').innerHTML='再次播放互动 <span>↗</span>';}else engine.request(DEMO[demoIndex][0],{manual:true});}}
   const old=engine.id;engine.update(dt);if(old!==engine.id)updateState();const micro=microPose(engine.time);micro.gaze[0]+=pointer.x;micro.gaze[1]+=pointer.y;rig.apply(engine.pose,micro);
 }
 controls.update();renderer.render(scene,camera);frames++;statsTime+=raw;
 if(statsTime>.4){$('time').textContent=engine.age.toFixed(1)+' s';$('progress').style.width=Math.min(engine.age/ACTIONS[engine.id].duration,1)*100+'%';$('metrics').textContent=`WebGL · ${Object.keys(rig.bones).length} 骨骼 · ${renderer.info.render.triangles.toLocaleString()} 三角形 · ${Math.round(frames/statsTime)} fps`;
   frames=0;statsTime=0;updateState();}
}
rig.apply(neutral(),microPose(0));updateState();requestAnimationFrame(tick);
