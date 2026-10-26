// v1: six LE u32 words: XLR5, version, command, session, count, validSamples.
// Commands start=1 chunk=2 finish=3 cancel=4. Response: LE length + JSON.
#include "vendor/c-api.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#ifdef _WIN32
#include <io.h>
#include <fcntl.h>
#endif
static void reply(const char *s) {
  unsigned n=(unsigned)strlen(s);
  unsigned char h[4]={(unsigned char)n,(unsigned char)(n>>8),(unsigned char)(n>>16),(unsigned char)(n>>24)};
  fwrite(h,1,4,stdout);fwrite(s,1,n,stdout);fflush(stdout);
}
int main(void) {
#ifdef _WIN32
  _setmode(_fileno(stdin),_O_BINARY);_setmode(_fileno(stdout),_O_BINARY);
#endif
  SherpaOnnxOfflineRecognizerConfig c={0};
  c.feat_config.sample_rate=16000;c.feat_config.feature_dim=80;
  c.model_config.sense_voice.model="model.int8.onnx";
  c.model_config.sense_voice.language="auto";c.model_config.sense_voice.use_itn=1;
  c.model_config.tokens="tokens.txt";c.model_config.num_threads=4;c.model_config.provider="cpu";
  c.decoding_method="greedy_search";
  const SherpaOnnxOfflineRecognizer *asr=SherpaOnnxCreateOfflineRecognizer(&c);
  if(!asr) return 2;
  SherpaOnnxVadModelConfig v={0};
  v.silero_vad.model="silero_vad.onnx";v.silero_vad.threshold=0.5f;
  v.silero_vad.min_silence_duration=0.32f;v.silero_vad.min_speech_duration=0.16f;
  v.silero_vad.window_size=512;v.silero_vad.max_speech_duration=25;
  v.sample_rate=16000;v.num_threads=1;v.provider="cpu";
  const SherpaOnnxVoiceActivityDetector *vad=SherpaOnnxCreateVoiceActivityDetector(&v,32);
  if(!vad) return 3;
  // Warm recognizer kernels; no network, no microphone needed.
  float silence[16000]={0};
  const SherpaOnnxOfflineStream *warm=SherpaOnnxCreateOfflineStream(asr);
  SherpaOnnxAcceptWaveformOffline(warm,16000,silence,16000);
  SherpaOnnxDecodeOfflineStream(asr,warm);SherpaOnnxDestroyOfflineStream(warm);
  fputs("READY\n",stdout);fflush(stdout);
  float *audio=(float *)malloc(32*16000*sizeof(float));
  if(!audio) return 5;
  unsigned total=0,session=0,revision=0;
  for(;;) {
    unsigned char h[24];if(fread(h,1,24,stdin)!=24) break;
    unsigned w[6];for(unsigned i=0;i<6;i++) w[i]=(unsigned)h[4*i]|((unsigned)h[4*i+1]<<8)|((unsigned)h[4*i+2]<<16)|((unsigned)h[4*i+3]<<24);
    unsigned cmd=w[2],n=w[4],valid=w[5];
    if(w[0]!=0x35524c58 || w[1]!=1 || !w[3] || cmd<1 || cmd>4 ||
       (cmd==2 ? (!n || n>4096 || n%512 || !valid || valid>n) : (n || valid))) break;
    if(cmd!=1 && w[3]!=session) break;
    if(cmd==1) {session=w[3];revision=0;total=0;SherpaOnnxVoiceActivityDetectorReset(vad);}
    if(cmd==2) {
      float pcm[4096];if(fread(pcm,sizeof(float),n,stdin)!=n) break;
      int ok=1;for(unsigned i=0;i<n;i++) if(!isfinite(pcm[i]) || fabsf(pcm[i])>1) ok=0;
      if(!ok || total+valid>32*16000) break;
      memcpy(audio+total,pcm,valid*sizeof(float));total+=valid;
    }
    if(cmd==4) {total=0;SherpaOnnxVoiceActivityDetectorReset(vad);}
    ++revision;
    if(cmd!=3) {
      char out[160];snprintf(out,sizeof(out),"{\"kind\":\"ack\",\"session\":%u,\"revision\":%u}",session,revision);reply(out);
      if(cmd==4) session=0;
      continue;
    }
    // Decode only at finish: chunk ACK never waits for SenseVoice. VAD splits
    // internally; every segment (including the flushed tail) is accumulated.
    char text[12000]={0};size_t used=0;
    for(unsigned offset=0;offset<total+512;offset+=512) {
      float block[512]={0};unsigned count=offset<total ? total-offset : 0;if(count>512) count=512;
      if(count) memcpy(block,audio+offset,count*sizeof(float));
      SherpaOnnxVoiceActivityDetectorAcceptWaveform(vad,block,512);
      if(offset+512>=total+512) SherpaOnnxVoiceActivityDetectorFlush(vad);
      while(!SherpaOnnxVoiceActivityDetectorEmpty(vad)) {
        const SherpaOnnxSpeechSegment *seg=SherpaOnnxVoiceActivityDetectorFront(vad);
        const SherpaOnnxOfflineStream *stream=SherpaOnnxCreateOfflineStream(asr);
        SherpaOnnxAcceptWaveformOffline(stream,16000,seg->samples,seg->n);
        SherpaOnnxDecodeOfflineStream(asr,stream);
        const SherpaOnnxOfflineRecognizerResult *r=SherpaOnnxGetOfflineStreamResult(stream);
        if(!r || !r->text) return 4;
        size_t len=strlen(r->text);if(used+len+2>=sizeof(text)) return 6;
        if(used && len) text[used++]=' ';
        memcpy(text+used,r->text,len+1);used+=len;
        SherpaOnnxDestroyOfflineRecognizerResult(r);SherpaOnnxDestroyOfflineStream(stream);
        SherpaOnnxDestroySpeechSegment(seg);SherpaOnnxVoiceActivityDetectorPop(vad);
      }
    }
    char out[16384];int prefix=snprintf(out,sizeof(out),"{\"kind\":\"final\",\"session\":%u,\"revision\":%u,\"text\":\"",session,revision);
    size_t j=(size_t)prefix;
    for(size_t i=0;i<used;i++) {
      unsigned char ch=(unsigned char)text[i];
      if(j+8>=sizeof(out)) return 6;
      if(ch=='"' || ch=='\\') {out[j++]='\\';out[j++]=(char)ch;}
      else if(ch<32) {j+=(size_t)snprintf(out+j,sizeof(out)-j,"\\u%04x",ch);}
      else out[j++]=(char)ch;
    }
    out[j++]='"';out[j++]='}';out[j]=0;reply(out);
    total=0;SherpaOnnxVoiceActivityDetectorReset(vad);
  }
  free(audio);
  SherpaOnnxDestroyVoiceActivityDetector(vad);SherpaOnnxDestroyOfflineRecognizer(asr);return 0;
}
