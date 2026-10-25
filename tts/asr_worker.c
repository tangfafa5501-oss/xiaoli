// Persistent SenseVoice + Silero worker. LE u32 sample count + float32 mono 16k.
// Count 0 resets a session. Reply: LE u32 length + UTF-8 JSON. DLLs shared with TTS.
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
  unsigned total=0;
  for(;;) {
    unsigned char h[4];if(fread(h,1,4,stdin)!=4) break;
    unsigned n=h[0]|(h[1]<<8)|(h[2]<<16)|((unsigned)h[3]<<24);
    if(n==0) {SherpaOnnxVoiceActivityDetectorReset(vad);total=0;reply("{\"done\":false}");continue;}
    if(n>4096 || n%512) break;
    float pcm[4096];if(fread(pcm,sizeof(float),n,stdin)!=n) break;
    int valid=1;for(unsigned i=0;i<n;i++) if(!isfinite(pcm[i]) || fabsf(pcm[i])>1) valid=0;
    if(!valid) break;
    total+=n;
    for(unsigned i=0;i<n;i+=512) SherpaOnnxVoiceActivityDetectorAcceptWaveform(vad,pcm+i,512);
    if(SherpaOnnxVoiceActivityDetectorEmpty(vad) && total<30*16000) {reply("{\"done\":false}");continue;}
    if(SherpaOnnxVoiceActivityDetectorEmpty(vad)) SherpaOnnxVoiceActivityDetectorFlush(vad);
    if(SherpaOnnxVoiceActivityDetectorEmpty(vad)) {reply("{\"done\":true,\"text\":\"\"}");continue;}
    const SherpaOnnxSpeechSegment *seg=SherpaOnnxVoiceActivityDetectorFront(vad);
    const SherpaOnnxOfflineStream *s=SherpaOnnxCreateOfflineStream(asr);
    SherpaOnnxAcceptWaveformOffline(s,16000,seg->samples,seg->n);
    SherpaOnnxDecodeOfflineStream(asr,s);
    const SherpaOnnxOfflineRecognizerResult *r=SherpaOnnxGetOfflineStreamResult(s);
    if(!r || !r->json) return 4;
    // r->json already escapes model output; nest it instead of hand-escaping text.
    size_t cap=strlen(r->json)+80;char *out=(char *)malloc(cap);if(!out) return 5;
    snprintf(out,cap,"{\"done\":true,\"result\":%s}",r->json);reply(out);free(out);
    SherpaOnnxDestroyOfflineRecognizerResult(r);SherpaOnnxDestroyOfflineStream(s);
    SherpaOnnxDestroySpeechSegment(seg);SherpaOnnxVoiceActivityDetectorReset(vad);total=0;
  }
  SherpaOnnxDestroyVoiceActivityDetector(vad);SherpaOnnxDestroyOfflineRecognizer(asr);return 0;
}
