import {ACTIONS,MotionEngine,actionPose,microPose,neutral} from './motion.mjs';
export function runTests(){
 const checks=[];const test=(name,fn)=>{try{if(!fn())throw new Error('assertion failed');checks.push({name,pass:true});}catch(e){checks.push({name,pass:false,error:e.message});}};
 const values=p=>Object.values(p).flat();
 test('所有动作全时段参数有限且腕部目标可达',()=>Object.entries(ACTIONS).every(([id,a])=>{for(let t=0;t<=a.duration;t+=1/60){const p=actionPose(id,t,0);if(!values(p).every(Number.isFinite))return false;for(const [k,s] of [['left',1],['right',-1]])if(Math.hypot(p[k][0]-s*.26,p[k][1],p[k][2])>.532)return false;}return true;}));
 test('眨眼完整闭合并恢复，间隔非等长',()=>{const close=[];let previous=0;for(let t=0;t<23;t+=.01){const b=microPose(t).blink;if(b>.98&&previous<=.98)close.push(t);previous=b;}return close.length>=4&&Math.abs((close[1]-close[0])-(close[2]-close[1]))>.5&&microPose(0).blink===0;});
 test('挥手与待机有可见三维位移差异',()=>Math.hypot(...actionPose('wave',1.3).right.map((v,i)=>v-neutral().right[i]))>.35);
 test('低优先级自动动作不打断聆听',()=>{const e=new MotionEngine();e.request('listen',{manual:true});e.update(.1);return !e.request('think');});
 test('用户打断立即接受且第一帧姿态连续',()=>{const e=new MotionEngine();e.request('wave',{manual:true});for(let i=0;i<18;i++)e.update(.05);const before=values(e.pose);e.request('listen',{manual:true});e.update(0);return e.id==='listen'&&values(e.pose).every((v,i)=>Math.abs(v-before[i])<1e-10);});
 test('任意两动作快速切换不产生数值错误',()=>{for(const a of Object.keys(ACTIONS))for(const b of Object.keys(ACTIONS)){const e=new MotionEngine();e.request(a,{manual:true});for(let i=0;i<12;i++)e.update(.05);e.request(b,{manual:true});for(let i=0;i<20;i++){e.update(.05);if(!values(e.pose).every(Number.isFinite))return false;}}return true;});
 test('单次动作结束后自动回待机',()=>{const e=new MotionEngine();e.request('nod',{manual:true});for(let i=0;i<60;i++)e.update(.05);return e.id==='idle';});
 test('说话口型含停顿且情绪不控制 mouth 通道',()=>actionPose('speak',2.9).mouth===0&&actionPose('happy',1).mouth===0&&actionPose('confused',1).mouth===0);
 return {createdAt:new Date().toISOString(),total:checks.length,passed:checks.filter(x=>x.pass).length,checks};
}
if(typeof window==='undefined'){const report=runTests();console.log(JSON.stringify(report,null,2));if(report.passed!==report.total)process.exitCode=1;}
