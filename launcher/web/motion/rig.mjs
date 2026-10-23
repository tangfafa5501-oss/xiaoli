import * as THREE from 'three';
const DOWN=new THREE.Vector3(0,-1,0);
export function createCharacter(){
 const root=new THREE.Group();root.name='OriginalCompanion';
 const bones={}, animated=[], facial={};
 const mat=(color,roughness=.7)=>new THREE.MeshStandardMaterial({color,roughness});
 const skin=mat('#efd3bd'),hair=mat('#382c36'),cloth=mat('#e9e4df'),dress=mat('#796b8d'),dark=mat('#302b38'),white=mat('#fff5e9'),iris=mat('#6c746a'),rose=mat('#b96d78');
 const sphere=new THREE.SphereGeometry(1,28,20);
 function ell(parent,name,material,xyz,scale){const m=new THREE.Mesh(sphere,material);m.name=name;m.position.set(...xyz);m.scale.set(...scale);m.castShadow=true;m.receiveShadow=true;parent.add(m);return m;}
 function bone(name,parent,xyz){const b=new THREE.Bone();b.name=name;b.position.set(...xyz);parent.add(b);bones[name]=b;animated.push(b);return b;}
 function limb(parent,name,len,r,material){const m=new THREE.Mesh(new THREE.CapsuleGeometry(r,len-2*r,6,12),material);m.position.y=-len/2;m.name=name;m.castShadow=true;parent.add(m);return m;}
 const hips=bone('hips',root,[0,.91,0]);ell(hips,'pelvis',dress,[0,0,0],[.21,.17,.13]);
 const spine=bone('spine',hips,[0,.12,0]),chest=bone('chest',spine,[0,.21,0]);
 ell(spine,'waist',cloth,[0,.075,0],[.185,.22,.12]);
 let body;
 // A simple original pinafore silhouette; no third-party character meshes.
 ell(chest,'bodice',dress,[0,-.02,.107],[.168,.18,.052]);
 for(const s of [-1,1])ell(chest,'strap'+s,dress,[s*.115,.105,.11],[.026,.12,.029]);
 const skirt=new THREE.Mesh(new THREE.CylinderGeometry(.19,.285,.37,40),dress);skirt.position.y=-.16;skirt.castShadow=true;hips.add(skirt);
 for(const [side,s] of [['left',1],['right',-1]]){
   const leg=bone(side+'UpperLeg',hips,[s*.115,-.08,0]);limb(leg,side+'Thigh',.37,.066,skin);
   const shin=bone(side+'LowerLeg',leg,[0,-.37,0]);limb(shin,side+'Shin',.36,.047,cloth);
   const foot=bone(side+'Foot',shin,[0,-.36,0]);ell(foot,side+'Shoe',dark,[0,-.045,.065],[.073,.052,.135]);
 }
 const neck=bone('neck',chest,[0,.23,0]);ell(neck,'neckMesh',skin,[0,.035,0],[.065,.1,.062]);
 const head=bone('head',neck,[0,.15,0]);ell(head,'face',skin,[0,.015,0],[.175,.225,.156]);
 // Back hair and a cap keep the face clear from the frontal camera.
 ell(head,'backHair',hair,[0,.01,-.065],[.19,.24,.145]);
 const cap=new THREE.Mesh(new THREE.SphereGeometry(1,32,20,0,Math.PI*2,0,Math.PI*.47),hair);cap.scale.set(.185,.237,.172);cap.position.y=.015;cap.castShadow=true;head.add(cap);
 for(const s of [-1,1]){
   ell(head,'sideHair'+s,hair,[s*.158,-.012,.002],[.047,.2,.105]);
   ell(head,'ear'+s,skin,[s*.17,-.002,0],[.028,.055,.027]);
   const eye=new THREE.Group();eye.name='eye'+s;eye.position.set(s*.068,.035,.139);head.add(eye);animated.push(eye);
   ell(eye,'eyeWhite'+s,white,[0,0,0],[.044,.031,.017]);
   const pupil=new THREE.Group();pupil.name='pupil'+s;eye.add(pupil);animated.push(pupil);
   ell(pupil,'iris'+s,iris,[0,0,.013],[.023,.027,.009]);ell(pupil,'pupilDot'+s,dark,[0,0,.021],[.012,.02,.004]);ell(pupil,'glint'+s,white,[-.007,.01,.025],[.006,.007,.003]);
   const brow=ell(head,'brow'+s,hair,[s*.068,.092,.143],[.046,.007,.012]);animated.push(brow);
   const cheek=ell(head,'cheek'+s,new THREE.MeshStandardMaterial({color:'#d69c94',transparent:true,opacity:.3,roughness:1}),[s*.102,-.026,.13],[.032,.013,.008]);
   facial[s]={eye,pupil,brow,cheek};
 }
 ell(head,'nose',skin,[0,-.012,.159],[.019,.025,.023]);
 const mouth=ell(head,'mouth',rose,[0,-.075,.143],[.037,.008,.008]);animated.push(mouth);facial.mouth=mouth;
 const arms={};
 for(const [side,s] of [['left',1],['right',-1]]){
   const shoulder=bone(side+'UpperArm',chest,[s*.26,.12,0]);limb(shoulder,side+'Sleeve',.285,.065,cloth);
   const elbow=bone(side+'LowerArm',shoulder,[0,-.285,0]);ell(elbow,side+'Elbow',cloth,[0,0,0],[.06,.06,.06]);limb(elbow,side+'Forearm',.255,.045,skin);
   const hand=bone(side+'Hand',elbow,[0,-.255,0]);ell(hand,side+'Palm',skin,[0,-.048,0],[.043,.062,.023]);
   const fingers=[];
   for(let f=0;f<4;f++){
     const len=[.073,.087,.081,.061][f],base=bone(side+'Finger'+f,hand,[(f-1.5)*.021,-.089,0]);
     limb(base,side+'FingerMesh'+f,len*.56,.009,skin);
     const tip=bone(side+'FingerTip'+f,base,[0,-len*.56,0]);limb(tip,side+'FingerTipMesh'+f,len*.44,.008,skin);fingers.push([base,tip]);
   }
   const thumb=bone(side+'Thumb',hand,[s*.041,-.025,0]);limb(thumb,side+'ThumbMesh',.06,.013,skin);thumb.rotation.z=s*.65;fingers.push([thumb]);
   arms[side]={shoulder,elbow,hand,fingers,sign:s};
 }
 root.updateMatrixWorld(true);
 const boneList=Object.values(bones),skeleton=new THREE.Skeleton(boneList);
 // The torso is genuinely skinned. Limbs remain rigid bone attachments for clear rig inspection.
 const torsoGeometry=sphere.clone();torsoGeometry.scale(.265,.225,.145);torsoGeometry.translate(0,1.275,0);
 const positions=torsoGeometry.getAttribute('position'),indices=[],weights=[];
 for(let i=0;i<positions.count;i++){const w=THREE.MathUtils.clamp((positions.getY(i)-1.08)/.27,0,1);indices.push(boneList.indexOf(spine),boneList.indexOf(chest),0,0);weights.push(1-w,w,0,0);}
 torsoGeometry.setAttribute('skinIndex',new THREE.Uint16BufferAttribute(indices,4));torsoGeometry.setAttribute('skinWeight',new THREE.Float32BufferAttribute(weights,4));
 body=new THREE.SkinnedMesh(torsoGeometry,cloth);body.name='sweater';body.castShadow=true;body.receiveShadow=true;root.add(body);body.bind(skeleton);
 function armIK(arm,target,palm,curl){
   const {shoulder,elbow,hand,sign}=arm,start=shoulder.position.clone();
   const goal=new THREE.Vector3(...target).add(new THREE.Vector3(0,.12,0)),delta=goal.clone().sub(start);
   const len=THREE.MathUtils.clamp(delta.length(),.06,.285+.255-.008),dir=delta.normalize();
   const pole=new THREE.Vector3(sign, -.4,-.3);pole.addScaledVector(dir,-pole.dot(dir)).normalize();
   const a=(.285**2-.255**2+len**2)/(2*len),h=Math.sqrt(Math.max(0,.285**2-a*a));
   const e=start.clone().addScaledVector(dir,a).addScaledVector(pole,h),g=start.clone().addScaledVector(dir,len);
   shoulder.quaternion.setFromUnitVectors(DOWN,e.clone().sub(start).normalize());
   const worldFore=new THREE.Quaternion().setFromUnitVectors(DOWN,g.sub(e).normalize());
   elbow.quaternion.copy(shoulder.quaternion).invert().multiply(worldFore);
   const desired=new THREE.Quaternion().setFromEuler(new THREE.Euler(...palm));
   hand.quaternion.copy(worldFore).invert().multiply(desired);
   arm.fingers.forEach((f,i)=>f.forEach((b,j)=>{b.rotation.x=curl*(j?.85:.7);if(i<4)b.rotation.z=(i-1.5)*.035*(1-curl);else b.rotation.z=sign*.65;}));
 }
 function apply(p,m={breath:0,blink:0,yaw:0,roll:0,gaze:[0,0]}){
   chest.rotation.set(p.chest[0]+m.breath*.006,p.chest[1],p.chest[2]+m.roll*.2);
   chest.scale.set(1+m.breath*.009,1+m.breath*.003,1+m.breath*.018);
   head.rotation.set(p.head[0],p.head[1]+m.yaw,p.head[2]+m.roll);
   for(const side of ['left','right'])armIK(arms[side],p[side],p[side+'Palm'],p.curl);
   for(const s of [-1,1]){const f=facial[s];f.eye.scale.y=Math.max(.035,1-Math.max(p.blink,m.blink)*.965-p.smile*.12);
     f.pupil.position.set(THREE.MathUtils.clamp(p.gaze[0]+m.gaze[0],-.2,.2)*.08,THREE.MathUtils.clamp(p.gaze[1]+m.gaze[1],-.2,.2)*.07,0);
     f.brow.position.y=.092+p.brow*.012+(s===1?p.asym*.012:0);f.brow.rotation.z=s*(-p.smile*.08+p.asym*.09);f.cheek.material.opacity=.2+p.smile*.22;}
   mouth.scale.set(.037+p.smile*.011,.008+p.mouth*.026,.008);mouth.rotation.z=p.asym*.07;
   root.updateMatrixWorld(true);skeleton.update();
 }
 return {root,bones,arms,skeleton,animated,body,apply};
}
