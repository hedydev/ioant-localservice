# AI 工作交接 — ILS

> Last updated: 2026-10-02  
> AI-Agent: ChatGPT  
> AI-Session: `ils-handoff-review-2026-10-02`

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
→ Build Upload
→ Build Upload.state
→ iOS prerelease version
→ Build
→ Build.processingState
→ Build Beta Detail
→ Beta Group / public link
```

Build Upload 是早期 Apple Processing 的第一证据层；普通 Build resource 尚未出现时，ILS 也必须能与 App Store Connect 的 Build Uploads 状态保持一致。

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

### 1.10 TestFlight 回填诊断与操作反馈（2026-10-02）

用户实测发现历史 Sowhat TestFlight build 224/228 在第一次 reconciliation 修复后仍显示 **ILS Release 待同步**，同时“验证连接 / 刷新发布状态”缺少明显的执行结果反馈。已在 `6ab460af2ca2d015905bd98dbd3daf81434361f1` 继续修复：

- App Store Connect **验证连接**和**刷新发布状态**统一使用固定顶部通知；执行中、成功、警告、失败都有明确状态，并自动消失。
- Refresh 完成后同时重新读取统一 Release catalog 和 Build Job 列表，避免后端已经修复但页面仍显示旧 `release_ids`。
- 后端返回 TestFlight reconciliation 报告：TestFlight 任务扫描数、可恢复数、新关联数、已有链接数、运行中数，以及不包含敏感数据的跳过原因计数。
- 旧 Build Job 顶层 `lane` 为空、旧持久化结果缺 `schema_version` 时不再直接排除；仍要求 result 自身明确为 `ios-testflight` 且 Bundle ID/version/build/distribution 等身份完整。
- 旧 result 如果还没有 `submission_result` 字段，只有在保存的 `build.log` 中存在结构化 `ILS_EVENT {stage:"upload",state:"succeeded"}` 时才允许回填。普通文本、猜测或等待时间不能作为上传成功证据。
- 前端刷新通知会直接显示本地 reconciliation 结果和 Apple 状态更新数量；如果仍不能回填，会显示例如“缺少 upload-succeeded 证据”等原因，而不是只显示“待同步”。

这些改动尚未在用户 Mac 上重新执行 `go test ./...` / `go build`，也尚未确认真实 224/228 是否已完成回填，不能写成通过。

### 1.11 交接代码复核与加固（2026-10-02）

接手 `385c0880f48d61f6f542b586aacbeb8bad15964f` 后，对 `0ffef75e...` / `943a302...` / `6ab460a...` 的实现和 docs 做了静态复核。整体方向保留，但修正了三个边界问题：

- 历史 TestFlight recovery 不再因为整个 Xcode `build.log` 超过 2 MiB 就放弃。ILS 现在只读取日志尾部的有界窗口并寻找严格结构化的 `ILS_EVENT {stage:"upload",state:"succeeded"}`；仍不接受自由文本作为上传成功证据。
- `publishTestFlightRelease()` 的 App Icon snapshot 移到去重确认之后，重复 reconciliation / retry 不再为随机临时 Release ID 写入孤儿 `release-icons/*.png`。
- 历史 reconciliation 仍会返回 skip/error 诊断，但单个旧 Job 的读取/关联问题不再阻断 ILS 启动，也不再阻断同一次 App Store Connect Apple 状态刷新；前端会显示 reconciliation warning。
- 新增回归测试覆盖大日志尾部 upload-success、TestFlight Release 去重 icon 泄漏、以及历史 Job 读取失败不应阻止 `New()` 启动。

核心加固提交：`ca40a14a13a9015e01e3612bf854a8b4799d6077`。

这些新增测试仍需在用户 Mac 上真实执行 `go test ./...` 和 `go build` 后才能记录为通过。

### 1.12 Build Upload state 对象解析修复（2026-10-02）

用户真实刷新 App Store Connect 后确认历史 Release reconciliation 已成功（2 个任务均可恢复/已关联），但 Apple 状态同步报 `App Store Connect build upload attributes 无效`。根因是 ILS 把当前 App Store Connect API 的 `BuildUpload.attributes.state` 错误定义成字符串；Apple 当前返回的是包含嵌套 `state` 以及 `errors / warnings / infos` 的对象。

修复内容：

- 新增兼容 Build Upload state 类型，正确解析当前对象格式；
- 同时兼容旧的纯字符串 state，避免历史/API 兼容回退；
- Release metadata 保存 Build Upload state 以及 error/warning/info 数量；
- Build Upload 状态消息会附带诊断数量；
- Release 详情显示这些计数；
- 新增 nested PROCESSING / FAILED、legacy string COMPLETE、缺少 nested state 的非法响应，以及诊断数量格式测试。

当前仍需用户 Mac 真实执行 `go test ./...` / `go build`，然后点击“刷新发布状态”确认 Sowhat 228 显示 `Build Upload Processing`，224 不再出现 attributes 解析错误并继续进入 Build/Beta Detail 状态链。

### 1.13 与 App Store Connect TestFlight 状态展示对齐（2026-10-02）

用户实测确认 Sowhat build 224 / 228 均已同步为：Build Upload `COMPLETE`、Internal `READY_FOR_BETA_TESTING`、External `READY_FOR_BETA_SUBMISSION`；App Store Connect 的 build 列表主状态显示 **Ready to Submit**。

本轮进一步收敛：

- Release 和 Build Job 不再用一个线性的“已提交 → Apple Processing → 可测试”控件合并 Internal/External 状态；
- 统一显示 **Build Upload / Internal Testing / External Testing** 三个独立状态块；
- Apple enum 映射为 App Store Connect 风格文案，例如 `READY_FOR_BETA_TESTING → Ready for Testing`、`READY_FOR_BETA_SUBMISSION → Ready to Submit`、`WAITING_FOR_BETA_REVIEW → Waiting for Review`；
- 当 Internal 已 Ready for Testing、External 仍 Ready to Submit 时，顶部主徽标显示 **Ready to Submit**，Internal 状态块仍明确显示 **Ready for Testing**；
- `Ready to Submit` 使用注意态而不是“已可测试”的绿色终态；
- Build Job 上传后的状态区复用同一套三块状态；
- Internal 已 ready 但 External 仍待提交/审核/合规时，Release 继续保持积极 Apple 刷新，不因为整体 `status=available` 就提前进入 15 分钟终态刷新节奏。

仍需在用户 Mac 上执行 `go test ./...` / `go build` 并刷新真实 224/228 页面确认布局与状态一致。

### 1.14 Build Job 稳定刷新、TestFlight availability 与 Overview 修正（2026-10-02）

根据用户真实页面继续修正四点：

- Build Job 3 秒轮询改为按 Job ID 增量更新，不再整块替换 `#build-jobs`；展开日志 DOM 保持挂载，日志无变化时不重写文本，有新增日志时追加 tail，避免每 3 秒闪烁并保持滚动位置。
- Release-level `available` 不再由 Internal Testing 的 `READY_FOR_BETA_TESTING` 单独触发。External 仍 `READY_FOR_BETA_SUBMISSION / WAITING_FOR_BETA_REVIEW / IN_BETA_REVIEW / ...` 时整体仍为 `processing`；Internal 状态只作为独立子状态展示。只有 External Testing 进入可测试状态，整体才进入 `available`。
- App Icon 上不再叠加重复的平台徽标；平台 icon 保留在标题旁。图标加载失败时仍可使用 icon shell 内的平台 fallback。
- Overview 原来只显示 `state.releases[0]`，并不是“只显示 available”。现改为分别显示最新 iOS Release 与最新 macOS Release，不按 availability 过滤，因此 iOS `Ready to Submit` 也会出现在概览。

仍需用户 Mac 执行 `go test ./...` / `go build` 并重启 ILS 后验证：日志不闪、224/228 不再被整体标为 available、App Icon 不再重复平台徽标、Overview 同时显示 iOS/macOS 最新 Release。

### 1.15 Ad Hoc OTA 公网 HTTPS Gateway 部署脚本（2026-10-02）

新增 `scripts/deploy-adhoc-ota-gateway.sh` 与 `docs/ADHOC_OTA_GATEWAY.md`，用于准备现有新加坡 EC2 上的独立 Nginx/Let's Encrypt OTA 站点。

当前默认目标来自既有开发环境：

- EC2 `52.77.167.119`；
- SSH user `ubuntu`；
- 本机 SSH 私钥 `/Users/ted/Documents/workspace/aws-sigapore-v2ray.pem`；
- EC2 静态根目录 `/srv/ils-adhoc-ota`。

注意：workspace 下的 `.pem` 是 SSH 私钥，不是 HTTPS 证书。公网 TLS 证书由 EC2 上的 Certbot + Let's Encrypt 管理。

脚本支持两阶段部署：

1. `--prepare-only`：仅创建独立 Nginx HTTP vhost/root，不申请证书，适合 DNS 尚未绑定时；
2. DNS A 记录指向 EC2 后，再带 `--email` 运行：校验 DNS、通过 Certbot Nginx plugin 申请/续用证书、启用 HTTPS redirect，并验证 `/_ils/health`。

脚本不会修改现有 `ioant.com` / V2Ray 站点逻辑；会在覆盖自己的 OTA site config 前备份，并在 reload 前执行 `nginx -t`。本轮只完成 gateway provisioning，尚未把 ILS Ad Hoc Release 的 IPA/manifest 自动同步到 EC2，也不会改写本地 ILS `-public-url`。下一步应在域名确定并真实部署验证后，再设计独立的 artifact publish/sync 步骤。

### 1.16 Ad Hoc OTA prepare-only 实机参数修复（2026-10-02）

用户首次真实执行：

```sh
./scripts/deploy-adhoc-ota-gateway.sh --domain ota.ioant.com --prepare-only
```

SSH 已成功连接 EC2，但远端脚本因 `set -u` 直接读取缺失的尾部空参数 `$5` 而失败：

```text
bash: line 7: $5: unbound variable
```

根因是 prepare-only 模式下 email 为空，SSH 远端命令重组不能依赖“尾部空字符串参数”被保留。

修复为：

- 远端只要求前 4 个参数；
- email 使用 `${5:-}` 安全缺省；
- 远端显式校验 `PREPARE_ONLY=0|1`；
- 只有非 prepare-only 模式才要求 email；
- 不需要清理 EC2，可直接拉取修复后重新执行原 prepare-only 命令。

该修复仍需用户重新运行真实 EC2 部署确认成功。

### 1.17 OTA HTTPS 首页 404 修复（2026-10-02）

用户完成 DNS + Certbot/Let's Encrypt 后，真实结果为：

- `https://ota.ioant.com/_ils/health` 返回 `{"ok":true,"service":"ils-adhoc-ota"}`；
- TLS/证书/Nginx vhost 已正常；
- 但访问 `https://ota.ioant.com/` 返回 Nginx 404。

根因是生成的 vhost 只使用 `try_files $uri =404`，未显式为根路径映射 `index.html`；部署脚本此前也只验证 health endpoint，因此没有捕获首页失败。

修复为：

- Nginx server 显式设置 `index index.html`；
- 增加精确 `location = /`，用 `try_files /index.html =404` 稳定返回 OTA 首页；
- TLS 部署完成后同时验证 `/_ils/health` 和 `/` 都必须返回 2xx；
- 已存在的 Let's Encrypt 证书无需删除，重新运行正常部署命令即可复用/保持证书并刷新 vhost。

该首页修复仍需用户重新运行真实 EC2 部署确认。







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
ca40a14a13a9015e01e3612bf854a8b4799d6077
Harden TestFlight reconciliation recovery
```

当前实现包含此前 reconciliation/backfill、macOS canonical-path 修复，以及本次交接复核后的大日志/去重/启动容错加固：

- 从持久化 `upload-succeeded` Build Job 自动回填缺失的 TestFlight Release；
- 重建并持久化 Job `release_ids` 关联；
- 服务启动和手动 App Store Connect 刷新都执行 reconciliation；
- 新 Build Job 保存恢复所需的 Release Profile 元数据快照；
- TestFlight 去重按 Bundle ID + version + build 的 Apple build 身份收敛；
- Build Job 在缺失 Release 时明确显示“ILS Release 待同步”，不再用 fallback card 掩盖数据缺口；
- 新增历史任务回填、Profile snapshot fallback、拒绝不可信上传结果和 Apple build 去重测试；
- 大型 Xcode 日志尾部的结构化 upload-success recovery；
- 重复 reconciliation 不产生孤儿 Release Icon；
- 单个历史 Job recovery 错误不阻断服务启动/Apple 刷新。

**已完成 GitHub 侧静态代码/文档复核。此前用户在 macOS 运行 `go test ./...` 时发现的 `/var → /private/var` canonical path 问题已由 `943a302642696b2c40635708d3dba83dec4f455d` 修复；本次 `ca40a14...` 又新增了三项 recovery 回归测试。当前 HEAD 的完整 `go test ./...` / `go build` 仍未在用户 Mac 上复跑，不能记录为通过。**

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


## 7. 当前接手点与验证（2026-10-02）

ILS 已重新由当前会话接手；不要再按“返回 Sowhat 后暂停”的旧状态理解此文档。

当前代码实现点：

- `ca40a14a13a9015e01e3612bf854a8b4799d6077` — 本次交接复核加固：大日志尾部结构化证据、去重 icon 泄漏修复、reconciliation 启动/刷新容错。
- `6ab460af2ca2d015905bd98dbd3daf81434361f1` — TestFlight 历史 Release 回填诊断、旧 Job 结构化上传成功证据兼容、刷新后同步重载 Release/Build Job、固定顶部操作通知。
- `943a302642696b2c40635708d3dba83dec4f455d` — 修复 macOS `/var -> /private/var` canonical path 导致的 App Icon/ASC 测试问题。
- `0ffef75e96ea9868ace0d2413de3844b3d0b2882` — 初始 TestFlight Release reconciliation/backfill。

当前尚未验证、接手者必须真实执行：

1. `git pull --ff-only`
2. `go test ./...`
3. `go build -o bin/localservice ./cmd/localservice`
4. 重启 ILS。
5. 在 iOS 发布页点击“刷新发布状态”，确认顶部通知能给出 `scanned / eligible / reconciled / skipped` 结果。
6. 确认历史 Sowhat TestFlight build `0.1.0 (224)`、`0.1.0 (228)` 是否自动出现在统一 iOS Release 列表并回写 Build Job `release_ids`。
7. 如果仍未回填，以顶部通知的 skip reason 和对应 Build Job 的结构化 `ILS_EVENT` 日志为第一证据继续定位；不要根据自由文本或等待时间猜测上传成功。
8. 继续确认 Apple Processing / TestFlight available 状态只由 App Store Connect API 真实证据推进。

在上述本机测试、重启和真实 224/228 回填验证完成前，不得把本轮 ILS 修复记录为“已验证通过”。
