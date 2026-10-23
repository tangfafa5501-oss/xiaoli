// Original motion design. Units: metres, radians, seconds. +Y up, +Z forward.
export const ACTIONS = {
  idle: { label: '安静陪伴', duration: 9, blend: .65, priority: 0, note: '呼吸与视线错开节奏，手臂自然垂落。' },
  listen: { label: '侧耳聆听', duration: 6, blend: .48, priority: 80, note: '先转眼，再转头；身体微倾，留出回应空间。' },
  think: { label: '认真思考', duration: 5.4, blend: .6, priority: 30, note: '短暂移开视线，手靠近下颌，再重新看向你。' },
  speak: { label: '轻声表达', duration: 6.8, blend: .45, priority: 50, note: '掌心打开，两次有间隔的手势；口型仅为无声示意。' },
  wave: { label: '见面问候', duration: 3.4, blend: .42, priority: 40, note: '先对视再抬手，轻挥两次，手指舒展。' },
  happy: { label: '会心一笑', duration: 3.5, blend: .48, priority: 35, note: '嘴角先上扬，轻收双手，不做夸张弹跳。' },
  confused: { label: '有点疑惑', duration: 3.8, blend: .5, priority: 35, note: '轻歪头、单侧抬眉、掌心询问，随后放松。' },
  nod: { label: '温柔确认', duration: 2.3, blend: .32, priority: 55, note: '一次有收尾的小点头，不连续机械点头。' },
  bye: { label: '下次再见', duration: 4.1, blend: .5, priority: 40, note: '略慢的挥手，轻点头，再将手放回身侧。' },
};
export const DEMO = [ ['wave',3.9],['listen',5],['think',5.8],['speak',7.1],['nod',2.8],['happy',4],['confused',4.3],['bye',4.6],['idle',5] ];
export const clamp=(v,a,b)=>Math.max(a,Math.min(b,v));
export const smooth=t=>{t=clamp(t,0,1);return t*t*t*(t*(t*6-15)+10);};
const pulse=(t,a,b,c,d)=>smooth((t-a)/(b-a))*(1-smooth((t-c)/(d-c)));
export function neutral(){return {head:[0,0,0],chest:[0,0,0],left:[.34,-.50,.05],right:[-.34,-.50,.05],leftPalm:[.08,0,0],rightPalm:[.08,0,0],gaze:[0,0],smile:.15,brow:0,asym:0,curl:.35,mouth:0,blink:0};}
export function blendPose(a,b,w){const out={};for(const k of Object.keys(b))out[k]=Array.isArray(b[k])?b[k].map((v,i)=>a[k][i]+(v-a[k][i])*w):a[k]+(b[k]-a[k])*w;return out;}
export function actionPose(id,t,variant=0){
  const p=neutral(), v=variant%3, s=v===1?-1:1;
  switch(id){
    case 'listen':{
      const focus=smooth(t/.65), nod=pulse(t,2,2.35,2.42,2.9);
      p.head=[.025+nod*.075, .16*focus*s, -.065*focus*s];p.chest=[.028, .035*s,0];p.gaze=[.08*s,.015];p.smile=.22;break;}
    case 'think':{
      const hold=pulse(t,0,1,3.5,5.1);p.right=[-.12,.045,.14];p.rightPalm=[-2.7,.15,-.1];
      p.head=[-.045,.13*s,-.07];p.gaze=[.17*s,.13*hold];p.brow=.35;p.smile=.08;p.curl=.65;
      return blendPose(neutral(),p,hold);}
    case 'speak':{
      const a=pulse(t,.25,1.15,1.7,2.7),b=pulse(t,3.25,4.2,4.8,6.1);
      p.right=[-.34-.07*a,-.48+.22*a,.07+.24*a];p.rightPalm=[-.9*a,0,-.35*a];
      p.left=[.34+.045*b,-.5+.17*b,.07+.23*b];p.leftPalm=[-.8*b,0,.25*b];
      p.head=[.025*Math.sin(t*2.4),.06*Math.sin(t*.8),.018*Math.sin(t*1.3)];
      p.chest=[0,.025*(a-b),.01*(a-b)];p.curl=.13;p.smile=.3;
      const syllable=Math.pow(Math.max(0,Math.sin(t*11.3)),1.4)*(.45+.55*Math.pow(Math.sin(t*2.7+.3),2));
      p.mouth=syllable*(pulse(t,.2,.4,2.3,2.6)+pulse(t,3.15,3.4,6.2,6.6));break;}
    case 'wave': case 'bye':{
      const end=id==='bye'?4.1:3.4, hold=pulse(t,.15,.95,end-1,end-.15), sway=Math.sin((t-.85)*7)*.16*pulse(t,.85,1.05,end-1.35,end-1);
      p.right=[-.49,.13,.17];p.rightPalm=[-Math.PI,0,sway];p.head=[id==='bye'?.07:0,-.025,.04];p.curl=.015;p.smile=.6;p.gaze=[0,0];
      return blendPose(neutral(),p,hold);}
    case 'happy':{
      const a=pulse(t,0,.7,2.3,3.5);p.head=[.04,0,-.055];p.left=[.25,-.25,.25];p.right=[-.24,-.26,.28];
      p.leftPalm=[-1.4,0,.25];p.rightPalm=[-1.4,0,-.25];p.smile=.95;p.curl=.3;p.brow=.2;p.chest=[.025,0,0];return blendPose(neutral(),p,a);}
    case 'confused':{
      const a=pulse(t,0,.7,2.6,3.8);p.head=[-.035,-.06,.13];p.left=[.4,-.28,.29];p.leftPalm=[-1.65,0,.35];p.gaze=[.02,.06];p.brow=.4;p.asym=.7;p.smile=0;p.curl=.03;return blendPose(neutral(),p,a);}
    case 'nod':{p.head=[.15*pulse(t,.1,.65,.8,1.4)-.025*pulse(t,1.35,1.55,1.7,2.1),0,0];p.smile=.45;break;}
  }return p;
}
export function microPose(time){
  // Deterministic, unequal blink intervals; close faster than opening.
  const intervals=[3.7,5.1,4.3,6.4,3.2],cycle=intervals.reduce((a,b)=>a+b,0);
  let t=time%cycle,blink=0;
  for(const span of intervals){if(t<span){blink=pulse(t,span-.24,span-.17,span-.13,span);break;}t-=span;}
  return {breath:Math.sin(time*1.32)*.6+Math.sin(time*.69+.4)*.4,blink,
    yaw:Math.sin(time*.61)*.012+Math.sin(time*.27+.9)*.008,
    roll:Math.sin(time*.47)*.009,
    gaze:[Math.sin(time*.43)*.028+Math.sin(time*1.11)*.008,Math.sin(time*.67)*.018]};
}
export class MotionEngine {
  constructor(){this.id='idle';this.time=0;this.age=0;this.pose=neutral();this.from=neutral();this.blend=0;this.count=0;this.variant=0;this.intensity=1;this.transitions=0;}
  request(id,{manual=false}={}){
    if(!ACTIONS[id])return false;
    if(!manual&&ACTIONS[id].priority<ACTIONS[this.id].priority&&this.age<ACTIONS[this.id].duration)return false;
    if(!manual&&id===this.id)return false;
    this.from=structuredClone(this.pose);this.id=id;this.age=0;this.blend=0;this.variant=(this.count++)%3;this.transitions++;return true;
  }
  update(dt){
    dt=clamp(dt,0,.1);this.time+=dt;this.age+=dt;this.blend+=dt;
    if(this.id!=='idle'&&this.age>ACTIONS[this.id].duration)this.request('idle',{manual:true});
    const target=blendPose(neutral(),actionPose(this.id,this.age,this.variant),this.intensity);
    this.pose=blendPose(this.from,target,smooth(this.blend/ACTIONS[this.id].blend));
    return this.pose;
  }
}
