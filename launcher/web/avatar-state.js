// Maps app activity to an avatar clip and filters short-lived state changes.
class XiaoliAvatarState {
  constructor(onChange = () => {}, { debounceMs = 250, sorryMs = 6040, now = () => Date.now(), timers = globalThis } = {}) {
    this.onChange=onChange;this.debounceMs=debounceMs;this.sorryMs=sorryMs;this.now=now;this.timers=timers;
    this.current='idle';this.desired='idle';this.pending=null;this.sorryTimer=null;this.lastChange=this.now();
  }
  normalize(state) { return ['idle','listening','thinking','speaking','sorry'].includes(state) ? state : 'idle'; }
  set(state) {
    state=this.normalize(state);
    if(state==='sorry') { this.desired=this.current==='sorry'?this.desired:this.current;this.clearPending();this.current='sorry';this.lastChange=this.now();this.onChange('sorry');this.timers.clearTimeout(this.sorryTimer);this.sorryTimer=this.timers.setTimeout(()=>{this.current='idle';this.lastChange=this.now();this.onChange(this.desired||'idle');this.current=this.desired||'idle';},this.sorryMs);return; }
    this.desired=state;
    if(this.current==='sorry') return;
    this.clearPending();
    const delay=Math.max(0,this.debounceMs-(this.now()-this.lastChange));
    if(delay===0){this.current=state;this.lastChange=this.now();this.onChange(state);}
    else this.pending=this.timers.setTimeout(()=>{this.pending=null;if(this.current!=='sorry'){this.current=state;this.lastChange=this.now();this.onChange(state);}},delay);
  }
  clearPending(){if(this.pending!==null){this.timers.clearTimeout(this.pending);this.pending=null;}}
  dispose(){this.clearPending();this.timers.clearTimeout(this.sorryTimer);}
}
if(typeof module!=='undefined')module.exports=XiaoliAvatarState;else globalThis.XiaoliAvatarState=XiaoliAvatarState;
