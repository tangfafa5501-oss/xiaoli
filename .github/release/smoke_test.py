"""Real-Windows smoke test of the final zip.
A: start runtime\\llama-server.exe with the launcher's arguments and chat once.
B: start 小离.exe, wait until it reports ready, chat once through its /api/chat."""
import json, os, re, subprocess, sys, time, urllib.request, zipfile

zip_path, work = sys.argv[1], sys.argv[2]
zipfile.ZipFile(zip_path).extractall(work)
app = os.path.join(work, "小离")
rt = os.path.join(app, "runtime")
persona = open(os.path.join(rt, "persona.txt"), encoding="utf-8").read()
summary = []

def check_reply(text, label):
    print(f"[{label}] reply: {text!r}")
    assert text.strip(), f"{label}: empty reply"
    assert "<think" not in text.lower(), f"{label}: thinking text leaked"
    assert re.search(r"[\u4e00-\u9fff]", text), f"{label}: reply has no Chinese"
    summary.append(f"- {label}: {text.strip()}")

def post(url, body, timeout=300):
    req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={"Content-Type": "application/json"})
    return urllib.request.urlopen(req, timeout=timeout)

def wait_http(url, ok, proc=None, limit=300, logfile=None):
    t = time.time()
    while time.time() - t < limit:
        if proc is not None and proc.poll() is not None:
            dump(logfile)
            sys.exit(f"process exited early with code {proc.returncode}")
        try:
            r = urllib.request.urlopen(url, timeout=5)
            if ok(r):
                return time.time() - t
        except Exception:
            pass
        time.sleep(1)
    dump(logfile)
    sys.exit(f"timeout waiting for {url}")

def dump(path):
    if path and os.path.exists(path):
        print(f"----- {path} -----")
        print(open(path, encoding="utf-8", errors="replace").read()[-6000:])

# ---------- A: llama-server directly ----------
log_a = os.path.join(work, "server_a.log")
args = [os.path.join(rt, "llama-server.exe"), "-m", os.path.join(rt, "model.gguf"),
        "--host", "127.0.0.1", "--port", "18080", "-c", "4096", "-np", "1",
        "--reasoning", "off", "--chat-template-kwargs", '{"enable_thinking":false}',
        "--no-webui", "--cache-ram", "256"]
with open(log_a, "w") as lf:
    p = subprocess.Popen(args, cwd=rt, stdout=lf, stderr=subprocess.STDOUT, creationflags=0x08000000)
dt = wait_http("http://127.0.0.1:18080/health", lambda r: r.status == 200, p, logfile=log_a)
print(f"[A] llama-server ready in {dt:.1f}s")
t = time.time()
r = json.load(post("http://127.0.0.1:18080/v1/chat/completions", {
    "messages": [{"role": "system", "content": persona}, {"role": "user", "content": "你好，你是谁？"}],
    "max_tokens": 200, "temperature": 0.7, "top_p": 0.8, "top_k": 20,
    "chat_template_kwargs": {"enable_thinking": False}}))
print(f"[A] reply took {time.time()-t:.1f}s, usage {r.get('usage')}")
check_reply(r["choices"][0]["message"]["content"], "llama-server 直接调用（你好，你是谁？）")
p.kill(); p.wait()
dump(log_a)
time.sleep(2)

# ---------- B: the real launcher ----------
data_dir = os.path.join(os.environ["LOCALAPPDATA"], "XiaoLi")
port_file = os.path.join(data_dir, "port.txt")
if os.path.exists(port_file):
    os.remove(port_file)
launcher = subprocess.Popen([os.path.join(app, "小离.exe")], cwd=app)
t = time.time()
while not os.path.exists(port_file):
    if launcher.poll() is not None:
        dump(os.path.join(data_dir, "launcher.log"))
        sys.exit(f"小离.exe exited early with code {launcher.returncode}")
    if time.time() - t > 60:
        sys.exit("小离.exe never wrote port.txt")
    time.sleep(0.5)
port = open(port_file).read().strip()
print("[B] launcher UI port", port)
base = f"http://127.0.0.1:{port}"
html = urllib.request.urlopen(base + "/", timeout=10).read().decode("utf-8")
assert "小离" in html, "UI page did not load"

def status_ok(r):
    s = json.load(r)
    if s.get("failed"):
        dump(os.path.join(data_dir, "brain.log"))
        sys.exit("launcher reports failure: " + s.get("message", ""))
    return s.get("ready")
dt = wait_http(base + "/api/status", status_ok, launcher, logfile=os.path.join(data_dir, "launcher.log"))
print(f"[B] launcher brain ready in {dt:.1f}s")
tl = subprocess.run(["tasklist", "/FI", "IMAGENAME eq llama-server.exe"], capture_output=True, text=True).stdout
print(tl)
assert "llama-server.exe" in tl, "launcher did not start llama-server.exe"

resp = post(base + "/api/chat", {"messages": [
    {"role": "assistant", "content": "下午好呀！我是小离～"},
    {"role": "user", "content": "有人发短信让我点链接退快递费，还要验证码，是真的吗？"}]})
text = ""
for raw in resp:
    line = raw.decode("utf-8").strip()
    if line.startswith("data:") and line[5:].strip() != "[DONE]":
        try:
            d = json.loads(line[5:])["choices"][0]["delta"].get("content")
            text += d or ""
        except Exception:
            pass
check_reply(re.sub(r"<think>.*?</think>", "", text, flags=re.S), "小离.exe 启动器（诈骗短信问题）")

launcher.kill(); launcher.wait()
time.sleep(3)
tl = subprocess.run(["tasklist", "/FI", "IMAGENAME eq llama-server.exe"], capture_output=True, text=True).stdout
print("[B] after killing launcher:", "llama-server still running!" if "llama-server.exe" in tl else "llama-server stopped (job object works)")
summary.append("- 关掉启动器后大脑进程：" + ("仍在运行" if "llama-server.exe" in tl else "已自动退出"))
dump(os.path.join(data_dir, "launcher.log"))
subprocess.run(["taskkill", "/F", "/IM", "msedge.exe"], capture_output=True)
subprocess.run(["taskkill", "/F", "/IM", "llama-server.exe"], capture_output=True)

out = os.environ.get("GITHUB_STEP_SUMMARY")
if out:
    with open(out, "a", encoding="utf-8") as f:
        f.write("## Windows 冒烟测试通过\n" + "\n".join(summary) + "\n")
print("SMOKE TEST PASSED")
