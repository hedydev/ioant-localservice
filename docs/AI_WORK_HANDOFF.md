# AI 工作交接 — ILS

> Last updated: 2026-10-02  
> AI-Agent: ChatGPT  
> AI-Session: `n6q4m8zt`

本文记录当前这轮 ILS UI / Apple Release 工作的关键实现、产品语义和下一步验证点，供下一位 AI 或开发者直接接手。可复用的架构规则仍以 `docs/WEB_UI_ARCHITECTURE.md`、`docs/RELEASE_PROFILES.md`、`docs/APP_STORE_CONNECT.md` 为准。

## 1. 已完成的重要工作

### 1.1 构建任务实时日志

构建任务日志已经从“整个任务列表底部的固定日志区域”调整为任务卡关联展示：

- 正在执行的任务，实时日志展开在对应任务卡片正下方。
- 运行中的任务默认自动展开。
- 任务完成后日志仍可查看。
- 支持“查看日志 / 收起日志”。
- 用户手动收起后，3 秒轮询不能强制重新展开。
- 原有日志自动跟随底部（follow）保留。
- 用户向上滚动查看历史日志后，follow 暂停；回到底部后恢复。
- 多任务同时存在时，日志上下文必须始终跟随各自 Build Job，不能再出现“日志属于列表底部某一个任务”的歧义。

后续如果继续修改 Build Job UI，不要重新引入全局固定日志面板。

### 1.2 TestFlight 统一为正常 Release

TestFlight 不再作为 ILS 中独立的“另一套发布系统”，而是普通 Release 的一种 delivery：

```text
Release
├─ delivery=artifact
│  ├─ Ad Hoc IPA / macOS artifact → 下载或安装
│  └─ 本地产物相关动作
└─ delivery=testflight
   └─ TestFlight → open_url
```

TestFlight Release 具有：

- version
- build
- platform
- architecture
- channel
- variant
- bundle_id
- App Icon
- status
- optional TestFlight URL

但没有本地 IPA filename / SHA256 / download URL。

因此：

- Overview / Release History / iOS Release 等共用发布区域都应把 TestFlight 与 Ad Hoc 放在同一 Release 模型中。
- Ad Hoc 继续提供 IPA 下载/设备安装动作。
- TestFlight 提供“在 TestFlight 中打开”动作。
- TestFlight 上传成功绝不能被伪造成“本地 IPA 已发布”。
- 没有 Apple 侧状态证据时，状态停在 `submitted`，不能假装“可测试”。

详细契约见 `docs/RELEASE_PROFILES.md`。

### 1.3 App Store Connect 状态同步

ILS 已加入 App Store Connect API 集成，用于在 TestFlight 上传之后继续追踪 Apple 侧状态。

解析链：

```text
Bundle ID
→ App Store Connect App
→ iOS prerelease version
→ build
→ processingState
→ Build Beta Detail
→ Beta Group / public link
```

统一 Release 状态：

| ILS 状态 | 语义 |
| --- | --- |
| `submitted` | 上传已被 App Store Connect 接受，但尚未获得匹配 build 的后续状态证据 |
| `processing` | Apple 正在处理，或 beta 状态仍处于等待处理/合规/审核阶段 |
| `available` | Apple 已提供可测试的 beta 状态 |
| `unavailable` | Apple 报告处理失败、过期、异常或 beta 不可用 |

同时保留原始 Apple 字段用于诊断。

刷新方式：

- Release 列表后台刷新，避免持续高频请求 Apple。
- `submitted/processing` 发布保持较积极的刷新。
- 已进入 `available/unavailable` 的终态降低刷新频率。
- iOS Release 页面提供管理员手动 **Refresh Release Status**。
- Build Job 会同步 Apple 侧 Release 状态/消息，但不会把“上传成功”历史改写成“上传失败”。

相关后端与 UI：

- `internal/service/app_store_connect.go`
- `web/js/app-store-connect.js`
- `docs/APP_STORE_CONNECT.md`

### 1.4 App Store Connect Key 管理

管理员可以在 iOS Release 页面配置：

- Key ID
- Team API Key 的 Issuer ID
- 本机 `.p8` 私钥路径

ILS 只保存标识和本机私钥路径，不保存私钥内容，也不把私钥内容返回浏览器、写入 Release metadata 或提交 Git。

Team API Key：

```text
ILS_ASC_KEY_ID
ILS_ASC_KEY_PATH
ILS_ASC_ISSUER_ID
```

可同时用于 ILS 状态查询和 xcodebuild TestFlight 上传。

Individual API Key：

- 可用于 ILS App Store Connect 状态查询；
- 不注入 xcodebuild TestFlight 上传，因为 xcodebuild 分发认证需要 issuer；
- 上传继续使用已登录的 Xcode Apple 账号。

