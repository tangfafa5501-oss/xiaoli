"""Build the final Windows zip: base zip (everything except the model) + the model file.
Keeps the 小离/... layout and UTF-8 file names (zipfile sets the UTF-8 flag for non-ASCII names)."""
import hashlib, sys, zipfile

base, model, expected_sha, out = sys.argv[1:5]

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
    for info in zb.infolist():
        data = zb.read(info.filename)
        ni = zipfile.ZipInfo(info.filename, date_time=info.date_time)
        ni.external_attr = info.external_attr
        zo.writestr(ni, data, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
        names.append(info.filename)
    target = "小离/runtime/model.gguf"
    assert target not in names
    zo.write(model, target, compress_type=zipfile.ZIP_STORED)

with zipfile.ZipFile(out) as z:
    assert z.testzip() is None
    infos = z.infolist()
    print("entries:", len(infos))
    bad = [i.filename for i in infos if not i.filename.isascii() and not (i.flag_bits & 0x800)]
    assert not bad, bad
    for n in ["小离/小离.exe", "小离/使用说明.txt", "小离/runtime/llama-server.exe", target]:
        assert n in z.namelist(), n
print("assembled", out)
