const test=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs');
for(const rate of [48000,44100]) test('resample '+rate,()=>{
 let C;const messages=[];
 vm.runInNewContext(fs.readFileSync(require.resolve('../web/asr-capture.js'),'utf8'),{Float32Array,AudioWorkletProcessor:class{constructor(){this.port={postMessage:m=>messages.push(m)}}},registerProcessor:(_,c)=>C=c});
 function run(hz){messages.length=0;const c=new C({processorOptions:{inputRate:rate}}),n=rate+137;
 for(let i=0;i<n;i+=128)c.process([[Float32Array.from({length:Math.min(128,n-i)},(_,j)=>Math.sin(2*Math.PI*hz*(i+j)/rate))]]);
 c.port.onmessage({data:{kind:'flush',id:4}});
 assert.equal(messages.at(-1).kind,'flushed');
 const chunks=messages.filter(x=>x.kind==='chunk');assert(chunks.slice(0,-1).every(x=>x.validSamples===1024));
 const y=chunks.flatMap(x=>Array.from(x.samples.subarray(0,x.validSamples)));
 assert.equal(y.length,Math.floor(n*16000/rate));assert(chunks.at(-1).samples.subarray(chunks.at(-1).validSamples).every(v=>v===0));return y.slice(100,-100);}
 const y=run(1000);const rms=Math.sqrt(y.reduce((s,v)=>s+v*v,0)/y.length);assert(Math.abs(rms-Math.SQRT1_2)<.01);
 const crossings=y.filter((v,i)=>i&&v>=0&&y[i-1]<0).length;assert(Math.abs(crossings*16000/y.length-1000)<3);
 const high=run(12000);assert(Math.sqrt(high.reduce((s,v)=>s+v*v,0)/high.length)<.02);
});
