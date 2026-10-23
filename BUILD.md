# 小离 v0.3 构建说明

v0.3 在 v0.2 的 Go 启动器、嵌入网页和 llama-server 上增加常驻原生 CPU TTS 子进程。Go 不启用 CGO，llama-server 的参数与隐藏窗口行为不变。

## 构建 Go 启动器

```sh
cd launcher
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o ../小离.exe .
CGO_ENABLED=0 go build -o ../xiaoli .
```

受限环境可设置 `GOCACHE=/tmp/xiaoli-go-cache GOMODCACHE=/tmp/xiaoli-go-mod`。保留已提交的 `rsrc_windows_amd64.syso`（图标和 manifest）；重新生成使用 `go-winres simply --icon ../icon.png --manifest gui`。

## TTS 和完整包

- `tts/worker.c` 只使用 sherpa-onnx 1.13.8 C API。启动一次，Melo FP32、sid=0、speed=1、CPU、2 线程。没有 Python 运行时。
- `tts/vendor/` 保留该版本原始头文件和依赖许可证。`prepare_tts.py` 下载官方 Windows x64 shared MT 库和 Melo 模型包，下载后校验固定 SHA256，保留完整词典、模型 LICENSE / README，不携带未使用的 int8 占位文件。
- 模型文件来源优先检查过 ModelScope；未找到已验证的 sherpa Melo 包镜像，CI 使用 GitHub。不能把 ModelScope 的原始 checkpoint 直接改名当作 ONNX。
- 包布局：`小离/小离.exe`、`runtime/llama-server.exe` 和 DLL / `model.gguf`、`runtime/tts/xiaoli-tts.exe` 和 DLL / `licenses/`、`models/tts/`、`使用说明.txt`、`THIRD_PARTY_NOTICES.txt`、`TTS-ASSETS-SHA256.json`。
- TTS 子进程 cwd 为 `models/tts`，使用相对模型路径，支持中文安装路径。二进制 stdin：小端 uint32 长度 + UTF-8；stdout：启动 `READY\n`，然后每请求 uint32 长度 + PCM16 WAV。stderr 独立日志。每个请求最多 240 字，单次合成 30 秒超时，失效后静默降级；Windows job object 确保父进程退出时一起关闭。

Windows（VS x64 开发命令提示符，仓库根目录）：

```bat
python .github\release\prepare_tts.py build\tts-cache build\tts-stage
cl /nologo /O2 /MT /utf-8 /I tts tts\worker.c /Fe:build\tts-stage\runtime\tts\xiaoli-tts.exe /link /LIBPATH:build\tts-cache\sherpa-onnx-v1.13.8-win-x64-shared-MT-Release-lib\lib sherpa-onnx-c-api.lib
```

Linux 验证使用相同版本 `sherpa-onnx-v1.13.8-linux-x64-shared-lib.tar.bz2`（SHA256 `3892d184be41027e18165e67f549cd4e4cdd8dcd73ac5579e97afd55e14e30b6`）：

```sh
gcc -O2 -Wall -Wextra tts/worker.c -L/path/to/sherpa/lib -lsherpa-onnx-c-api '-Wl,-rpath,$ORIGIN' -o /path/to/app/runtime/tts/xiaoli-tts
```

该目录同时复制 `libsherpa-onnx-c-api.so`、`libonnxruntime.so`；在 `runtime/` 放 Linux llama-server b11799、依赖 so、Qwen3 GGUF 和 persona.txt。模型使用与 Windows 相同目录布局。

## 网页语音行为

`/api/tts` 仅监听本机，接收 POST JSON `{"text":"早上好！"}`，返回 `audio/wav`，503 代表静默降级。`/api/status` 的 `tts_ready` 独立于聊天的 `ready`。

按 `。！？!?；;…\n` 分句，长句在 24–72 字之间优先取逗号，否则 72 字截断；流结束补发尾句。音频合成串行且与播放重叠，WebAudio 顺序播放；每句开始时显示该句。静音/浏览器未解锁时流式显示文字，失败时按顺序直接显示。emoji、常见 Markdown 和「～」不会朗读。静音和新问题会取消旧待播队列。首句目标是已加载模型、短句末尾后约 1 秒，不包含 LLM 等待，不保证慢电脑和长句。

UI 默认声音开启，欢迎语默认文字；首次实际播放依赖点击/键盘解锁浏览器音频。localStorage 保存静音，启动器在 `%LOCALAPPDATA%\XiaoLi\ui-port.txt` 保存端口并优先复用，保持同一 origin；端口被占用时回退随机端口，设置可能重置。日志含朗读文本，位于同一目录。

## 发布流程（须 fafa 单独批准；此处只是操作说明）

1. 审查本地修改和测试报告；提交代码。批准后再 push（push 本身不会触发此 workflow，它只有 workflow_dispatch）。
2. 批准后为目标提交创建 **draft** release，tag=`v0.3`。
3. 上传 `XiaoLi-v0.3-base-no-model.zip` 到该 draft。base 包只包含小离 exe、既有 llama Windows DLL/许可、persona、使用说明；不包含 GGUF 和 TTS 大资源，必须小于约 60 MB。
   本地重建 base（仓库根目录）：

   ```sh
   python .github/release/make_base.py /workspace/xl/XiaoLi-v0.2.zip /workspace/xiaoli-tts/downloads/小离.exe /workspace/xiaoli-tts/dist/XiaoLi-v0.3-base-no-model.zip
   ```

4. 手动运行 `.github/workflows/release-windows.yml`，tag=`v0.3`，先 `publish=false`。runner 从仓库重新编译启动器和 worker，下载并校验模型与 TTS 资产，装配完整 zip，运行真实 Windows smoke。检查 `windows-v0.3-validation` artifact 内的 WAV / metrics / 日志，确认非静音、首音频时间、合计进程 RSS 和退出清理。
5. fafa 批准正式发布后，再以同一 tag 和同一提交运行 `publish=true`。再次构建和测试成功后上传完整 zip、删除 base asset、公开 draft。仅 `publish=false` 不上传 release 资产、不公开 draft；完整 zip 保留在 runner 作测试，本流程不把它作为 Actions artifact（较大）。

Smoke 每 100 ms 对启动器进程树中的 小离.exe、llama-server.exe、xiaoli-tts.exe 采样 RSS，记录同时刻合计峰值和各进程峰值（不含 Edge）。TTS 首音频是 WAV endpoint 首字节时间，不是实体声卡播放延迟。失败使 job 失败；无论成功与否保存 metrics 和日志。Windows 实机、声音设备和主观音质仍需人工确认。
