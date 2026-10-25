"""Build the final Windows zip: base zip (everything except the model) + the model file.
Keeps the 小离/... layout and UTF-8 file names (zipfile sets the UTF-8 flag for non-ASCII names)."""
import hashlib, sys, zipfile, pathlib

base, model, expected_sha, out, tts_stage, launcher = sys.argv[1:7]

h = hashlib.sha256()
with open(model, "rb") as f:
    for chunk in iter(lambda: f.read(1 << 20), b""):
        h.update(chunk)
got = h.hexdigest()
print("model sha256:", got)
if got != expected_sha:
    sys.exit(f"model sha256 mismatch: expected {expected_sha}")

with zipfile.ZipFile(base) as zb, zipfile.ZipFile(out, "w") as zo:
    names = []
    overlays = {"小离/" + f.relative_to(tts_stage).as_posix() for f in pathlib.Path(tts_stage).rglob("*") if f.is_file()}
    overlays.add("小离/小离.exe")
    for info in zb.infolist():
        if info.filename in overlays:
            continue
        data = zb.read(info.filename)
        ni = zipfile.ZipInfo(info.filename, date_time=info.date_time)
        ni.external_attr = info.external_attr
        zo.writestr(ni, data, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
        names.append(info.filename)
    zo.write(launcher, "小离/小离.exe", compress_type=zipfile.ZIP_DEFLATED)
    for f in sorted(pathlib.Path(tts_stage).rglob("*")):
        if f.is_file():
            zo.write(f, "小离/" + f.relative_to(tts_stage).as_posix(), compress_type=zipfile.ZIP_DEFLATED)
    target = "小离/runtime/model.gguf"
    assert target not in names
    zo.write(model, target, compress_type=zipfile.ZIP_STORED)

with zipfile.ZipFile(out) as z:
    assert z.testzip() is None
    infos = z.infolist()
    assert len(z.namelist()) == len(set(z.namelist())), "duplicate zip entries"
    print("entries:", len(infos))
    bad = [i.filename for i in infos if not i.filename.isascii() and not (i.flag_bits & 0x800)]
    assert not bad, bad
    for n in ["小离/小离.exe", "小离/使用说明.txt", "小离/runtime/llama-server.exe", target, "小离/models/tts/LICENSE", "小离/models/tts/README.md", "小离/runtime/tts/xiaoli-tts.exe", "小离/THIRD_PARTY_NOTICES.txt", "小离/models/asr/model.int8.onnx", "小离/models/asr/tokens.txt", "小离/models/asr/silero_vad.onnx", "小离/models/asr/asr-test.wav", "小离/models/asr/LICENSE", "小离/runtime/tts/xiaoli-asr.exe", "小离/runtime/tts/licenses/LICENSE-SenseVoice", "小离/runtime/tts/licenses/LICENSE-Silero"]:
        assert n in z.namelist(), n
print("assembled", out)
