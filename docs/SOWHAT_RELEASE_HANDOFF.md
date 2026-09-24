# Sowhat 开发 AI：构建并发布到 Localservice

目标：由 Sowhat 的开发 AI 完成新的构建、签名和打包，然后自动上传，让用户在 Localservice 页面看到真实可下载的安装包。用户自行验证安装和运行；不要上传占位包冒充构建成功。

## 先读取

1. `/Users/ted/Documents/workspace/ioant-sowhat/AGENTS.md` 及其要求的项目文档/任务流程。
2. `/Users/ted/Documents/workspace/hero-skills/skills/publish-localservice-builds/SKILL.md`。
3. 该 skill 的 `references/localservice-contract.md`。

这份文档只包含此 Mac 的接入事实和参考命令；不取代 Sowhat 自身的开发规范。不要继承其他 AI 的 Session ID。

## 开发 AI 的管理员发布授权

项目所有者已明确允许负责 Sowhat 及其他接入项目的开发 AI，以 Localservice 管理员身份直接发布各自负责项目的安装包。接到构建发布任务后，直接在本机读取下方发布密钥文件，通过 Bearer 鉴权创建所需项目、上传构建并确认发布记录；无需用户先登录页面，也无需对每次常规发布再次请求确认。通过 `scripts/push.sh` 调用时会自动读取密钥文件并带上管理员鉴权。

发布密钥只在执行进程中读取，不把内容展示给用户、写入日志、复制进知识库或提交 Git。同一台 Mac、可读取该文件的开发 AI 可直接使用；其他机器或其他系统用户需要先配置可访问的服务地址与受保护的凭据文件。

这里的管理员是分发服务管理员，不是 macOS root 或 Apple 账号管理员。当前密钥对所有项目有效，但开发 AI 只操作其负责项目；没有独立 AI 账号或项目级权限隔离。管理员发布权限不意味着新设备已在 Apple 登记，也不免除有效签名与 HTTPS 安装条件。

## 已准备好的接入参数

| 项目 | 值 |
| --- | --- |
| Sowhat 源码 | `/Users/ted/Documents/workspace/ioant-sowhat` |
| Localservice 源码 | `/Users/ted/Documents/workspace/ioant-localservice` |
| 本机发布 origin | `http://127.0.0.1:8787` |
| 当前局域网页面 | `http://192.168.1.7:8787`，IP 变化时需更新 |
| 服务项目 ID / 名称 | `sowhat` / `Sowhat`，已通过 API 创建 |
| 发布密钥文件 | `/Users/ted/Documents/workspace/ioant-localservice/.localservice/admin-token` |
| Xcode project / scheme | `Sowhat.xcodeproj` / `Sowhat` |
| Bundle ID | `com.ioant.sowhat` |
| 当前识别的团队 | `64RS366WKG`，执行前确认 Xcode 登录与签名身份可用 |
| 发布渠道 | `dev` |

目前创建了项目记录，没有替 Sowhat 执行新构建、上传安装包或登记 Apple 设备；以上准备不能作为构建/安装成功证据。HTTPS 尚未配置，HTTP 页面可展示与下载，不能完成 iOS 网页安装或设备描述文件回传流程。

## 管理页面自动发布入口

Localservice 已支持通过本地目录识别根目录 / scripts 下的 release*.sh。请同时读取本目录 [RELEASE_SCRIPTS.md](RELEASE_SCRIPTS.md)，按其环境变量和多产物 variant 规范实现脚本及 Markdown。脚本和说明必须提交并同步到可拉取的 main 分支；页面执行的是原项目目录，请保持该目录干净，执行期间避免并行编辑。脚本通过注入的 `LOCALSERVICE_ROOT/scripts/push.sh` 发布，会自动关联当前任务；不要覆盖注入的 origin、凭据、项目 ID 或 job ID。

## 选择正确的构建路径

现有 `scripts/build-install-sowhat-ios-lan.sh` 即使设置 `SOWHAT_IOS_INSTALL=0`，仍要求 `SOWHAT_IOS_DEVICE_ID` 并检查设备可达，且产物是 development IPA。不要用它冒充无需连接手机的 Ad Hoc 网页分发。

现有 `scripts/install-sowhat-macos.sh` 会停止当前 App、覆盖安装目录并启动 App；本次只要发布安装包，应该新增或使用 **package-only** 路径，避免直接调用安装脚本。

