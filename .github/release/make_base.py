"""Reuse the small v0.2 Windows runtime, refresh launcher and user docs.
Usage: python make_base.py original-full.zip newly-built.exe output-base.zip
Large LLM/TTS assets are downloaded separately by the workflow."""
import pathlib, sys, zipfile
root = pathlib.Path(__file__).resolve().parents[2]
source, launcher, out = sys.argv[1:4]
with zipfile.ZipFile(source) as old, zipfile.ZipFile(out, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as new:
    for info in old.infolist():
        n = info.filename
        if n.startswith('小离/runtime/') and not n.endswith('model.gguf'):
            if '/tts/' not in n and not info.is_dir():
                new.writestr(info.filename, old.read(info.filename))
    new.write(launcher,'小离/小离.exe')
    for name in ['使用说明.txt','THIRD_PARTY_NOTICES.txt']:
        new.write(root/name,'小离/'+name)
assert pathlib.Path(out).stat().st_size < 60_000_000, 'base must stay under 60 MB'
with zipfile.ZipFile(out) as z:
    assert z.testzip() is None
    assert not any(n.endswith('.gguf') or '/models/' in n for n in z.namelist())
print(out, pathlib.Path(out).stat().st_size, 'bytes')
