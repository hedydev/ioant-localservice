# 本地目录 → 多脚本 → 构建发布

管理页中选择项目，进入“发布管理”，在“从项目目录发布”填写 Mac 上的本地目录与发布分支（默认 main）。服务自动解析 Git 根目录、当前分支、upstream、远程地址及工作区状态。路径保存在本地受限配置，不会向普通下载用户公开。

可以点击“选择文件夹…”打开服务 Mac 的原生目录选择窗口，选择后自动填入 Git 根目录，再点击“关联目录”。首次使用 macOS 可能提示控制 Finder 的自动化权限；取消不会修改原路径。从 iPhone 或其他电脑访问时，选择窗口仍位于服务 Mac 上。服务没有图形桌面会话时可手动输入路径。此功能不会上传文件夹内容。

## 脚本发现约定

只发现项目根目录或 `scripts/` 下已纳入 Git 的 Bash 脚本：

```text
scripts/release-ios.sh       scripts/release-ios.md
scripts/release-macos.sh     scripts/release-macos.md
scripts/release-all.sh       scripts/release-all.md
release-helper.sh           release-helper.md
```

文件名匹配 `release*.sh`（release 后仅字母、数字、下划线、连字符）；不递归扫描依赖或构建目录。脚本、说明文件和路径中的目录不能是符号链接。使用 `/bin/bash` 执行，不要求 chmod +x。脚本上限 1 MiB，Markdown 上限 64 KiB。必须有已纳入 Git、非空的同名 `.md` 才能点击发布。

Markdown 的第一个一级标题作为发布入口名称。页面支持安全的标题、列表、段落、代码块、行内代码和加粗；原始 HTML 和链接只作文本，不执行代码或加载外部内容。

同名 Markdown 建议包含：

```markdown
# iOS 开发测试包（Ad Hoc）

用于向团队登记设备发布完整原生 IPA。

## 目标与产物
- 平台：iOS / arm64
- 产物：Sowhat.ipa，variant=default
- 渠道：dev

## 前置条件
- Xcode 已登录付费开发者账号，证书可用。
- 目标 UDID 已登记并包含在 Ad Hoc 描述文件中。

## 执行内容
1. 确定版本及新的构建号。
2. archive，然后使用 release-testing 导出。
3. 调用 Localservice push.sh 上传 IPA。

## 副作用与失败处理
- 只构建、打包和上传，不安装、启动 App 或改写源码。
- 构建日志和中间文件保留在 LOCALSERVICE_OUTPUT_DIR。
- 任一步失败以非零退出；不继续上传旧产物。
```

这些说明帮助用户选择脚本，不是静态认证。页面“已识别脚本与说明”不代表构建、签名或安装已验证。

## 点击“拉取并发布”

1. 检查当前分支匹配配置、工作区无修改/未跟踪文件、upstream 已配置。
2. 在**用户绑定的原目录**执行 `git pull --ff-only`。禁用 Git hooks 和 autostash，不自动切分支、stash、reset、merge 解冲突。
3. 再检查工作区、HEAD 与 upstream 一致，以及选中的脚本和说明仍有效。
4. 记录实际 commit，在相同目录执行 `/bin/bash <script>`。
5. 记录脚本通过 API 发布的全部安装包。脚本退出 0 且至少收到一条关联发布记录时显示成功；多产物是否完整仍由脚本保证，不能上传一个文件就声称所有产物完成。

构建过程中不要让另一个开发 AI 同时修改这个目录；服务只能串行自己发起的任务，不能锁住外部编辑器。拉取后脚本可能随远程分支更新，绑定的仓库必须是管理员信任的项目。脚本具有服务进程所在用户的本地执行权限。

目前服务一次运行一个构建任务；pull 超时 3 分钟、整任务 60 分钟。退出服务时终止当前构建进程组。构建日志最大 2 MiB，管理员页面每 3 秒刷新。失败时保留日志、中间文件和已经发布成功的包，不会删除远程/本地产物做自动回滚。进程重启前的未完成任务显示中断。