不要把 `.p8` 文件复制进项目仓库。

### 1.5 TestFlight 链接

Beta Group 暴露 enabled public link 时：

- API 查询得到的 public link 优先；
- Release Profile 中的 `testflight_url` 作为 fallback；
- TestFlight Release 在统一 Release UI 中显示“在 TestFlight 中打开”。

TestFlight 链接只属于 `ios-testflight` delivery，不应出现在 Ad Hoc 或普通 macOS artifact Release 上。

### 1.6 App Icon / 平台图标

ILS 已采用“真实 App Icon + 平台标识”并存的方向：

- 项目自己的 App Icon 是 Release/项目数据的一部分，不再只显示 Mac/iPhone/iPad 线框平台图标。
- iOS 与 macOS 可以使用不同真实 App Icon。
- 优先从构建产物（IPA / .app）提取真实图标，并按 artifact/platform 保存/展示。
- 没有可提取产物时，可使用项目平台图标缓存/元数据。
- Release Profile、Build Job、最新发布、Release History、iOS Release 等适合的位置可以显示真实 App Icon。
- Mac / iPhone / iPad 等平台 icon 仍保留，用于表达支持平台；真实 App Icon 负责表达“这是哪个 App”。

不能把 Sowhat 的图片路径或图片文件硬编码进 ILS。

### 1.7 平台扩展方向

平台图标/平台识别已经按可扩展方向设计：

```text
platform
├─ macOS
├─ iPhone
├─ iPad
└─ future: Android / Android Tablet / ...
```

后续新增 Android 时，应扩展统一 platform metadata / icon mapping，而不是在各页面单独写死判断。

### 1.8 统一 Release 展示与 ASC 状态体验（2026-10-02）

本次继续在上述模型上做收敛，不改变 Build Job 的 3 秒轮询规则：

- 新增 `web/js/release-ui.js`，Overview / Release History / iOS Release 统一使用同一个 Release card、动作和 TestFlight lifecycle renderer，不再由 `releases.js` 与 `ios.js` 各维护一套状态文案。
- TestFlight Release 明确显示 `submitted → Apple Processing → available` 三段生命周期；Build Job 结果区复用同一状态文案与 lifecycle。
- App Store Connect 管理区显示 Team / Individual Key、连接状态、最近检查、TestFlight 上传认证路径；保存且验证成功后立即同步一次 Release 状态，页面可见时每 10 秒只回读本地配置状态。
- TestFlight Apple Public Link 与 Release Profile fallback 现在分开保存。Apple 关闭 Public Link 后，下一次成功同步会清除陈旧 public link 并回退到 Profile URL。
- App Icon 使用统一 identity shell，并叠加平台徽标；真实 artwork 缺失或加载失败时保留平台 glyph fallback，不再留下空白图标位。
- App Icon 扫描支持 Git-tracked `Contents.json` 引用的本地生成/ignored PNG；这是 Sowhat 当前 AppIcon 生成方式。读取范围仍限制在对应 `.appiconset`，并拒绝 traversal/symlink/非 PNG/超大文件。失败的前端 `<img>` 会直接移除，避免破图标记。
- 本轮没有改变 `build-jobs.js` 的 3 秒任务/日志轮询、用户上滚暂停 follow、手动收起日志等行为。
- Build Job 日志布局进一步固定为“任务详情 → 查看/收起日志 → 此任务日志面板”；按钮和日志都位于对应任务卡内部，不允许出现列表底部的共享日志区域。
- App Store Connect 同步新增 **Build Upload** 层：按 Bundle ID / marketing version / build number 查询 `buildUploads`，ILS 主状态徽标直接显示 `Build Upload Processing / Complete / Failed`；`COMPLETE` 后才继续解析 Build/Beta Detail。
- TestFlight Release Profile 可配置目标 Group、Internal/External、自动创建 Group、以及显式的 External Beta Review 自动提交。Group 自动化只在正式 Build `processingState=VALID` 后执行；自动化错误独立记录，不会覆盖上传成功历史。
- 持久低频配置统一采用“页面状态摘要 + 配置/编辑弹框”：当前已把项目来源（目录/发布分支）和 App Store Connect Key 配置从长期展开表单改为 dialog；页面保留刷新/验证等高频动作。状态轮询不得覆盖正在编辑的弹框字段。

### 1.9 TestFlight Release 自动回填（2026-10-02）

修复了“Build Job 显示 TestFlight 上传成功，但 iOS Release 列表为空”的数据链缺口。

