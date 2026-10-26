// Pure recording state machine. Identity + epoch fence every asynchronous result.
// partial revisions replace only this recording's insertion; final commits once.
class XiaoliRecording {
  constructor(hooks={}) { this.hooks=hooks; this.current=null; this.epoch=0; this.pressAt=null; }
  down(now,draft='',cursor=draft.length) {
    if(this.current) { this.stop(); this.pressAt=null; return; }
    const rec={epoch:++this.epoch,draft,cursor,phase:'starting',revision:0,sendPending:false};
    this.current=rec;this.pressAt=now;
    this.hooks.render?.(draft,true,'正在听，松开结束；滑出取消');
    this.hooks.start?.(rec);
  }
  up(now) {
    if(this.pressAt===null) return;
    const held=now-this.pressAt;this.pressAt=null;
    if(held>=300) this.stop();
  }
  move(inside) { if(this.pressAt!==null && !inside) this.cancel(); }
  live(rec) { return this.current===rec && rec.epoch===this.epoch; }
  ready(rec) { if(this.live(rec) && rec.phase==='starting') rec.phase='recording'; }
  stop() {
    const rec=this.current;
    if(!rec || rec.phase==='finishing') return;
    rec.phase='finishing';this.pressAt=null;
    this.hooks.render?.(null,true,'正在整理文字');
    this.hooks.finish?.(rec);
  }
  requestSend() {
    if(!this.current) return false;
    this.current.sendPending=true;this.stop();return true;
  }
  result(rec,result) {
    if(!this.live(rec) || result.session!==rec.session || !Number.isInteger(result.revision) || result.revision<=rec.revision) return false;
    if(result.kind!=='partial' && result.kind!=='final') return false;
    rec.revision=result.revision;
    const text=(result.text||'').replace(/<\|[^|]*\|>/g,'').trim();
    const value=rec.draft.slice(0,rec.cursor)+text+rec.draft.slice(rec.cursor);
    if(result.kind==='partial') {this.hooks.render?.(value,true,rec.phase==='finishing' ? '正在整理文字' : '正在听');return true;}
    this.current=null;this.pressAt=null;
    this.hooks.render?.(value,false,'');this.hooks.committed?.(rec,text);
    if(rec.sendPending) this.hooks.send?.(rec);
    return true;
  }
  cancel() {
    const rec=this.current;if(!rec) return;
    ++this.epoch;this.current=null;this.pressAt=null;
    this.hooks.render?.(rec.draft,false,'');this.hooks.cancel?.(rec);
  }
}
if(typeof module!=='undefined') module.exports=XiaoliRecording;
else globalThis.XiaoliRecording=XiaoliRecording;
