// Capture at the device rate; emit anti-aliased 16 kHz audio. flush ACK follows all queued audio on this port.
class XiaoliCapture extends AudioWorkletProcessor {
  constructor(options={}) {
    super(); this.buffer=new Float32Array(1024); this.used=0; this.stopped=false;
    this.rate=options.processorOptions?.inputRate||16000;this.input=[];this.base=0;this.total=0;this.output=0;
    this.port.onmessage=({data})=>{
      if(data.kind!=='flush') return;
      this.stopped=true;
      if(this.rate!==16000) this.convert(true);
      if(this.used) {
        const tail=new Float32Array(Math.ceil(this.used/512)*512);
        tail.set(this.buffer.subarray(0,this.used));
        this.port.postMessage({kind:'chunk',samples:tail,validSamples:this.used});
        this.used=0;
      }
      this.port.postMessage({kind:'flushed',id:data.id});
    };
  }
  emit(v) {
    this.buffer[this.used++]=v;
    if(this.used===1024) {
      this.port.postMessage({kind:'chunk',samples:this.buffer,validSamples:1024});
      this.buffer=new Float32Array(1024);this.used=0;
    }
  }
  convert(flush) {
    // 64-tap Blackman-windowed sinc, cutoff below output Nyquist.
    const radius=32,step=this.rate/16000,cutoff=.45/step;
    const count=Math.floor(this.total/step);
    while(this.output<count) {
      const at=this.output*step,center=Math.floor(at);
      if(!flush && center+radius>=this.total) break;
      let sum=0,weight=0;
      for(let i=center-radius+1;i<=center+radius;i++) {
        const x=i-at,u=x/radius;
        if(Math.abs(u)>=1) continue;
        const sinc=x===0 ? 2*cutoff : Math.sin(2*Math.PI*cutoff*x)/(Math.PI*x);
        const w=sinc*(.42+.5*Math.cos(Math.PI*u)+.08*Math.cos(2*Math.PI*u));
        weight+=w;
        if(i>=0 && i<this.total) sum+=(this.input[i-this.base]||0)*w;
      }
      this.emit(sum/weight);this.output++;
    }
    const drop=Math.max(0,Math.floor(this.output*step)-radius-this.base);
    this.input.splice(0,drop);this.base+=drop;
  }
  process(inputs) {
    const samples=inputs[0]?.[0];
    if(samples && !this.stopped) {
      if(this.rate===16000) for(const v of samples) this.emit(v);
      else {this.input.push(...samples);this.total+=samples.length;this.convert(false);}
    }
    return true; // output stays silent
  }
}
registerProcessor('xiaoli-capture',XiaoliCapture);
