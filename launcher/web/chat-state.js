// Chat alone owns endpointing; all delayed work is fenced by generation.
class XiaoliChat {
  constructor(h={}) { this.h=h;this.enabled=false;this.epoch=0;this.state='off'; }
  set(state) {this.state=state;this.h.state?.(state);}
  live(e) {return this.enabled && e===this.epoch;}
  open() {this.enabled=true;this.h.cancel?.();this.resume();}
  close() {this.enabled=false;++this.epoch;this.h.cancel?.();this.set('off');}
  resume() {const e=++this.epoch;this.set('guard');this.h.delay(()=>{if(this.live(e)){this.voice=0;this.silence=0;this.set('listening');this.h.listen?.(e);}},300);}
  activity(e,active,ms) {if(!this.live(e)||this.state!=='listening')return;if(active){this.voice+=ms;this.silence=0;}else if(this.voice)this.silence+=ms;if(this.voice && this.silence>=1500){this.set('thinking');this.h.finish?.();}}
  final(e,text) {if(!this.live(e) || !['listening','thinking'].includes(this.state))return false;if(this.voice<300 || [...text.replace(/[^\p{L}\p{N}]/gu,'')].length<2){this.resume();return false;}this.set('thinking');return true;}
  speaking(e) {if(this.live(e))this.set('speaking');}
  done(e) {if(this.live(e))this.resume();}
  interrupt() {if(!this.enabled)return;++this.epoch;this.h.cancel?.();this.resume();}
}
if(typeof module!=='undefined')module.exports=XiaoliChat;else globalThis.XiaoliChat=XiaoliChat;
