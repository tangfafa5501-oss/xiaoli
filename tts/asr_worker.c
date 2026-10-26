// v1: six LE u32 words: XLR5, version, command, session, count, validSamples.
// Commands start=1 chunk=2 finish=3 cancel=4. Response: LE length + JSON.
#include "vendor/c-api.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#ifdef _WIN32
#include <windows.h>
#else
#include <pthread.h>
#endif
#ifdef _WIN32
#include <io.h>
#include <fcntl.h>
#endif
static void reply(const char *s) {
  unsigned n=(unsigned)strlen(s);
  unsigned char h[4]={(unsigned char)n,(unsigned char)(n>>8),(unsigned char)(n>>16),(unsigned char)(n>>24)};
  fwrite(h,1,4,stdout);fwrite(s,1,n,stdout);fflush(stdout);
}
// The retained PCM is also the bounded queue (32 seconds). Only this thread
// touches OnlineStream. The protocol thread locks only to copy PCM/snapshots;
// neither chunk ACKs nor stdout ever wait for a model decode.
#define CAPACITY (32*16000)
static float audio[CAPACITY];
static unsigned total, consumed;
static int stopping, flushing, running;
static char preview[12000];
static const SherpaOnnxOnlineRecognizer *online;
#ifdef _WIN32
static CRITICAL_SECTION mutex;
static CONDITION_VARIABLE changed;
static HANDLE thread;
#define LOCK() EnterCriticalSection(&mutex)
#define UNLOCK() LeaveCriticalSection(&mutex)
#define WAKE() WakeConditionVariable(&changed)
#define WAIT() SleepConditionVariableCS(&changed,&mutex,INFINITE)
#else
static pthread_mutex_t mutex=PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t changed=PTHREAD_COND_INITIALIZER;
static pthread_t thread;
#define LOCK() pthread_mutex_lock(&mutex)
#define UNLOCK() pthread_mutex_unlock(&mutex)
#define WAKE() pthread_cond_signal(&changed)
#define WAIT() pthread_cond_wait(&changed,&mutex)
#endif
#ifdef _WIN32
static DWORD WINAPI decode_thread(LPVOID unused) {
#else
static void *decode_thread(void *unused) {
#endif
  (void)unused;
  const SherpaOnnxOnlineStream *stream=SherpaOnnxCreateOnlineStream(online);
  if(!stream) return 0;
  for(;;) {
    float block[4096];unsigned n;
    LOCK();
    while(consumed==total && !stopping && !flushing) WAIT();
    if(stopping) {UNLOCK();break;}
    n=total-consumed;if(n>4096) n=4096;
    memcpy(block,audio+consumed,n*sizeof(float));consumed+=n;
    int end=flushing && consumed==total;
    UNLOCK();
    if(n) SherpaOnnxOnlineStreamAcceptWaveform(stream,16000,block,(int)n);
    if(end) {
      float tail[12800]={0}; // Official transducer example: 0.8s tail padding.
      SherpaOnnxOnlineStreamAcceptWaveform(stream,16000,tail,12800);
      SherpaOnnxOnlineStreamInputFinished(stream);
    }
    while(SherpaOnnxIsOnlineStreamReady(online,stream)) SherpaOnnxDecodeOnlineStream(online,stream);
    const SherpaOnnxOnlineRecognizerResult *r=SherpaOnnxGetOnlineStreamResult(online,stream);
    if(r && r->text) {
      LOCK();snprintf(preview,sizeof(preview),"%s",r->text);UNLOCK();
    }
    if(r) SherpaOnnxDestroyOnlineRecognizerResult(r);
    if(end) break;
  }
  SherpaOnnxDestroyOnlineStream(stream);return 0;
}
static void stop_decoder(int finish) {
  if(!running) return;
  LOCK();stopping=!finish;flushing=finish;WAKE();UNLOCK();
#ifdef _WIN32
  WaitForSingleObject(thread,INFINITE);CloseHandle(thread);
#else
  pthread_join(thread,NULL);
#endif
  running=0;
}
static int start_decoder(void) {
  LOCK();total=0;consumed=0;stopping=0;flushing=0;preview[0]=0;UNLOCK();
  if(!online) return 1;
#ifdef _WIN32
  thread=CreateThread(NULL,0,decode_thread,NULL,0,NULL);running=thread!=NULL;
#else
  running=pthread_create(&thread,NULL,decode_thread,NULL)==0;
#endif
  return running;
}
static int readable(const char *path) {
  FILE *f=fopen(path,"rb");if(!f) return 0;fclose(f);return 1;
}
static void text_reply(const char *kind,unsigned session,unsigned revision,const char *text) {
  char out[16384];size_t j=(size_t)snprintf(out,sizeof(out),
    "{\"kind\":\"%s\",\"session\":%u,\"revision\":%u,\"text\":\"",kind,session,revision);
  for(size_t i=0;text[i];i++) {
    unsigned char ch=(unsigned char)text[i];
    if(j+8>=sizeof(out)) exit(6);
    if(ch=='"' || ch=='\\') {out[j++]='\\';out[j++]=(char)ch;}
    else if(ch<32) j+=(size_t)snprintf(out+j,sizeof(out)-j,"\\u%04x",ch);
    else out[j++]=(char)ch;
  }
  out[j++]='"';out[j++]='}';out[j]=0;reply(out);
}
int main(void) {
#ifdef _WIN32
  _setmode(_fileno(stdin),_O_BINARY);_setmode(_fileno(stdout),_O_BINARY);
#endif
#ifdef _WIN32
  InitializeCriticalSection(&mutex);InitializeConditionVariable(&changed);
#endif
  SherpaOnnxOnlineRecognizerConfig o={0};
  o.feat_config.sample_rate=16000;o.feat_config.feature_dim=80;
  o.model_config.transducer.encoder="streaming-zh-en/encoder.int8.onnx";
  o.model_config.transducer.decoder="streaming-zh-en/decoder.onnx";
  o.model_config.transducer.joiner="streaming-zh-en/joiner.int8.onnx";
  o.model_config.tokens="streaming-zh-en/tokens.txt";
  o.model_config.num_threads=1;o.model_config.provider="cpu";
  o.decoding_method="greedy_search";o.enable_endpoint=0;
  if(readable(o.model_config.transducer.encoder) && readable(o.model_config.transducer.decoder) &&
     readable(o.model_config.transducer.joiner) && readable(o.model_config.tokens))
    online=SherpaOnnxCreateOnlineRecognizer(&o);
  if(!online) fputs("Streaming model unavailable; final-only fallback\n",stderr);
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
  fputs(online ? "READY partial=1\n" : "READY\n",stdout);fflush(stdout);
  unsigned session=0,revision=0;
  char sent[12000]={0};
  for(;;) {
    unsigned char h[24];if(fread(h,1,24,stdin)!=24) break;
    unsigned w[6];for(unsigned i=0;i<6;i++) w[i]=(unsigned)h[4*i]|((unsigned)h[4*i+1]<<8)|((unsigned)h[4*i+2]<<16)|((unsigned)h[4*i+3]<<24);
    unsigned cmd=w[2],n=w[4],valid=w[5];
    if(w[0]!=0x35524c58 || w[1]!=1 || !w[3] || cmd<1 || cmd>4 ||
       (cmd==2 ? (!n || n>4096 || n%512 || !valid || valid>n) : (n || valid))) break;
    if(cmd!=1 && w[3]!=session) break;
    if(cmd==1) {stop_decoder(0);session=w[3];revision=0;sent[0]=0;
      if(!start_decoder()) return 7;
      SherpaOnnxVoiceActivityDetectorReset(vad);}
    if(cmd==2) {
      float pcm[4096];if(fread(pcm,sizeof(float),n,stdin)!=n) break;
      int ok=1;for(unsigned i=0;i<n;i++) if(!isfinite(pcm[i]) || fabsf(pcm[i])>1) ok=0;
      if(!ok || total+valid>32*16000) break;
      LOCK();memcpy(audio+total,pcm,valid*sizeof(float));total+=valid;WAKE();UNLOCK();
    }
    if(cmd==4) {stop_decoder(0);total=0;SherpaOnnxVoiceActivityDetectorReset(vad);}
    ++revision;
    if(cmd!=3) {
      char snapshot[12000];LOCK();snprintf(snapshot,sizeof(snapshot),"%s",preview);UNLOCK();
      if(cmd==2 && snapshot[0] && strcmp(snapshot,sent)) {
        text_reply("partial",session,revision,snapshot);snprintf(sent,sizeof(sent),"%s",snapshot);continue;
      }
      char out[160];snprintf(out,sizeof(out),"{\"kind\":\"ack\",\"session\":%u,\"revision\":%u}",session,revision);reply(out);
      if(cmd==4) session=0;
      continue;
    }
    stop_decoder(1);
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
    text_reply("final",session,revision,text);
    total=0;SherpaOnnxVoiceActivityDetectorReset(vad);
  }
  stop_decoder(0);
  if(online) SherpaOnnxDestroyOnlineRecognizer(online);
  SherpaOnnxDestroyVoiceActivityDetector(vad);SherpaOnnxDestroyOfflineRecognizer(asr);return 0;
}
