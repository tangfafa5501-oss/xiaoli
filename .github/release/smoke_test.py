"""Windows final-package smoke: real launcher, real chat + CPU Melo, process-tree RSS.
No microphone or physical speaker assertion; WAV endpoint audio is validated."""
import array, io, json, math, os, pathlib, re, subprocess, sys, threading, time
import urllib.request, wave, zipfile
import psutil
from asr_smoke import test_builtin
from playwright.sync_api import sync_playwright

zip_path, work = sys.argv[1:3]
work = pathlib.Path(work)
zipfile.ZipFile(zip_path).extractall(work)
app = work / '小离'
data_dir = pathlib.Path(os.environ['LOCALAPPDATA']) / 'XiaoLi'
port_file = data_dir / 'port.txt'
port_file.unlink(missing_ok=True)
metrics = {'rss_sample_interval_s': 0.1, 'peak_total_rss_mib': 0,
           'peak_by_process_mib': {}, 'browser_included': False}
stop = threading.Event()
launcher = None
known = {}

def post(url, body):
    return urllib.request.urlopen(urllib.request.Request(url, data=json.dumps(body).encode(),
        headers={'Content-Type': 'application/json'}), timeout=120)

def monitor():
    while not stop.is_set():
        try:
            procs = [psutil.Process(launcher.pid)] + psutil.Process(launcher.pid).children(recursive=True)
            total = 0
            for proc in procs:
                try:
                    name = proc.name()
                    if name.lower() not in ('小离.exe', 'llama-server.exe', 'xiaoli-tts.exe', 'xiaoli-asr.exe'): continue
                    known[proc.pid] = name
                    rss = proc.memory_info().rss / 2**20
                    total += rss
                    metrics['peak_by_process_mib'][name] = max(rss, metrics['peak_by_process_mib'].get(name, 0))
                except psutil.Error: pass
            metrics['peak_total_rss_mib'] = max(total, metrics['peak_total_rss_mib'])
        except psutil.Error: pass
        stop.wait(0.1)

def wait_ready(base):
    start = started
    brain_at = None
    while time.perf_counter()-start < 300:
        assert launcher.poll() is None, 'launcher exited'
        try:
            status = json.load(urllib.request.urlopen(base+'/api/status', timeout=5))
        except OSError:
            time.sleep(0.25); continue
        assert not status.get('failed'), status
        if status['ready'] and brain_at is None: brain_at = time.perf_counter()-start
        if status['ready'] and status.get('tts_ready') and status.get('asr_ready'):
            metrics['brain_ready_s'] = brain_at
            metrics['both_ready_s'] = time.perf_counter()-start
            return
        time.sleep(0.25)
    raise AssertionError('brain/TTS readiness timeout')

def test_tts(base, text, filename):
    start = time.perf_counter()
    with post(base+'/api/tts', {'text': text}) as response:
        assert response.headers.get_content_type() == 'audio/wav'
        server_timing = response.headers.get('Server-Timing')
        first = response.read(1)
        ttfb = time.perf_counter()-start
        audio = first + response.read()
    elapsed = time.perf_counter()-start
    (work/filename).write_bytes(audio)
    with wave.open(io.BytesIO(audio)) as wav:
        assert wav.getnchannels() == 1 and wav.getsampwidth() == 2
        sr, frames = wav.getframerate(), wav.getnframes()
        assert sr >= 16000 and frames > sr/5
        samples = array.array('h', wav.readframes(frames))
    rms = math.sqrt(sum(v*v for v in samples)/len(samples))
    assert rms > 10 and max(abs(v) for v in samples) > 100, f'silent WAV: {rms}'
    return {'text':text, 'time_to_first_audio_byte_s':ttfb, 'wav_complete_s':elapsed,
            'server_timing':server_timing, 'audio_duration_s':frames/sr, 'sample_rate':sr, 'rms_pcm16':rms,
            'meaning':'sentence WAV endpoint; excludes sound-card/browser playback'}

