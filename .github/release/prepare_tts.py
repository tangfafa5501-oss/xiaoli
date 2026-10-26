"""Pinned offline TTS assets. Run on CI; large model never enters base zip."""
import hashlib, json, pathlib, shutil, sys, tarfile, urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
STREAMING = json.loads((ROOT / "tts/streaming-model.json").read_text(encoding="utf-8"))
ASSETS = {
    "streaming-zh-en.tar.bz2": (STREAMING["url"], STREAMING["sha256"]),
    "sensevoice-int8.tar.bz2": ("https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2",
        "7d1efa2138a65b0b488df37f8b89e3d91a60676e416f515b952358d83dfd347e"),
    "silero_vad.onnx": ("https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx",
        "9e2449e1087496d8d4caba907f23e0bd3f78d91fa552479bb9c23ac09cbb1fd6"),
    'melo.tar.bz2': ('https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-melo-tts-zh_en.tar.bz2',
                     'e58351ed7149f290a54534538badd4077cdbe6fddc964b24d0bee870415d1514'),
    'sherpa-win.tar.bz2': ('https://github.com/k2-fsa/sherpa-onnx/releases/download/v1.13.8/sherpa-onnx-v1.13.8-win-x64-shared-MT-Release-lib.tar.bz2',
                           'b8eedf41bd6d3779218887b48367bb7a3ece5aaa7667f01f69ee823a12b0a9e7'),
}

def prepare(cache, stage):
    cache.mkdir(parents=True, exist_ok=True)
    manifest = {}
    for name, (url, expected) in ASSETS.items():
        dest = cache / name
        if not dest.exists():
            for attempt in range(3):
                try:
                    urllib.request.urlretrieve(url, dest)
                    break
                except Exception:
                    dest.unlink(missing_ok=True)
                    if attempt == 2: raise
        with dest.open('rb') as f:
            digest = hashlib.file_digest(f, 'sha256').hexdigest()
        if digest != expected: raise ValueError(f'{name}: SHA256 {digest} != {expected}')
        manifest[name] = {'url': url, 'sha256': digest}
        if name.endswith('.tar.bz2'):
            with tarfile.open(dest) as tar:
                tar.extractall(cache, filter='data')
    model = stage / 'models/tts'
    if model.exists(): shutil.rmtree(model)
    shutil.copytree(cache / 'vits-melo-tts-zh_en', model)
    # FP32 chosen in evaluation. No unused int8 duplicate in distribution.
    (model / 'model.int8.onnx').unlink(missing_ok=True)
    asr = stage / 'models/asr'
    if asr.exists(): shutil.rmtree(asr)
    asr.mkdir(parents=True)
    source = cache / 'sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17'
    for name in ['model.int8.onnx', 'tokens.txt', 'LICENSE', 'README.md']:
        shutil.copy2(source / name, asr / name)
    streaming = asr / 'streaming-zh-en'
    streaming.mkdir()
    for name, expected in STREAMING['files'].items():
        src = cache / STREAMING['name'] / name
        with src.open('rb') as f:
            digest = hashlib.file_digest(f, 'sha256').hexdigest()
        if src.stat().st_size != expected['bytes'] or digest != expected['sha256']:
            raise ValueError(f'streaming model file mismatch: {name}')
        shutil.copy2(src, streaming / name)
    shutil.copy2(ROOT / 'tts/vendor/LICENSE-X-ASR', streaming / 'LICENSE')
    shutil.copy2(ROOT / 'tts/streaming-model.json', streaming / 'MODEL.json')
    shutil.copy2(cache / STREAMING['name'] / 'README.md', streaming)
    manifest['streaming_deployment'] = STREAMING
    shutil.copy2(cache / 'silero_vad.onnx', asr)
    shutil.copy2(ROOT / 'tts/testdata/asr-test.wav', asr)
    runtime = stage / 'runtime/tts'
    runtime.mkdir(parents=True, exist_ok=True)
    lib = cache / 'sherpa-onnx-v1.13.8-win-x64-shared-MT-Release-lib/lib'
    for name in ['sherpa-onnx-c-api.dll', 'onnxruntime.dll', 'onnxruntime_providers_shared.dll']:
        shutil.copy2(lib / name, runtime / name)
    shutil.copytree(ROOT / 'tts/vendor', runtime / 'licenses', dirs_exist_ok=True)
    # Header needed to compile but not useful in the end-user package.
    (runtime / 'licenses/c-api.h').unlink(missing_ok=True)
    shutil.copy2(ROOT / 'THIRD_PARTY_NOTICES.txt', stage)
    (stage / 'TTS-ASSETS-SHA256.json').write_text(json.dumps(manifest, indent=2), encoding='utf-8')
    print(json.dumps(manifest, indent=2))

if __name__ == '__main__':
    prepare(pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]))
