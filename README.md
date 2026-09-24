# Localservice

运行在 Mac 的局域网安装包服务。Go 1.24+，无第三方 Go 依赖，前端嵌入单个二进制。iPhone / iPad / Mac 可浏览多项目、最新构建、历史安装包与更新说明。上传和下载均使用流式 IO；文件保存到本地，不依赖云存储。

## 启动

```sh
go build -o bin/localservice ./cmd/localservice
./bin/localservice
```

打开 `http://localhost:8787`。其他设备使用 `http://<Mac局域网IP>:8787`，需要 Mac 防火墙允许入站并处于同一网络。默认监听 `0.0.0.0:8787`；仅本机调试用 `-listen 127.0.0.1:8787`。

首次启动生成 `.localservice/admin-token`（权限 0600）。在 Mac 查看此文件，点击页面“发布管理”输入密钥；然后创建项目、上传包。**不要把密钥提交到 Git 或 AI 对话中**。浏览下载默认对局域网开放；写入、查看设备 UDID、请求签名需要密钥。页面密钥只在内存保存。HTTP 仅适合受信任的开发局域网；需要传输密钥时优先使用 HTTPS。

默认数据目录 `.localservice/`，可用 `-data /absolute/path` 修改。安装包保存在 `artifacts/`，元数据 `state.json` 原子写入。备份整个数据目录。**同一数据目录只能启动一个进程**；目前不支持多实例或集群。没有自动删除历史版本。

## 从本地项目目录点击发布

管理页新增“从项目目录发布”：关联目录后识别当前 Git、分支、upstream 和根目录 / scripts 下的 `release*.sh`，读取同名 `.md` 展示说明。选中脚本后在该目录执行 `git pull --ff-only`，再运行脚本完成构建、打包、上传；工作区不干净或分支不符时停止。支持多个脚本和每次发布多个产物。详细规范见 [RELEASE_SCRIPTS.md](docs/RELEASE_SCRIPTS.md)。

## iOS：签名与“信任”有什么区别

- Safari 下载或信任 `.mobileconfig` **不会让 iPhone 自己给 IPA 签名**。真正的签名使用 Mac 钥匙串中的私钥和 Xcode 的账号。
- 付费 Apple Developer 团队适合 `release-testing`（Ad Hoc）导出。需要 Apple 团队已登记的 UDID、包含设备的描述文件、匹配的 Bundle ID / entitlement 和有效签名。
- `debugging` 导出是开发签名：在设备开启 Developer Mode，再使用 Xcode / Apple Configurator 安装。服务不会把开发包显示成保证可用的网页安装。
- Ad Hoc 网页安装需要 **iPhone 实际信任的 HTTPS 证书**，manifest 与 IPA 都从配置的 HTTPS origin 提供。HTTP 可下载文件，不能满足 OTA 条件。
- 上传 IPA 会读取主应用 Info.plist 和描述文件的类型、设备数量、到期时间。此检查**不等于验证代码签名、设备授权或完整 entitlement 匹配**。页面不会假装安装已成功。
- 免费 Personal Team 不适合本服务的 Ad Hoc 网页分发；其描述文件通常 7 天到期。

## Mac 签名服务

已经支持 HTTP 触发 **Xcode 归档导出和签名**，不是任意来源 IPA 的通用重签服务。保留项目原有扩展和 entitlement 由 Xcode 处理。需要先在 Xcode 设置里登录账号，钥匙串中有证书和私钥，并准备自己的 `.xcarchive`。

```sh
# 只查看团队、有效签名身份、描述文件类型/到期时间，不导出私钥或登录令牌
python3 scripts/doctor.py

# 配置已存在的项目归档，修改 archive 为实际绝对路径
cp scripts/signing.example.json .localservice/signing.json
chmod 600 .localservice/signing.json
```

示例团队 `64RS366WKG` 来自此 Mac 已存在的 Apple Distribution 身份。复制配置不会自动构建业务项目。先在你的项目中构建归档，例如（以该项目自身构建说明为准）：

```sh
xcodebuild -project /path/to/DemoApp.xcodeproj -scheme DemoApp \
  -configuration Release -destination 'generic/platform=iOS' \
  -archivePath /path/to/DemoApp.xcarchive \
  DEVELOPMENT_TEAM=64RS366WKG CODE_SIGN_STYLE=Automatic \
  -allowProvisioningUpdates archive
```

