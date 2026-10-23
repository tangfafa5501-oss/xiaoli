# 小离动作协作约定

本分支是独立演示交付，不改变现有 `launcher/`。正式接入小离应另开集成分支，由用户或维护者审阅后合并。本轮不联系 Grok、不合并主分支。

## 分支与工作目录

- 每位开发者一个工作目录、一个功能分支；例如 `codex/motion-…` 和 `grok/dialogue-…`。不要两位代理共同编辑同一工作目录或向同一分支推送。
- 以主分支为起点开发普通功能；需要本原型时以本交付分支为起点，待维护者合并后统一基线。
- 已有未提交修改的目录保持原状，不用 `reset --hard`、不 force push。提交前执行 `git status`，只暂存自己负责的文件。
- 拉取别人的更新时先保存自己的工作，再 fetch 并在自己的分支解决冲突。用 PR 展示差异；未核对的 PR 保持 draft。

例如在已有仓库旁建立新的 Windows 审阅目录（分支名需未被占用）：

```powershell
git fetch origin
git worktree add ..\xiaoli-motion-review -b review/companion-motion origin/codex/3d-motion-demo-20261023
```

## 最小动作接口

现有接口位于 `motion.mjs`，保持名称和单位稳定：

```js
import { MotionEngine, microPose } from './motion.mjs';
const motion = new MotionEngine();
motion.request('listen', { manual: true }); // 用户接话：明确打断
// 每帧 dt 单位秒；实际播放器还有暂停、速度和可见性控制。
const pose = motion.update(dt);
rig.apply(pose, microPose(motion.time));
```

动作 ID：`idle/listen/think/speak/wave/happy/confused/nod/bye`。`request(id)` 返回是否接受；普通请求服从优先级、同状态不会重启，`manual:true` 是显式覆盖。

姿态字段：`head/chest/leftPalm/rightPalm` 是弧度数组；`left/right` 是以米计的腕部目标，实际坐标约定以 `rig.mjs` 为准；`gaze` 是限幅偏移；`smile/brow/asym/curl/mouth/blink` 是标量。单位米、Y 向上、Z 向前，原型根固定。不得把这些腕坐标直接套到不同比例的正式模型。

接入建议：用户讲话 → listen；组织回复 → think；TTS 播放 → speak；TTS 结束 → idle；会话开始/结束 → wave/bye。**这些是未来事件映射，当前没有真实 VAD/TTS/AI 接口。** 接入方需要用会话/话轮标识忽略过期异步回调，避免旧的“语音结束”事件打断新的聆听。

表情与口型必须统一分配通道：情绪不得写嘴部开合，眨眼闭合优先于眯眼，真实 AEIOU 口型接入时替换示意 `mouth`。修改接口先更新本文件和状态测试，不同时让两个模块直接写同一关节。

## 文件职责与二进制资产

| 范围 | 建议职责 |
| --- | --- |
| `motion.mjs`、`tests.mjs` | 动作调度/参数负责人 |
| `rig.mjs`、`output/*.glb` | 角色骨骼/重定向负责人 |
| `app.mjs`、HTML/CSS | 演示界面负责人 |
| 小离 `launcher/` 与对话事件 | 集成负责人，在另一个分支处理 |

每一轮仅指定一人更新 `output/` 中 GLB、视频、PNG。二进制文件不能像代码一样逐行合并：先约定负责人；其他人将候选放入本地 `generated/`，不要并行覆盖同名基准资产。切换负责人时记录源代码 commit、Three.js 版本、动作规格修改和测试结果。

资产当前仅约 3.5 MB，可普通 Git 管理；不要为本次交付改写历史或全仓引入 Git LFS。若以后高精模型增大，再单独讨论 LFS 策略。正式角色素材必须逐项确认许可，不能从 AIRI 代码 MIT 推导所有模型都允许分发。

每个改动的审阅说明至少写：修改文件、接口影响、测试结果、是否更新二进制资产、Windows 是否实际测试。只准备分支/PR 不等于已集成到发布版。