- ILS 启动时会扫描持久化 Build Job；只有完整保存了可信 `upload-succeeded` TestFlight result 的任务才可恢复。
- 缺失的 `delivery=testflight` Release 会自动重建，并把 Release ID 回写到对应 Job 的 `release_ids`。
- App Store Connect 手动刷新在请求 Apple 前也执行同一 reconciliation，因此无需手工伪造 Release。
- 新 Build Job 保存必要的 Release Profile 快照，以便 Profile 后续修改/删除后仍能恢复当时发布语义。
- TestFlight 去重按 Apple app/build 身份收敛，避免相同 Bundle ID + version + build 因 variant/channel 元数据差异产生重复卡片。
- Build Job UI 如果真的还找不到关联 Release，会明确显示“ILS Release 待同步”，不再把 fallback result card 伪装成已经存在的 Release。
- 该修复不把上传成功解释为可测试；恢复后的 Release 仍从 `submitted` 开始，后续状态必须由 App Store Connect API 证据推进。

当前实现仍需在用户 Mac 上执行 `go test ./...`、重启 ILS，并确认历史 Sowhat build 224/228 自动出现在 iOS Release 列表。

## 2. 当前产品语义必须保持

最重要的一条：

> **xcodebuild exportArchive + destination=upload 成功 = App Store Connect 已接受上传，不等于 TestFlight 已可安装。**

正确生命周期：

```text
上传中
  ↓
已提交到 App Store Connect
  ↓
Apple Processing
  ↓
TestFlight 可测试
```

ILS 只能根据 Apple API 实际返回状态推进，不能根据上传命令成功、等待时间或本地猜测推进到 `available`。

## 3. 当前代码结构

本轮重要模块：

```text
internal/service/
├─ app_store_connect.go
├─ builds.go
├─ service.go
└─ *_test.go

web/
├─ app.js
└─ js/
   ├─ ios.js
   ├─ release-ui.js
   ├─ releases.js
   ├─ build-jobs.js
   └─ app-store-connect.js
```

App Store Connect UI 已从 iOS 主模块中拆出，避免后续 Apple API / TestFlight 控制继续膨胀 `ios.js`。

## 4. 当前验证状态

最近一次主分支实现提交：

```text
0ffef75e96ea9868ace0d2413de3844b3d0b2882
Reconcile missing TestFlight releases
```

提交在既有 TestFlight / App Store Connect 集成上新增：

- 从持久化 `upload-succeeded` Build Job 自动回填缺失的 TestFlight Release；
- 重建并持久化 Job `release_ids` 关联；
- 服务启动和手动 App Store Connect 刷新都执行 reconciliation；
- 新 Build Job 保存恢复所需的 Release Profile 元数据快照；
- TestFlight 去重按 Bundle ID + version + build 的 Apple build 身份收敛；
- Build Job 在缺失 Release 时明确显示“ILS Release 待同步”，不再用 fallback card 掩盖数据缺口；
- 新增历史任务回填、Profile snapshot fallback、拒绝不可信上传结果和 Apple build 去重测试。

**本次改动已做 GitHub 侧静态结构检查，但没有在用户 Mac 上实际执行 `go test ./...`、`go build` 或重启后的真实 state.json / Build Job 回填验证；这些不能记录为通过。**

因此下一位接手者首先应该在本机从当前 `main` 开始：

```bash
git pull --ff-only
go test ./...
go build -o bin/localservice ./cmd/localservice
```

然后重启 ILS，进行 UI + TestFlight 状态同步实测。

## 5. 下一步建议验证顺序

1. 更新本地 ILS 到当前 `main`。
2. `go test ./...`。
3. 构建并重启 ILS。
4. 检查 Build Jobs：运行任务日志是否出现在对应任务卡正下方，手动收起后轮询是否保持收起。
5. 检查 Overview / Release History / iOS Release：TestFlight 是否与 Ad Hoc 一起作为普通 Release。
6. 配置 App Store Connect API Key。
7. 对一个真实 TestFlight Build 执行上传。
8. 确认任务先进入 **submitted**，而不是直接显示 TestFlight 可测试。
9. 手动刷新/等待后台刷新，确认 Apple Processing 状态真实推进。
10. Apple 暴露 beta-ready 状态后确认 ILS 变为 **available / TestFlight 可测试**。
11. 如果 Beta Group 有 public link，确认“在 TestFlight 中打开”只出现在 TestFlight Release。
12. 检查 iOS/macOS App Icon 与平台 icon 是否正确显示。

## 6. Git / AI 工作约定

后续 AI 修改 ILS 或其他项目时：

- 从当前 `main` HEAD 开始，不擅自 reset/clean/stash/switch/merge。
- 修改代码后同步更新对应 docs。
- commit message 明确写出 AI-Agent 身份和本次工作 session。
- 不提交 `.localservice/`、admin token、`.p8`、证书、私钥、provisioning profile 或运行时产物。
- 不把 Apple 私钥内容输出到聊天、日志或 Git。
- 不把 TestFlight 上传成功解释为 TestFlight 已可安装。
