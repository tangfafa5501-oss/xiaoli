# 小离聊天与 3D 动作接入

分支：`codex/chat-motion-integration-20261023`。以已验收原型提交 `db04ece870c188d92850facf146b5748048735cc` 为基线，保留 main、v0.2 和另一个 v0.3 开发分支。此处是原有小离聊天界面的集成，不是另一个聊天产品。

## 本轮行为

| 实际聊天事件 | 动作与界面 |
| --- | --- |
| 页面打开 | 原创测试人偶自然待机；本地大脑未就绪时仍按原逻辑等待 |
| 用户发送文字 | 原来的 `/api/chat` 请求发出，立即进入思考 |
| 首个可显示文本片段到达 | 显示流式文本并进入表达手势；隐藏 `<think>` 文本不触发表达 |
| `[DONE]` 到达 | 表达至少可见 600 ms 后回到待机，释放发送按钮 |
| HTTP/网络/空回复/截断/超时错误 | 回待机、显示可重试提示，失败话轮不写入模型上下文 |
| 点击“停止” | AbortController 取消同一真实请求，立即回待机，可接着发新问题 |
| 忙碌时重复发送 | 忽略，不创建并行请求 |
| 切到其他标签页、pagehide | 取消当前回复；回来后可再次发送，旧话轮不能覆盖新话轮 |
| 3D 资源加载/WebGL 失败 | 回到原静态背景，文字聊天仍可继续 |

这是文字驱动的手势，不是语音、TTS 或口型同步；渲染层每帧强制 `mouth=0`。测试人偶不代表最终小离形象，也不是 Olivia Lin 官方模型。

正常产品仍只使用原来的本地 llama-server。对话不写持久存储，不引入外部 AI、CDN、遥测、摄像头或麦克风。原先“关闭所有窗口后自动退出”的后台机制未改；离开程序页面超过原有退出等待时间后，仍需重新启动小离。

## 实现边界

- `launcher/web/chat-session.mjs`：请求、流解析、取消、超时、话轮身份和动作生命周期；独立于 WebGL。
- `launcher/web/chat-ui.mjs`：从现有页面提取的真实界面逻辑；保留 `/api/status`、`/api/alive`、`/api/chat`，按生命周期驱动可选 3D。
- `launcher/web/avatar.mjs`：复用测试人偶和动作，长时间思考保持姿态，表达随文本生命周期结束，口部动画关闭。
- `launcher/web/motion/`：可直接被现有 `//go:embed web/*` 收入的离线资源；仅必要的 Three.js 构建文件、原型模块、许可证和校验清单。
- `launcher/scripts/build-motion.mjs`：从 `experiments/companion-motion` 和固定 npm Three.js 0.180.0 重建离线资源。不要手动改生成的 `web/motion`；修改源动作后重建。
- `launcher/tests/`：纯测试代码，不在 `web/` 中，不嵌入产品。测试 SSE 服务器不是真实大脑，只服务真实产品 HTML 并明确显示“模拟接口”。

## 构建与运行

已经提交可嵌入的离线网页资源，普通 Go 构建无需 npm。需要重新生成动作资源时：

```sh
cd launcher
npm ci --ignore-scripts
npm run build:web
npm test
npm run test:http
```

Windows 获取代码（PowerShell；本次未在 Windows 实机执行）：

```powershell
git clone --branch codex/chat-motion-integration-20261023 --single-branch https://github.com/tangfafa5501-oss/xiaoli.git xiaoli-chat-motion
cd xiaoli-chat-motion\launcher
go test ./...
go build -trimpath -o ..\xiaoli-motion.exe .
```

这会生成本分支的新启动器；v0.2 下载包不会自动变成 3D 版。将自己已有、来源可信且许可允许的 `runtime` 文件夹放到 `xiaoli-motion.exe` 旁，其中包括 Windows 版 `llama-server.exe`、其所需 DLL、`model.gguf` 和现有配置，再运行 `xiaoli-motion.exe`。不要把 macOS 的 llama-server 复制到 Windows，也不要覆盖原 v0.2 目录。首次先用带控制台的上述构建检查运行情况。

真实小离的 Go 构建沿用根目录 `BUILD.md`，需要符合 `launcher/go.mod` 的 Go 工具链（当前声明 `go 1.27.2`）。编译出的启动器旁仍必须有原先的 `runtime/llama-server[.exe]` 和 `runtime/model.gguf`；本分支不下载或附带模型。不要用 `go run .` 的临时可执行文件目录代替正确的运行时打包布局。

在具备 Go 的环境中，先在 `launcher/` 执行 `go test ./...`，再按对应平台构建。新增 `web_assets_test.go` 用于验证真实 Go 嵌入模块的 MIME/可访问性，以及请求取消向本地代理上游传播；这两项测试在本次 Mac 上尚未执行。

浏览器验收夹具（不调用模型）：

```sh
cd launcher
npm run test:browser
```

终端输出 `FIXTURE_UI` 是实际小离界面加模拟 SSE；`BROWSER_TESTS` 是浏览器检查入口。检查覆盖正常状态顺序、停止、重复发送、错误、页面生命周期和 3D 加载失败。测试记录写入被忽略的 `launcher/.test-output/`。此入口只用于验证，不是新的产品启动方式。

## 当前证据等级

| 检查 | 当前结果 |
| --- | --- |
| Node.js 24.22.0 单元/资源测试 | 16/16 通过；包括取消后旧结果、UTF-8 分片、错误与恢复 |
| 离线网页资源构建 | `npm run build:web` 通过；6 个资源有 SHA-256 清单，无运行时 CDN |
| JS 语法检查 | 通过 |
| HTTP 夹具冒烟测试 | 通过：9 个页面/模块路径返回 200，JavaScript MIME 正确，模拟 SSE 完成与 HTTP 500 可读；未执行浏览器渲染或 Go 代理 |
| Go 单元测试与可执行文件构建 | 未执行：本机没有 Go 工具链，`go version` 返回 command not found |
| 实际浏览器交互与新截图 | 未执行：Mac 锁屏，浏览器工具无法自动解锁；等待手动解锁 |
| 真实 llama-server/model.gguf 对话 | 未执行：该运行时与模型不在本机任务环境 |
| Windows 实机 | 未执行，也未获得本轮操作 Windows 的授权 |

因此这份分支是待浏览器/Go/真实模型验收的集成实现，**不能把单元测试或模拟 SSE 当作真实对话验收通过**。原型目录内的旧视频只证明独立动作原型，不是本次聊天集成的演示证据。

## 协作与后续验收

继续使用独立分支，保留 `experiments/companion-motion` 基准；不并行覆盖二进制资产。对话负责人只通过 `ChatSession` 生命周期驱动动作，不绕过它让多个请求直接写骨骼。3D 回调异常必须保持文字请求可继续。

下一步先解锁 Mac 完成浏览器检查，再在具备所需 Go 与已授权本地模型的环境验证真实代理和可执行文件。Windows 需要单独实机验收后才能声称能实际运行。本轮不合并 main、不发布 Release、不部署、不接语音。
