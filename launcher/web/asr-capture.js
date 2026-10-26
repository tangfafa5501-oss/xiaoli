// AudioContext is 16 kHz. flush ACK follows all queued audio on this port.
class XiaoliCapture extends AudioWorkletProcessor {
  constructor() {
    super(); this.buffer=new Float32Array(1024); this.used=0; this.stopped=false;
    this.port.onmessage=({data})=>{
      if(data.kind!=='flush') return;
      this.stopped=true;
      if(this.used) {
        const tail=new Float32Array(Math.ceil(this.used/512)*512);
        tail.set(this.buffer.subarray(0,this.used));
        this.port.postMessage({kind:'chunk',samples:tail,validSamples:this.used});
        this.used=0;
      }
      this.port.postMessage({kind:'flushed',id:data.id});
    };
  }
  process(inputs) {
    const samples=inputs[0]?.[0];
    if(samples && !this.stopped) for(const v of samples) {
      this.buffer[this.used++]=v;
      if(this.used===1024) {
        this.port.postMessage({kind:'chunk',samples:this.buffer,validSamples:1024});
        this.buffer=new Float32Array(1024);this.used=0;
      }
    }
    return true; // output stays silent
  }
}
registerProcessor('xiaoli-capture',XiaoliCapture);
