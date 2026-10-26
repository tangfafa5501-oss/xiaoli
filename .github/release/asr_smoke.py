"""Exercise the exact browser -> launcher -> Silero -> SenseVoice protocol without a mic."""
import array, json, math, pathlib, time, urllib.request, wave


def request(base, body):
    with urllib.request.urlopen(urllib.request.Request(base + '/api/asr',
            data=json.dumps(body).encode(), headers={'Content-Type': 'application/json'}), timeout=15) as r:
        return json.load(r)


def recognize(base, filename, realtime=False):
    with wave.open(str(filename)) as w:
        assert (w.getnchannels(), w.getsampwidth(), w.getframerate()) == (1, 2, 16000)
        pcm = array.array('h', w.readframes(w.getnframes()))
    # The last audible sample is an energy proxy; report this definition explicitly.
    last_voice = max(i for i, v in enumerate(pcm) if abs(v) > 200) / 16000
    samples = [v / 32768 for v in pcm]
    session = request(base, {'action': 'start'})['session']
    started = time.perf_counter()
    try:
        for i in range(0, len(samples), 1024):
            chunk = samples[i:i + 1024]
            valid = len(chunk)
            chunk += [0.] * ((-valid) % 512)
            if realtime:
                time.sleep(max(0, started + (i + valid) / 16000 - time.perf_counter()))
            result = request(base, {'action': 'chunk', 'session': session,
                                   'samples': chunk, 'validSamples': valid})
            assert result['kind'] == 'ack' and result['session'] == session, result
        t = time.perf_counter()
        result = request(base, {'action': 'finish', 'session': session})
        assert result['kind'] == 'final' and result['session'] == session, result
        text = result['text'].strip()
        assert text, result
        return {'text': text, 'finish_to_text_s': time.perf_counter()-t,
                'end_to_text_s': time.perf_counter()-started-last_voice if realtime else None,
                'last_voice_energy_threshold_pcm16': 200,
                'endpoint_audio_s': len(samples)/16000,
                'last_voice_audio_s': last_voice, 'realtime': realtime}
    finally:
        try: request(base, {'action': 'cancel', 'session': session})
        except Exception: pass  # completed sessions are already released


def test_builtin(base, filename):
    result = recognize(base, filename)
    assert '天气' in result['text'] and '怎么样' in result['text'], result
    return result
