// Text-only request lifecycle shared by the real chat UI and unit tests.
// No renderer, speech, persistent storage or remote-provider dependency.
export function tidy(text) {
  return text.replace(/<think>[\s\S]*?<\/think>/g, '').replace(/<think>[\s\S]*$/, '')
    .replace(/<\/?think>/g, '').replace(/\/no_think/g, '').replace(/\*\*/g, '')
    .replace(/^#{1,6}\s*/gm, '').replace(/[ \t]+\n/g, '\n').replace(/^\s+/, '');
}

export class ChatSession {
  constructor({fetchImpl=globalThis.fetch, onState=()=>{}, onStart=()=>{}, onText=()=>{}, onFinish=()=>{}, timeoutMs=90000, expressionMs=600}={}) {
    Object.assign(this,{fetchImpl,onState,onStart,onText,onFinish,timeoutMs,expressionMs});
    this.history=[];this.active=null;this.nextId=0;this.state='idle';
  }
  get busy() { return this.active!==null; }
  setState(state) {
    this.state=state;
    // Animation is optional: a failed renderer callback cannot break chat.
    try { this.onState(state); } catch {}
  }
  cancel(reason='cancelled') {
    const turn=this.active;
    if(!turn) { this.setState('idle');return false; }
    this.active=null;clearTimeout(turn.timer);turn.abort.abort();this.setState('idle');
    this.onFinish({id:turn.id,status:'cancelled',reason,text:turn.shown});return true;
  }
  async send(input) {
    const text=input.trim();
    if(!text||this.busy)return {status:'ignored'};
    const turn={id:++this.nextId,abort:new AbortController(),raw:'',shown:'',firstTextAt:0,timedOut:false};
    this.active=turn;this.setState('thinking');this.onStart({id:turn.id,text});
    turn.timer=setTimeout(()=>{turn.timedOut=true;turn.abort.abort();},this.timeoutMs);
    const current=()=>this.active===turn;
    let reader;
    try {
      const response=await this.fetchImpl('/api/chat',{
        method:'POST',headers:{'Content-Type':'application/json'},signal:turn.abort.signal,
        body:JSON.stringify({messages:[...this.history,{role:'user',content:text}].slice(-20)})
      });
      if(!current())return {status:'cancelled'};
      if(!response.ok||!response.body)throw new Error('HTTP '+response.status);
      reader=response.body.getReader();const decoder=new TextDecoder();let buffer='',finished=false;
      const line=(raw)=>{
        const value=raw.trim();if(!value.startsWith('data:'))return;
        const payload=value.slice(5).trim();if(!payload)return;
        if(payload==='[DONE]'){finished=true;return;}
        const item=JSON.parse(payload);if(item.error)throw new Error('stream error');
        const piece=item.choices?.[0]?.delta?.content;
        if(typeof piece==='string') {
          turn.raw+=piece;const shown=tidy(turn.raw);
          if(shown&&shown!==turn.shown&&current()) {
            turn.shown=shown;
            if(!turn.firstTextAt){turn.firstTextAt=Date.now();this.setState('expressing');}
            this.onText({id:turn.id,text:shown});
          }
        }
      };
      while(!finished) {
        const {value,done}=await reader.read();if(!current())return {status:'cancelled'};
        buffer+=done?decoder.decode():decoder.decode(value,{stream:true});
        let split;
        while(!finished&&(split=buffer.indexOf('\n'))>=0){line(buffer.slice(0,split));buffer=buffer.slice(split+1);}
        if(done){if(buffer.trim()&&!finished)line(buffer);break;}
      }
      if(!current())return {status:'cancelled'};
      if(!finished||!turn.shown.trim())throw new Error('incomplete or empty response');
      // Give fast text replies one visible, bounded gesture. This is not lip sync.
      const remaining=this.expressionMs-(Date.now()-turn.firstTextAt);
      if(remaining>0)await new Promise(resolve=>{
        const finish=()=>{clearTimeout(timer);turn.abort.signal.removeEventListener('abort',finish);resolve();};
        const timer=setTimeout(finish,remaining);turn.abort.signal.addEventListener('abort',finish,{once:true});
      });
      if(!current())return {status:'cancelled'};
      if(turn.abort.signal.aborted)throw new Error('timeout');
      this.history.push({role:'user',content:text},{role:'assistant',content:turn.shown.trim()});
      this.history=this.history.slice(-20);
      this.active=null;this.setState('idle');this.onFinish({id:turn.id,status:'completed',text:turn.shown.trim()});
      return {status:'completed'};
    } catch(error) {
      if(!current())return {status:'cancelled'};
      this.active=null;this.setState('idle');this.onFinish({id:turn.id,status:'error',reason:turn.timedOut?'timeout':'request',text:turn.shown});
      return {status:'error'};
    } finally {
      clearTimeout(turn.timer);
      if(reader){try{await reader.cancel();}catch{}try{reader.releaseLock();}catch{}}
    }
  }
}