页面“iOS 签名与安装 → 请求 Mac 签名”，或 `POST /api/projects/demo-app/signing`（Bearer 鉴权）。返回 `202` 与任务 `id`；`GET /api/signing/{id}` 查询 `running / succeeded / failed`，同样需要鉴权。一次只运行一个签名任务，20 分钟超时，成功后自动上传到该项目。当前只允许本机配置中的 archive；远程不能指定任意命令或本地路径。

`allow_provisioning_updates: true` 允许 Xcode 使用已登录账号向 Apple 更新签名资料，可能创建描述文件/证书。**它不保证注册浏览器提交的新 UDID**。新设备在 Apple Developer 网站登记后再导出；否则仍不可安装。日志只存本机 `.localservice/signing/<id>/xcodebuild.log`，不会通过公开接口泄露。任务成功与失败会保存在本地；服务重启后的未完成任务需要重新发起。修改 signing.json 无需重启。

再次签同一版本、同一 build 后，文件可能不同；这时上传会返回 409，需在源码递增构建号并重新归档，或选择另一发布渠道，不会覆盖历史安装包。

## 设备自助登记入口

1. 配置 HTTPS（下一节），iPhone 在 Safari 打开页面。
2. 点击“登记这台 iPhone / iPad”，下载 `localservice-device.mobileconfig`。
3. 到“设置 → 通用 → VPN 与设备管理”确认。该描述文件没有 MDM、根证书、SCEP 或签名权限，只请求 UDID、产品型号、系统版本，可能显示“未签名”。
4. iOS 回传设备信息；页面提示已收集。管理员输入发布密钥并点击“查看待登记设备”，复制 UDID 到 Apple Developer 的 Devices 登记。
5. 更新 Ad Hoc 描述文件，再请求 Mac 签名；从分发页面安装新包。

登记 challenge 有效 15 分钟、一次使用。设备资料保存在受限的本地 `devices/`，公开列表不返回 UDID。服务校验回传 CMS 内容签名，但**未实现 Apple 设备证书链认证**，所以记录标为 `identity_verified: false` 和 `pending_apple_registration`，必须由管理员核对，不能据此自动信任设备或耗用 Apple 设备名额。手机实际描述文件安装、重定向和 OTA 仍需真机验证。

## HTTPS

已有受设备信任的域名与证书时：

```sh
./bin/localservice -listen 0.0.0.0:8787 \
  -public-url https://builds.example.com:8787 \
  -tls-cert /path/to/fullchain.pem -tls-key /path/to/private-key.pem
```

也可以由本地 HTTPS 反向代理终止 TLS，服务监听环回地址；`-public-url` 填用户访问的 HTTPS origin。服务只使用这一固定地址生成 IPA/manifest/设备回传链接，不信任任意 Host 请求头。自签证书需要设备先建立完整信任，仅在描述文件上点击信任不够。第一版不自动创建/安装根证书、不修改 DNS 或防火墙。

## 项目 / AI 自动推送

Sowhat 开发 AI 的本机接入参数与构建发布步骤见 [SOWHAT_RELEASE_HANDOFF.md](docs/SOWHAT_RELEASE_HANDOFF.md)。通用知识已整理为 hero-skills 的 `publish-localservice-builds` skill；Sowhat 是首个接入项目，不限制其他项目复用。

**开发 AI 可以直接作为管理员发布。** 用户已授权负责接入项目的开发 AI 在构建发布任务中读取本机 `.localservice/admin-token`，通过 Bearer 接口创建项目、上传安装包及查询发布结果，不需要网页登录或每次再次确认。`scripts/push.sh` 已支持这一方式。凭据不向模型输出或写入文档；其他机器需单独配置受保护的凭据文件。当前是服务级共享管理员密钥，没有每个 AI / 项目的独立账号或权限隔离，AI 应只操作其负责的项目。

先创建项目 `demo-app`。在打包成功后调用：

