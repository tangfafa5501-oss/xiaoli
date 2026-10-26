const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const html=fs.readFileSync(require.resolve('../web/index.html'),'utf8');
test('chunk 400 clears pending token and reports numeric stats; empty chunks stay local',async()=>{
 const source=html.slice(html.indexOf('function queueChunk('),html.indexOf('function startMic('));const reported=[];const rec={session:'released',queued:0,chain:Promise.resolve()};let calls=0;
 const c=vm.createContext({rec,recorder:{live:()=>true,result:()=>{}},reportMicFailure:(stage,e)=>reported.push({stage,e}),asrRequest:async()=>{calls++;throw Object.assign(new Error('samples'),{status:400})},chatMode:{activity:()=>{}}});vm.runInContext("let pendingMicSession='released';"+source,c);
 c.data={samples:new Float32Array(),validSamples:0};vm.runInContext('queueChunk(rec,data)',c);assert.equal(calls,0);assert.equal(rec.queued,0);
 c.data={samples:Float32Array.from([1,-.5,0]),validSamples:2};vm.runInContext('queueChunk(rec,data)',c);await rec.chain;
 assert.equal(calls,1);assert.equal(vm.runInContext('pendingMicSession',c),'');assert.equal(rec.queued,0);assert.equal(reported.length,1);assert.equal(reported[0].stage,'chunk');assert.deepEqual(JSON.parse(JSON.stringify(reported[0].e.chunkStats)),{n:3,validSamples:2,maxAbs:1});
});
