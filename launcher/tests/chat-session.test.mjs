import test from 'node:test';import assert from 'node:assert/strict';
import {ChatSession,tidy} from '../web/chat-session.mjs';
const encoder=new TextEncoder();
const piece=text=>'data: '+JSON.stringify({choices:[{delta:{content:text}}]})+'\n\n';
const done='data: [DONE]\n\n';
const response=text=>new Response(encoder.encode(text),{status:200,headers:{'Content-Type':'text/event-stream'}});
function setup(fetchImpl,options={}){const states=[],events=[];const session=new ChatSession({fetchImpl,expressionMs:0,timeoutMs:1000,onState:s=>states.push(s),onFinish:e=>events.push(e),...options});return {session,states,events};}
test('actual request shape and idle → thinking → expressing → idle',async()=>{
 let request;const {session,states,events}=setup(async(url,opts)=>{request={url,...opts};return response(piece('你好')+done);});
 await session.send('  你好  ');assert.equal(request.url,'/api/chat');assert.equal(request.method,'POST');assert.deepEqual(JSON.parse(request.body).messages,[{role:'user',content:'你好'}]);assert.ok(request.signal instanceof AbortSignal);
 assert.deepEqual(states,['thinking','expressing','idle']);assert.equal(events[0].status,'completed');assert.equal(session.history.length,2);assert.equal(session.busy,false);
});
test('SSE arbitrary byte/UTF-8 splits and DONE without final newline',async()=>{
 const bytes=encoder.encode(piece('中文🙂')+'data: [DONE]');let i=0;
 const {session}=setup(async()=>new Response(new ReadableStream({pull(c){if(i<bytes.length)c.enqueue(bytes.slice(i,i+=1));else c.close();}})));
 await session.send('测试');assert.equal(session.history[1].content,'中文🙂');
});
test('hidden reasoning does not start expression',async()=>{
 const {session,states}=setup(async()=>response(piece('<think>隐藏')+piece('</think>答复')+done));await session.send('问');assert.deepEqual(states,['thinking','expressing','idle']);assert.equal(session.history[1].content,'答复');assert.equal(tidy('<think>未结束'),'');
});
for(const [name,reply] of [['HTTP error',()=>new Response('bad',{status:500})],['empty',()=>response(done)],['truncated',()=>response(piece('半段'))],['malformed',()=>response('data: {bad}\n\n')]]){
 test(name+' returns idle and keeps failed turn out of context',async()=>{
  const {session,events}=setup(async()=>reply());await session.send('问');assert.equal(session.state,'idle');assert.equal(session.busy,false);assert.equal(events[0].status,'error');assert.deepEqual(session.history,[]);
 });
}
test('repeated send is ignored until first turn completes',async()=>{
 let release,calls=0;const {session}=setup(()=>{calls++;return new Promise(r=>release=r);});const first=session.send('第一条');assert.equal((await session.send('重复')).status,'ignored');assert.equal(calls,1);release(response(piece('答')+done));await first;
});
test('cancel aborts immediately; late old response cannot overwrite new turn',async()=>{
 let release,signal,calls=0;const {session,events,states}=setup((url,options)=>{calls++;if(calls===1){signal=options.signal;return new Promise(r=>release=r);}return Promise.resolve(response(piece('新回复')+done));});
 const old=session.send('旧问题');session.cancel();assert.ok(signal.aborted);assert.equal(session.state,'idle');await session.send('新问题');release(response(piece('过时回复')+done));await old;
 assert.equal(session.history[1].content,'新回复');assert.equal(session.history.length,2);assert.deepEqual(events.map(x=>x.status),['cancelled','completed']);assert.equal(states.at(-1),'idle');
});
test('leave/restore cancellation can be followed by a fresh request',async()=>{
 let reject,calls=0;const {session}=setup((u,{signal})=>{if(++calls>1)return Promise.resolve(response(piece('返回后回复')+done));return new Promise((_,r)=>{reject=r;signal.addEventListener('abort',()=>r(new Error('aborted')));});});
 const pending=session.send('离开前');session.cancel('hidden');await pending;assert.equal(session.state,'idle');await session.send('回来后');assert.equal(session.history[1].content,'返回后回复');
});
test('network failure is recoverable',async()=>{let calls=0;const {session}=setup(async()=>{if(++calls===1)throw Error('offline');return response(piece('恢复')+done);});await session.send('一');await session.send('二');assert.equal(session.history[1].content,'恢复');});
test('timeout aborts and releases busy state',async()=>{const {session,events}=setup((u,{signal})=>new Promise((_,reject)=>signal.addEventListener('abort',()=>reject(Error('abort')))),{timeoutMs:10});await session.send('问');assert.equal(events[0].reason,'timeout');assert.equal(session.busy,false);});
test('renderer failure never interrupts text/chat completion',async()=>{const {session}=setup(async()=>response(piece('仍可聊天')+done),{onState(){throw Error('WebGL unavailable');}});assert.equal((await session.send('问')).status,'completed');assert.equal(session.history[1].content,'仍可聊天');});
test('only latest 20 messages retained, and text-only gestures settle',async()=>{const {session}=setup(async()=>response(piece('答')+done));for(let i=0;i<15;i++)await session.send('问'+i);assert.equal(session.history.length,20);assert.equal(session.state,'idle');});