try:
    started = time.perf_counter()
    launcher = subprocess.Popen([str(app/'小离.exe')], cwd=app)
    sampler = threading.Thread(target=monitor, daemon=True); sampler.start()
    while not port_file.exists():
        assert launcher.poll() is None, 'launcher exited before port.txt'
        assert time.perf_counter()-started < 60, 'missing port.txt'
        time.sleep(0.1)
    base = 'http://127.0.0.1:' + port_file.read_text().strip()
    html = urllib.request.urlopen(base+'/').read().decode('utf-8')
    assert 'id="voice"' in html and '小离' in html and '/asr-state.js' in html
    wait_ready(base)
    metrics['asr'] = test_builtin(base, app/'models/asr/asr-test.wav')
    metrics['tts'] = [test_tts(base, '早上好！今天天气不错。我在呢，有什么事直接跟我说，我帮你搞定。', 'tts-test.wav')]
    metrics['warm_short_clauses'] = [test_tts(base, text, f'short-{i}.wav') for i,text in enumerate(['早上好！', '今天有什么需要帮忙？']*3)]
    # Run chat while another sentence synthesizes; aggregate RSS includes both workloads.
    concurrent = []
    def during_chat():
        try: concurrent.append(test_tts(base,'今天有什么需要帮忙？','tts-concurrent.wav'))
        except Exception as e: concurrent.append(e)
    t = threading.Thread(target=during_chat)
    reply = ''
    with post(base+'/api/chat', {'messages':[{'role':'user','content':'你好，你是谁？请用两句话回答。'}]}) as response:
        for raw in response:
            line = raw.decode().strip()
            if line.startswith('data:') and line[5:].strip() != '[DONE]':
                obj = json.loads(line[5:])
                delta = obj['choices'][0]['delta'].get('content') or ''
                if delta and not t.ident: t.start()
                reply += delta
    assert t.ident, 'no chat tokens to trigger concurrent TTS'
    t.join(120)
    assert not t.is_alive(), 'concurrent TTS hung'
    assert concurrent and isinstance(concurrent[0],dict), concurrent
    metrics['tts'] += concurrent
    assert reply.strip() and re.search('[\u4e00-\u9fff]',reply), reply
    assert '<think' not in reply.lower(), reply
    metrics['chat_reply'] = reply
    # Real headless WebAudio start, not an HTTP-byte surrogate. No physical speaker.
    with sync_playwright() as pw:
        browser = pw.chromium.launch(args=['--autoplay-policy=no-user-gesture-required'])
        page = browser.new_page(viewport={'width':1024,'height':640})
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.goto(base)
        assert page.title() == '小离'
        page.wait_for_function('!document.querySelector("#send").disabled')
        page.click('#voice'); page.click('#voice')  # unlock and clear greeting
        page.evaluate('playChain')
        page.evaluate("""() => {
          window.latencyPlays = [];
          const info = console.info;
          console.info = (...args) => {
            if (args[0] === 'TTS_PLAY') window.latencyPlays.push(JSON.parse(args[1]));
            info(...args);
          };
        }""")
        page.fill('#input', '请只回复：早上好！今天有什么需要帮忙？')
        page.press('#input', 'Enter')
        page.wait_for_function('window.latencyPlays.length > 0', timeout=120000)
        metrics['browser_first_play'] = page.evaluate('window.latencyPlays[0]')
        page.wait_for_function('!document.querySelector("#send").disabled', timeout=120000)
        page.evaluate('playChain')
        metrics['browser_plays'] = page.evaluate('window.latencyPlays')
        metrics['browser_errors'] = errors
        assert not errors, errors
        page.screenshot(path=str(work/'browser-chat.png'))
        browser.close()
    metrics['warm_short_target_1s_met'] = all(x['wav_complete_s'] <= 1.0 for x in metrics['warm_short_clauses'])
    metrics['concurrent_short_target_1s_met'] = concurrent[0]['wav_complete_s'] <= 1.0

    assert 'llama-server.exe' in known.values() and 'xiaoli-tts.exe' in known.values() and 'xiaoli-asr.exe' in known.values(), known
    # Killing the launcher must close all Windows child-process job handles.
    launcher.kill(); launcher.wait()
    time.sleep(3)
    survivors = {pid:name for pid,name in list(known.items()) if pid != launcher.pid and psutil.pid_exists(pid)}
    assert not survivors, f'children survived launcher: {survivors}'
    metrics['children_stopped_on_launcher_exit'] = True
    metrics['passed'] = True
    print('SMOKE TEST PASSED', json.dumps(metrics,ensure_ascii=False,indent=2))
finally:
    stop.set()
    if launcher is not None and launcher.poll() is None: launcher.kill(); launcher.wait()
    for pid in list(known):
        try: psutil.Process(pid).kill()
        except psutil.Error: pass
    metrics.setdefault('passed',False)
    tts_log = data_dir/'tts.log'
    if tts_log.exists():
        log_text = tts_log.read_text(encoding='utf-8',errors='replace')
        metrics['worker_timings'] = [dict(re.findall(r'(\w+)=([\d.]+)', line)) for line in log_text.splitlines() if line.startswith('TTS ')]
    (work/'metrics.json').write_text(json.dumps(metrics,ensure_ascii=False,indent=2),encoding='utf-8')
    for filename in ['launcher.log','tts.log','asr.log','brain.log']:
        source = data_dir/filename
        if source.exists():
            (work/filename).write_bytes(source.read_bytes())
            print(filename,source.read_text(encoding='utf-8',errors='replace')[-4000:])
    if os.environ.get('GITHUB_STEP_SUMMARY'):
        with open(os.environ['GITHUB_STEP_SUMMARY'],'a',encoding='utf-8') as f:
            f.write('## Windows v0.5-dev smoke\n```json\n'+json.dumps(metrics,ensure_ascii=False,indent=2)+'\n```\n')