建议由 Sowhat AI 增加 `scripts/release-ios.sh`、`scripts/release-macos.sh`，需要一次发布全部平台时可加 `scripts/release-all.sh`，每个文件配同名 `.md`（例如 `scripts/release-ios.md`）：支持明确平台、版本、build、渠道；构建成功后调用下面的上传步骤；遇到失败立即停止并保留日志。不要把密钥写入脚本或仓库，也不要同时调用直接上传和服务端重复签名。

## iOS 首次发布：archive → Ad Hoc export → push

以下为参考命令，交给开发 AI 根据项目现状执行；这里没有执行过。`VERSION` 与 `BUILD` 必须先结合源码、当前发布历史选择，不要每次重复使用构建号 1。

```bash
set -euo pipefail
APP_ROOT=/Users/ted/Documents/workspace/ioant-sowhat
export LOCALSERVICE_ROOT=/Users/ted/Documents/workspace/ioant-localservice
export LOCALSERVICE_URL=http://127.0.0.1:8787
export LOCALSERVICE_TOKEN_FILE="$LOCALSERVICE_ROOT/.localservice/admin-token"
PROJECT_ID=sowhat
TEAM_ID=64RS366WKG
CHANNEL=dev

# 在当前 shell 设置准备发布的真实版本和递增的正整数构建号。
# 不在此预填固定值，避免重新执行产生冲突。
: "${VERSION:?Select a SemVer such as 0.1.0}"
: "${BUILD:?Select a new positive integer build number}"

cd "$APP_ROOT"
OUTPUT="$APP_ROOT/.build/localservice/ios/$VERSION-$BUILD"
mkdir -p "$OUTPUT"
# 输出目录必须是本次构建专用；已有输出时先检查原因，不覆盖已发布构建。
[[ ! -e "$OUTPUT/Sowhat.xcarchive" ]] || { echo 'Archive already exists; choose a new build or inspect existing output' >&2; exit 1; }

# 如项目图标尚未生成，先按项目规范运行该脚本；不要将产物当业务变更混入提交。
bash scripts/generate-sowhat-app-icons.sh

xcodebuild -project Sowhat.xcodeproj -scheme Sowhat \
  -configuration Release -sdk iphoneos -destination 'generic/platform=iOS' \
  -archivePath "$OUTPUT/Sowhat.xcarchive" \
  -derivedDataPath "$OUTPUT/DerivedData" \
  DEVELOPMENT_TEAM="$TEAM_ID" CODE_SIGN_STYLE=Automatic \
  MARKETING_VERSION="$VERSION" CURRENT_PROJECT_VERSION="$BUILD" \
  SUPPORTED_PLATFORMS=iphoneos \
  -allowProvisioningUpdates archive 2>&1 | tee "$OUTPUT/archive.log"

cat > "$OUTPUT/ExportOptions.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>method</key><string>release-testing</string>
  <key>teamID</key><string>$TEAM_ID</string>
  <key>signingStyle</key><string>automatic</string>
  <key>manageAppVersionAndBuildNumber</key><false/>
</dict></plist>
EOF

xcodebuild -exportArchive -archivePath "$OUTPUT/Sowhat.xcarchive" \
  -exportPath "$OUTPUT/export" -exportOptionsPlist "$OUTPUT/ExportOptions.plist" \
  -allowProvisioningUpdates 2>&1 | tee "$OUTPUT/export.log"

# 精确选择这次 export 产生的唯一 IPA，不要从旧 dist 目录随便找一个。
ipa_files=("$OUTPUT"/export/*.ipa)
[[ ${#ipa_files[@]} -eq 1 && -f "${ipa_files[0]}" ]] || { echo 'Expected exactly one exported IPA' >&2; exit 1; }
ARTIFACT="${ipa_files[0]}"

# RELEASE_NOTES 包含本次源码 revision、变更摘要、分发方式；不包含凭据。
export RELEASE_NOTES="Sowhat iOS $VERSION ($BUILD); Ad Hoc; source $(git rev-parse --short HEAD)"
"$LOCALSERVICE_ROOT/scripts/push.sh" \
  "$PROJECT_ID" "$VERSION" "$BUILD" ios "$ARTIFACT" "$CHANNEL" arm64 \
  > "$OUTPUT/publish.json"
cat "$OUTPUT/publish.json"
```

在上传前读取 IPA 内主应用的 Info.plist，确认 `com.ioant.sowhat`、`VERSION`、`BUILD` 均匹配；核对嵌入描述文件为 Ad Hoc、未过期、包含目标设备。正确性检查属于打包必要步骤，不等同于替用户运行 App 测试。遇到签名/账号/设备错误，说明缺少的条件，不擅自改成 unsigned、development 或 App Store 包。

