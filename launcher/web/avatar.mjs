import * as THREE from 'three';
import {createCharacter} from './motion/rig.mjs';
import {MotionEngine,microPose} from './motion/motion.mjs';

export function createAvatar(container,{onFailure=()=>{}}={}) {
  const renderer=new THREE.WebGLRenderer({alpha:true,antialias:true});
  renderer.setPixelRatio(Math.min(devicePixelRatio,1.5));renderer.setClearColor(0,0);
  renderer.outputColorSpace=THREE.SRGBColorSpace;renderer.toneMapping=THREE.ACESFilmicToneMapping;renderer.toneMappingExposure=1.15;
  const scene=new THREE.Scene();scene.add(new THREE.HemisphereLight('#fff4df','#a8a1b9',3));
  const key=new THREE.DirectionalLight('#fff2df',3);key.position.set(-3,4,5);scene.add(key);
  const fill=new THREE.DirectionalLight('#ddd6ff',1.5);fill.position.set(3,2,-2);scene.add(fill);
  const rig=createCharacter();scene.add(rig.root);
  const camera=new THREE.PerspectiveCamera(31,1,.05,20);camera.position.set(.18,1.45,4.4);camera.lookAt(0,.98,0);
  const engine=new MotionEngine();let state='idle',stopped=false,frame=0,last=performance.now();
  container.appendChild(renderer.domElement);renderer.domElement.setAttribute('aria-hidden','true');
  const resize=()=>{const {width,height}=container.getBoundingClientRect();if(width&&height){renderer.setSize(width,height);camera.aspect=width/height;camera.updateProjectionMatrix();}};
  const observer=new ResizeObserver(resize);observer.observe(container);resize();
  const fail=()=>{if(stopped)return;stopped=true;cancelAnimationFrame(frame);observer.disconnect();onFailure();};
  renderer.domElement.addEventListener('webglcontextlost',event=>{event.preventDefault();fail();});
  function tick(now) {
    if(stopped)return;frame=requestAnimationFrame(tick);
    const dt=Math.min((now-last)/1000,.06);last=now;if(document.hidden)return;
    try {
      if(state==='thinking')engine.age=Math.min(engine.age,2.8); // hold the thoughtful pose until text arrives
      const pose=engine.update(dt);
      if(state==='expressing'&&engine.id==='idle')engine.request('speak',{manual:true});
      pose.mouth=0; // Text-only milestone: deliberately no voice or mouth animation.
      rig.apply(pose,microPose(engine.time));renderer.render(scene,camera);
    } catch {fail();}
  }
  tick(performance.now());
  return {
    setState(next){if(stopped||next===state)return;state=next;engine.request({idle:'idle',thinking:'think',expressing:'speak'}[next]||'idle',{manual:true});},
    dispose(){stopped=true;cancelAnimationFrame(frame);observer.disconnect();renderer.dispose();container.replaceChildren();}
  };
}