```sh
export LOCALSERVICE_URL=http://127.0.0.1:8787
export LOCALSERVICE_TOKEN_FILE=/path/to/ioant-localservice/.localservice/admin-token
export RELEASE_NOTES='新增语音入口，修复连接断开'
./scripts/push.sh demo-app 1.0.0 12 ios /path/to/DemoApp.ipa dev arm64
./scripts/push.sh demo-app 1.0.0 12 macos /path/to/DemoApp.dmg dev universal
```

脚本不会在命令行参数或正常输出里打印密钥。支持包最大 4 GiB。iOS 的 version / build 必须与 IPA 的 `CFBundleShortVersionString / CFBundleVersion` 完全一致。版本目前使用完整 SemVer `x.y.z[-prerelease][+metadata]`；构建号必须是正整数，不接受 Apple 允许的点分构建号。macOS 支持 `.dmg / .pkg / .zip`，iOS 支持 `.ipa`；不支持裸 `.app` 或上传 `.xcarchive`。

接口返回相对 `download_url`；AI/客户端应按服务 origin 解析。相同项目、安装包标识 variant、平台、架构、渠道、version 和 build 下，相同 SHA-256 重试返回 200；不同文件返回 409。页面每 20 秒刷新，新包自动显示，旧包保留。

| 接口 | 功能 | 鉴权 |
|---|---|---|
| GET /api/health | 服务状态、OTA 配置状态、上传上限 | 无 |
| GET /api/projects | 项目列表 | 无 |
| POST /api/projects | JSON `{ "id":"demo-app", "name":"SoWhat" }` | Bearer |
| GET /api/projects/{project}/releases | 版本历史，SemVer + build 降序 | 无 |
| POST /api/projects/{project}/releases | multipart: version, build, platform, architecture, channel, variant, notes, file（构建脚本另传 job_id） | Bearer |
| GET /api/projects/{project}/updates | 检查更新 | 无 |
| GET /api/releases/{id}/download | 下载与 Range 断点续传 | 无 |
| GET /api/releases/{id}/manifest.plist | 合适的 iOS 包生成 OTA manifest | 无 |
| POST /api/projects/{project}/signing | 签名并发布预配置归档 | Bearer |
| GET /api/signing/{id} | 签名任务状态 | Bearer |
| GET /api/devices/enroll.mobileconfig | 一次性设备信息收集描述文件 | 无 |
| POST /api/devices/callback/{challenge} | iOS CMS 设备回传 | 一次性 challenge |
| GET /api/devices | 设备 UDID 待办列表 | Bearer |

更新检查示例：

```sh
curl 'http://127.0.0.1:8787/api/projects/demo-app/updates?platform=ios&architecture=arm64&channel=dev&current_version=1.0.0&current_build=11'
```

先比较 SemVer，再比较 build；不同渠道不会混用，架构筛选包含兼容的 universal 包。元数据 `+...` 不影响 SemVer 大小；预发布版本小于同版本正式版。客户端比服务新时不会提示降级。浏览器无法自动读取手机上已安装 App 版本，需要 App 调用接口或手动输入。

## 交付范围与待配置项

按要求不继续执行测试，实际签名、手机描述文件安装与 OTA 下载由使用者验证。

已经实现服务代码、网页、HTTP API、设备资料收集、Xcode 归档签名任务和自动发布脚本。实际签名与安装还需要有效项目归档、Apple 网络访问、已登记设备、Ad Hoc 描述文件及受设备信任的 HTTPS。目前没有自动绑定任何业务项目，也没有修改其他项目。

本机已读取到团队 `64RS366WKG` 和 Apple Distribution 身份；并未导出登录令牌、账号密码或私钥。本机当前找到的描述文件为开发类型，Ad Hoc 分发需要 Xcode 为指定项目导出适用描述文件。

Apple 参考：
- [登记设备分发](https://developer.apple.com/documentation/xcode/distributing-your-app-to-registered-devices)
- [创建 Ad Hoc 描述文件](https://developer.apple.com/help/account/provisioning-profiles/create-an-ad-hoc-provisioning-profile)
- [开发者模式](https://developer.apple.com/documentation/xcode/enabling-developer-mode-on-a-device)
- [设备信息收集协议（归档文档）](https://developer.apple.com/library/archive/documentation/NetworkingInternet/Conceptual/iPhoneOTAConfiguration/profile-service/profile-service.html)
- [账号能力与 Personal Team 限制](https://developer.apple.com/support/compare-memberships/)
