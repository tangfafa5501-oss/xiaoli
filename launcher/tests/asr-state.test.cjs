const assert=require('node:assert/strict');
const test=require('node:test');
const Recording=require('../web/asr-state.js');
function setup(){
 const events=[];
 const r=new Recording(Object.fromEntries(['render','start','finish','cancel','send','committed'].map(k=>[k,(...args)=>events.push([k,...args])])));
 return {r,events};
}
test('tap stays recording; second tap stops once',()=>{
 const {r,events}=setup();r.down(0);r.up(299);assert.equal(r.current.phase,'starting');r.down(400);r.up(410);
 assert.equal(r.current.phase,'finishing');assert.equal(events.filter(e=>e[0]==='finish').length,1);
});
test('300ms hold stops on release',()=>{const {r}=setup();r.down(10);r.up(310);assert.equal(r.current.phase,'finishing');});
for(const reason of ['slide','Esc','pointercancel','blur']) test(reason+' restores draft and rejects late result',()=>{
 const {r,events}=setup();r.down(0,'草稿尾',2);const old=r.current;old.session='old';
 if(reason==='slide') r.move(false);else r.cancel();
 assert.equal(r.current,null);assert.deepEqual(events.at(-2),['render','草稿尾',false,'']);
 assert.equal(r.result(old,{kind:'final',text:'迟到',session:'old',revision:10}),false);
 r.down(900,'新草稿');assert.equal(r.live(old),false);
});
test('partial reserved interface, revision fence, final insertion and send exactly once',()=>{
 const {r,events}=setup();r.down(0,'前后',1);const rec=r.current;rec.session='s';
 assert(r.result(rec,{kind:'partial',session:'s',revision:2,text:'临时'}));
 assert(!r.result(rec,{kind:'partial',session:'s',revision:1,text:'旧'}));
 r.stop();r.requestSend();r.requestSend();assert(r.result(rec,{kind:'final',session:'s',revision:3,text:'最终'}));
 assert.deepEqual(events.filter(e=>e[0]==='render').at(-1),['render','前最终后',false,'']);
 assert.equal(events.filter(e=>e[0]==='send').length,1);assert(!r.result(rec,{kind:'final',session:'s',revision:4,text:'重复'}));
});
test('normal final never sends; cancel clears queued send',()=>{
 for(const cancel of [true,false]) {const {r,events}=setup();r.down(0,'草稿');const rec=r.current;rec.session='s';r.stop();if(cancel){r.requestSend();r.cancel();}
 r.result(rec,{kind:'final',session:'s',revision:2,text:'文字'});assert.equal(events.filter(e=>e[0]==='send').length,0);}
});
