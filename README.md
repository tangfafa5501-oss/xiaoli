# 小离 v0.1

一个住在你电脑里的 AI 小帮手。完全离线运行，不用登录、不用联网、不用设置。

## 下载和使用（Windows 10/11 64 位）

1. 到 [Releases](../../releases) 下载 `XiaoLi-v0.1-Windows-x64.zip`，右键选「全部解压缩」，不要在压缩包里直接双击。
2. 打开解压出来的「小离」文件夹，双击「小离.exe」。如果 Windows 提示有风险，点「更多信息」再点「仍要运行」。
3. 等几秒小离会先打招呼，在下面打字、按回车就能聊；关掉窗口她就去休息了。

需要约 2 GB 空闲内存，只用 CPU。

## 目前的限制

- 关掉窗口后聊天记录不保存，没有长期记忆。
- 1.7B 小模型，有时回答比较单薄。
- 还没有语音和动画。
- 程序没有数字签名，所以会有 SmartScreen 提示。

## 组成

- 启动器：Go（见 `launcher/`，构建说明见 `BUILD.md`）
- 推理：[llama.cpp b11799](https://github.com/ggml-org/llama.cpp/releases/tag/b11799) Windows CPU 版
- 模型：[Qwen3-1.7B Q4_K_M](https://huggingface.co/unsloth/Qwen3-1.7B-GGUF)（Apache-2.0）
