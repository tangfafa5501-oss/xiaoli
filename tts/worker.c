// Persistent CPU-only Melo worker. stdin: LE uint32 byte count + UTF-8;
// stdout: READY\n once, then LE uint32 byte count + PCM16 mono WAV.
// Working directory is models/tts (relative paths also support Unicode install paths).
#include "vendor/c-api.h"
#include <stdio.h>
#include <stdlib.h>
#include <math.h>
#include <time.h>
#ifdef _WIN32
#include <windows.h>
static double now_ms(void) { LARGE_INTEGER f, t; QueryPerformanceFrequency(&f); QueryPerformanceCounter(&t); return 1000.0*t.QuadPart/f.QuadPart; }
#else
static double now_ms(void) { struct timespec t; clock_gettime(CLOCK_MONOTONIC, &t); return t.tv_sec*1000.0+t.tv_nsec/1000000.0; }
#endif
#ifdef _WIN32
#include <io.h>
#include <fcntl.h>
#endif
static void u16(unsigned v) { putchar(v & 255); putchar((v >> 8) & 255); }
static void u32(unsigned v) { u16(v); u16(v >> 16); }
int main(void) {
#ifdef _WIN32
  _setmode(_fileno(stdin), _O_BINARY);
  _setmode(_fileno(stdout), _O_BINARY);
#endif
  SherpaOnnxOfflineTtsConfig c = {0};
  c.model.vits.model = "model.onnx";
  c.model.vits.lexicon = "lexicon.txt";
  c.model.vits.tokens = "tokens.txt";
  c.model.vits.dict_dir = "dict";
  c.model.vits.noise_scale = 0.667f;
  c.model.vits.noise_scale_w = 0.8f;
  c.model.vits.length_scale = 1;
  c.model.num_threads = 2;
  c.model.provider = "cpu";
  c.rule_fsts = "phone.fst,date.fst,number.fst";
  c.max_num_sentences = 1;
  c.silence_scale = 0.2f;
  double load_start = now_ms();
  const SherpaOnnxOfflineTts *tts = SherpaOnnxCreateOfflineTts(&c);
  if (!tts) return 2;
  fprintf(stderr,"TTS load_ms=%.3f threads=2 sid=0 speed=1 native_rate=1\n",now_ms()-load_start);
  // Exercise lazy jieba/lexicon and ORT kernels before advertising readiness.
  SherpaOnnxGenerationConfig warm = {0};
  warm.sid=0; warm.speed=1; warm.silence_scale=0.2f;
  double cold_start = now_ms();
  const SherpaOnnxGeneratedAudio *cold = SherpaOnnxOfflineTtsGenerateWithConfig(tts,"早上好！",&warm,NULL,NULL);
  fprintf(stderr,"TTS cold_first_call frontend_onnx_ms=%.3f samples=%d\n",now_ms()-cold_start,cold ? cold->n : 0);
  if (!cold || cold->n <= 0) return 3;
  SherpaOnnxDestroyOfflineTtsGeneratedAudio(cold);
  fputs("READY\n", stdout); fflush(stdout);
  for (;;) {
    unsigned char h[4];
    if (fread(h,1,4,stdin) != 4) break;
    unsigned n = h[0] | (h[1]<<8) | (h[2]<<16) | ((unsigned)h[3]<<24);
    if (!n || n > 4096) break;
    char text[4097];
    if (fread(text,1,n,stdin) != n) break;
    text[n]=0;
    SherpaOnnxGenerationConfig g = {0};
    g.sid=0; g.speed=1; g.silence_scale=0.2f;
    double begin = now_ms();
    const SherpaOnnxGeneratedAudio *a = SherpaOnnxOfflineTtsGenerateWithConfig(tts,text,&g,NULL,NULL);
    if (!a || a->n <= 0 || a->n > 2400000 || a->sample_rate <= 0) {
      if (a) SherpaOnnxDestroyOfflineTtsGeneratedAudio(a);
      u32(0); fflush(stdout); continue;
    }
    double generated = now_ms();
    unsigned bytes=(unsigned)a->n*2;
    short *pcm = (short *)malloc(bytes);
    if (!pcm) return 4;
    for (int i=0;i<a->n;i++) {
      float v=a->samples[i]; if (!isfinite(v)) v=0;
      if(v>1) v=1;
      if(v< -1) v= -1;
      pcm[i]=(short)(v*32767);
    }
    double encoded = now_ms();
    u32(44+bytes);
    fwrite("RIFF",1,4,stdout); u32(36+bytes); fwrite("WAVEfmt ",1,8,stdout);
    u32(16); u16(1); u16(1); u32(a->sample_rate); u32(a->sample_rate*2);
    u16(2); u16(16); fwrite("data",1,4,stdout); u32(bytes);
    fwrite(pcm,1,bytes,stdout); fflush(stdout);
    fprintf(stderr,"TTS request utf8_bytes=%u frontend_onnx_ms=%.3f pcm_encode_ms=%.3f pipe_write_ms=%.3f resample_ms=0 samples=%d rate=%d\n",n,generated-begin,encoded-generated,now_ms()-encoded,a->n,a->sample_rate);
    free(pcm);
    SherpaOnnxDestroyOfflineTtsGeneratedAudio(a);
  }
  SherpaOnnxDestroyOfflineTts(tts);
  return 0;
}
