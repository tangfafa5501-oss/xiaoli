# 小离 · 独立 3D 动作原型

自建测试角色，**不是 Olivia Lin 官方模型，也不是小离最终形象**。目前有呼吸、眨眼、视线、聆听、思考、说话手势、问候、确认、情绪与告别，可平滑切换和打断。与仓库现有 Go 启动器、聊天功能和 v0.2 Windows 安装包独立，尚未接入真实语音或 AI。

## 只想先看效果

下载本分支后，解压并打开本目录的 **[观看动作预览.html](观看动作预览.html)**，即可播放约 24 秒实录，无需 Node.js。用 Chrome / Edge 打开该本地 HTML，保持 `output/` 相对目录完整。GitHub 的文件页面不会直接执行 HTML。

- [预览视频](output/motion-preview.webm)：WebM / VP9，无音频，实际时长 23.965 秒，2,191,723 字节。
- [角色与动画 GLB](output/companion-motion.glb)：35 骨骼、1 个躯干蒙皮、9 段动画，1,221,532 字节。GLB 不是 VRMA；导入其他引擎后仍需选择动画片段。
- [动作规格与模型替换](动作规格与接入说明.md)：时长、触发、优先级、过渡、许可及局限。
- [协作约定](COLLABORATION.md)：与 Grok 等开发者分工、接口和二进制资产规则。

## Windows 获取与运行交互版

需要 Git、Node.js 22 或更新版本及支持 WebGL2 的 Chrome / Edge。**Windows 尚未实机测试**；Mac 上用 Node.js 24.22.0 和 Chrome 验证。首次安装 npm 依赖需要联网，之后本地播放无需联网、密钥、账号、摄像头或麦克风。

在 PowerShell 中使用一个新的文件夹，避免覆盖已有开发目录：

```powershell
git clone --branch codex/3d-motion-demo-20261023 --single-branch https://github.com/tangfafa5501-oss/xiaoli.git xiaoli-motion
cd xiaoli-motion\experiments\companion-motion
npm.cmd ci --ignore-scripts
npm.cmd test
npm.cmd start
```

复制终端输出的 `MOTION_DEMO_URL=http://127.0.0.1:端口` 地址到浏览器。默认选择空闲端口，不要照搬另一台电脑的端口号。地址也记录在已忽略的本机 `runtime.json`。Ctrl-C 停止；依赖安装完成后也可双击 `start.cmd`。

这里使用 `npm.cmd`，避免 PowerShell 把 `npm` 解析为受本机脚本策略限制的 `npm.ps1`，无需修改系统执行策略。视频播放器和交互版是两个入口：`观看动作预览.html` 可直接打开，`index.html` 必须通过本地服务访问。

不使用 Git 也可以在本分支页面选择 **Code → Download ZIP**，解压后进入 `experiments/companion-motion`，再执行上述 `npm.cmd` 命令。当前 v0.2 Release 不含这套 3D 实验。

如果这台 Windows 已经有另一个“小离”工作目录，不切换它正在开发的分支；用上述新目录克隆，或参考协作说明建立独立 worktree。

## macOS / Linux

```sh
cd experiments/companion-motion
npm ci --ignore-scripts
npm test
npm start
```

macOS 也可在安装依赖后双击 `启动动作演示.command`。脚本使用 PATH 中的 Node，不依赖某个人的安装绝对路径。

## 使用与输出

“播放完整互动”运行约 42.5 秒的演示序列；动作按钮随时切换，“我想接着说”打断进入聆听。拖动旋转视角、滚轮缩放、逐帧查看会暂停，空格继续；可显示骨骼。口型仅为无声示意。

`output/` 保存经过验收的版本；浏览器导出、截图、录屏及本地回读报告写入 **`generated/`**，不会自动覆盖仓库中的二进制资产。先检查生成结果，再由本轮资产负责人更新 `output/`。

只提交源码、锁文件、必要文档、许可及原创交付资产；不提交 `node_modules/`、`runtime.json`、`generated/`、秘密或系统缓存。

## 验证范围

`npm test` 依次执行：

1. 8 项动作参数与状态测试，覆盖所有 81 对状态切换。
2. 用 GLTFLoader 回读实际 GLB，检查 35 骨骼、9 动画、270 个采样点及 idle 首尾闭合。
3. 启动独立回环 HTTP 服务，检查入口、中文路径、Three.js 模块、视频/GLB、HEAD 和本地保存。

[验收记录](output/验收记录.md) 区分原始浏览器验证和本次跨平台整理。仓库当前仅有手动 Windows Release 工作流，没有自动运行本演示的 CI；本实验不会触发或修改 Release，也未部署网站。Windows 视觉和脚本兼容性仍待 Windows 实测。

原创代码与程序生成角色按本目录 [MIT LICENSE](LICENSE) 分发。Three.js 固定为 0.180.0，其 MIT 许可保留在 [licenses/THREE-LICENSE.txt](licenses/THREE-LICENSE.txt)；第三方角色和商业专有资产未纳入交付。