## 注入给发布脚本的环境变量

| 变量 | 含义 |
| --- | --- |
| `LOCALSERVICE_URL` | 构建端的上传 origin，默认 http://127.0.0.1:8787 |
| `LOCALSERVICE_TOKEN_FILE` | 本机发布密钥文件路径，不是密钥内容 |
| `LOCALSERVICE_ROOT` | Localservice 源码目录，可调用其 scripts/push.sh |
| `LOCALSERVICE_PROJECT_ID` | 当前项目 ID |
| `LOCALSERVICE_JOB_ID` | 当前任务关联 ID，push.sh 会自动发送 job_id |
| `LOCALSERVICE_OUTPUT_DIR` | 本任务专用产物目录，位于服务数据目录外部项目之外 |
| `LOCALSERVICE_GIT_COMMIT` | 拉取成功后实际构建的 HEAD |

脚本负责实际构建、签名、打包、选择版本/build 和依次上传，失败必须非零退出。采用 `set -euo pipefail`；不要启用 `set -x` 打印凭据。不要让后台进程继续构建，脚本必须等待其所有子任务完成。

下面仅展示已完成打包之后的调用，不能代替真实构建：

```bash
"$LOCALSERVICE_ROOT/scripts/push.sh" \
  "$LOCALSERVICE_PROJECT_ID" "$VERSION" "$BUILD" \
  ios "$IPA_PATH" dev arm64 default

"$LOCALSERVICE_ROOT/scripts/push.sh" \
  "$LOCALSERVICE_PROJECT_ID" "$VERSION" "$BUILD" \
  macos "$DMG_PATH" dev universal desktop-dmg

"$LOCALSERVICE_ROOT/scripts/push.sh" \
  "$LOCALSERVICE_PROJECT_ID" "$VERSION" "$BUILD" \
  macos "$ZIP_PATH" dev universal desktop-zip
```

一项目支持多个脚本，一脚本支持多个产物。同平台、架构、渠道下不同安装包用稳定的 **variant** 标识区分（例如 desktop、helper、desktop-dmg、desktop-zip）。默认 default 向后兼容既有包。不要每次改 variant 来绕过版本冲突。完整唯一键为：项目 + variant + 平台 + 架构 + 渠道 + version + build。

`GET /api/projects/{project}/updates` 可加 `variant=desktop-dmg`，不传则只检查 default，避免把其他安装包当成更新。页面支持按安装包标识筛选。

直接用 multipart API 的脚本要发送 `job_id=$LOCALSERVICE_JOB_ID`。脚本退出 0 但没有关联的上传记录会显示“未完成”；普通 AI/手工上传不需要 job_id。相同文件的幂等上传也会关联到本次任务。

## 服务配置和接口

服务 `-build-root` 指向包含 push.sh 的 Localservice 目录；默认当前目录。`-build-origin` 默认 `http://127.0.0.1:8787`。更改监听端口或启用 TLS 时要显式设置构建能访问的 origin；脚本不会自动绕过 HTTPS 校验。

以下全部需要管理员 Bearer 鉴权：

- `POST /api/projects/{id}/build-source`：JSON `{ "path":"/absolute/project", "branch":"main" }`，关联目录，不拉取或执行脚本。
- `GET /api/projects/{id}/build-source`：扫描 Git 与脚本、Markdown、阻塞原因。
- `POST /api/projects/{id}/builds`：JSON `{ "script":"scripts/release-ios.sh" }`，发起拉取与发布，返回 202 和任务 ID。
- `GET /api/projects/{id}/builds`：最近 30 个任务、阶段、commit、关联 release_ids、失败原因。
- `GET /api/builds/{job}/log`：任务日志。

源码与说明已经实现；按用户要求不执行真实 git pull、构建发布或真机测试。需要项目 AI 把真实 release*.sh 与同名 Markdown 提交到可拉取的发布分支后，由用户在页面执行。
