// AudioContext is explicitly 16 kHz. Send bounded 64 ms frames to local Silero.
class XiaoliCapture extends AudioWorkletProcessor {
  constructor() { super(); this.buffer=new Float32Array(1024); this.used=0; }
  process(inputs) {
    const samples=inputs[0]?.[0];
    if(samples) for(const v of samples) {
      this.buffer[this.used++]=v;
      if(this.used===1024) {this.port.postMessage(this.buffer);this.buffer=new Float32Array(1024);this.used=0;}
    }
    // Outputs stay silent: microphone audio must never play through speakers.
    return true;
  }
}
registerProcessor('xiaoli-capture',XiaoliCapture);
