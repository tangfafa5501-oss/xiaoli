const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
for(const count of [0,128,512,1023,1024,1152,2047]) test(`flush retains ${count} samples before ACK`,()=>{
 let Capture;const messages=[];
 const context={Float32Array,AudioWorkletProcessor:class{constructor(){this.port={postMessage:m=>messages.push(m)};}},registerProcessor:(_,c)=>Capture=c};
 vm.runInNewContext(fs.readFileSync(require.resolve('../web/asr-capture.js'),'utf8'),context);
 const capture=new Capture();const samples=Float32Array.from({length:count},(_,i)=>i/4096);
 for(let i=0;i<count;i+=128) capture.process([[samples.subarray(i,i+128)]]);
 capture.port.onmessage({data:{kind:'flush',id:7}});
 assert.equal(messages.at(-1).kind,'flushed');assert.equal(messages.at(-1).id,7);
 const chunks=messages.filter(m=>m.kind==='chunk');
 assert.deepEqual(chunks.flatMap(m=>Array.from(m.samples.subarray(0,m.validSamples))),Array.from(samples));
 for(const m of chunks){assert.equal(m.samples.length%512,0);assert(m.samples.subarray(m.validSamples).every(x=>x===0));}
 const before=messages.length;capture.process([[new Float32Array(128)]]);assert.equal(messages.length,before);
});