命令里的 `-allowProvisioningUpdates` 会允许 Xcode 更新 Apple 签名资源，但不会自动登记网站收集到的任意 UDID。保留 archive 供后续更新描述文件时重新导出；服务当前将相同版本/build 的不同文件视为冲突，因此要发布一个新的文件，通常需要递增 build 并重新归档。

## macOS 发布

由 Sowhat AI 按该项目选择的分发方式实现 package-only 构建。当前项目文档使用开发签名，不能直接宣称它在任意 Mac 都可安装；面向其他 Mac 的通用分发应选 Developer ID 并按需要处理 notarization。不要绕过 Gatekeeper。

已得到正确签名的 `.app` 后，可用以下方式保留资源和权限打 ZIP；`APP_PATH`、架构、版本和 build 必须从实际构建确认：

```bash
: "${APP_PATH:?Absolute path to the just-built Sowhat.app}"
: "${OUTPUT:?Absolute path to this build output directory}"
: "${VERSION:?Version from Contents/Info.plist}"
: "${BUILD:?Build from Contents/Info.plist}"
: "${ARCHITECTURE:?Actual arm64, x86_64 or universal}"
ditto -c -k --sequesterRsrc --keepParent "$APP_PATH" "$OUTPUT/Sowhat-$VERSION-$BUILD.zip"
"$LOCALSERVICE_ROOT/scripts/push.sh" sowhat "$VERSION" "$BUILD" macos \
  "$OUTPUT/Sowhat-$VERSION-$BUILD.zip" dev "$ARCHITECTURE"
```

架构可用 `lipo -archs "$APP_PATH/Contents/MacOS/Sowhat"` 查看；不要把仅 arm64 的应用标为 universal。服务不会检查 macOS ZIP 内应用签名/架构/版本，这些由构建端保证。

## 发布完成的判定

1. 上传返回 201（新发布）或 200（相同文件已存在），记录 `id`、`sha256`、`download_url`。
2. `GET http://127.0.0.1:8787/api/projects/sowhat/releases` 能找到这一真实 release。
3. 提供局域网可访问下载地址：当前为 `http://192.168.1.7:8787` 加返回的相对 `download_url`。页面选择 Sowhat，点击刷新或等 20 秒。
4. 明确报告“构建、导出、发布”的实际结果；安装和运行由用户测试，不能把 HTTP 201 写成安装通过。

若服务没有运行，可在 Localservice 目录执行 `./scripts/start.sh`。不要在同一数据目录启动第二个实例。密钥错误用正确文件；409 需检查是否同一 build 的不同包；不要删除历史记录绕过冲突。

## 可选：把归档接入页面的“请求 Mac 签名”

首次直接导出上传即可，不强制配置第二条路径。若用户希望以后在网页触发重新导出，把新产生的绝对 archive 路径合并进 `.localservice/signing.json` 的 `projects.sowhat`，保留其他项目：

```json
{
  "archive": "/actual/output/path/Sowhat.xcarchive",
  "team_id": "64RS366WKG",
  "method": "release-testing",
  "channel": "dev",
  "allow_provisioning_updates": true
}
```

这条接口只签名/导出已有归档，不会构建最新源码或自动递增 build。对已经发布的 archive 重新签名，若 IPA 字节不同会出现 409，必须按版本策略处理。

## 新设备首次安装

```text
服务配置受信任 HTTPS
→ iPhone Safari 下载设备登记描述文件
→ 用户到系统设置确认
→ 服务收集 UDID（仍是 pending_apple_registration）
→ 管理员核对并在 Apple Developer 登记
→ 更新涵盖新设备的 Ad Hoc 描述文件
→ 新构建号归档 / 导出签名 IPA / 发布
→ iPhone 页面选择新包进行安装
```

设备登记不是自动 Apple 注册；Xcode 登录信息可供签名使用，不代表服务实现了 Apple 设备注册 API。当前 `identity_verified: false` 也不能作为自动信任设备的凭据。首次登记后，后续包只要仍包含该 UDID 且签名/profile 有效，设备无需重复登记。

Apple 说明：[登记设备分发](https://developer.apple.com/documentation/xcode/distributing-your-app-to-registered-devices)、[更新描述文件的设备集合](https://developer.apple.com/help/account/provisioning-profiles/edit-download-or-delete-profiles)。
