# Pinelink

跨设备端到端加密文件传输，支持直连、中继、云端暂存与离线局域网。

[源码](https://github.com/simiooo/pinelink) · [开发预览包](https://github.com/simiooo/pinelink/releases) · [Apache-2.0](LICENSE)

## 当前进度

这是开发预览版本。Go 服务端与 CLI 已具备加密传输、接收确认、真实接收进度、云副本生命周期、离线原生直传及加密增量同步。Web 已支持 PWA、既有页面离线冷启动、本地历史与恢复联网后的自动补同步；Flutter 已接入本地持久化和同步数据层。

Web 访客模式支持无需注册的一次性设备连接与独立接收确认。已配对浏览器优先尝试 WebRTC 直连，失败或中断后使用同一密文和接收账本回退云端；节点图按真实路径显示两个端点或设备—云端—设备，保留真实蓝灰进度、暂停、刷新恢复与本机文件保存。登录不改写在途访客身份。启用方式、资源限制和验证边界见[访客传输](infra/docs/guests.md)及[直连验证](infra/docs/web-direct-tests.md)。

Web/PWA 新增独立的本地连接：双方交换二维码或签名连接包、核对号码并确认，无中心、外部 DNS、STUN/TURN 时仍可配对和加密收发。暂停、断链重连、持久化确认量与签名回执由接收设备负责；桌面本地启动包解决首次未缓存网页的离线启动。浏览器不能通用扫描 Bonjour，采用显式交换发现目标，详见[无中心服务的 Web 局域网收发](infra/docs/web-serverless-lan.md)。

账号模式和 Flutter 的完整文件数据面、原生离线发现的界面适配，以及新版节点图的其余界面仍在实施。Web 本地连接暂不与原生 CLI 的 TLS 数据面互操作；已有原生 CLI 离线能力不能冒充 Android 界面交付。实际部署版本以交付记录为准。

Web/Flutter 的账号预览已接入 Google 首次授权自动建号、邮箱与密码注册/登录、邮件验证码及短链恢复。Google 入口独立，不要求另填邮箱；服务端确定账号后才登记该账号的设备密钥。部署方式与身份边界见[账号服务](infra/docs/accounts.md)，已验证的运行范围见[账号验收记录](designs/27-account-password/runtime-evidence.json)。

产品计划默认连接官方服务 `pinelink.sqmem.top`，并允许使用自定义服务。官方模式不展示技术配置表单；高级配置只在主动连接自己的服务时提供。完整服务切换和官方生产服务尚未发布。当前预览包用于开发和受控的自托管验证；关闭账号服务时使用的默认开发身份不能用于公开多用户服务。

## 仓库结构

| 目录 | 内容 |
| --- | --- |
| `backend/` | Go API、传输协调与生命周期维护 |
| `cli/` | CLI、WebRTC 与离线局域网传输 |
| `protocol/` | 共享协议与加密增量复制格式 |
| `web/` | React / TanStack Web 与 PWA |
| `mobile/` | Flutter 客户端 |
| `infra/` | 数据库迁移、本地开发、部署与集成检查 |
| `designs/` | OpenPencil `.fig`、SVG 与动效原型 |

四个 Go 模块通过根目录 `go.work` 协同开发，公开模块路径为 `github.com/simiooo/pinelink/<module>`。

## 本地开发

需要 Go 1.26、Node.js 24、Docker Compose；Flutter 使用 [.fvmrc](.fvmrc) 固定的 stable SDK。

```sh
git clone https://github.com/simiooo/pinelink.git
cd pinelink
docker compose -f infra/compose.yaml up -d --wait
go -C backend run ./cmd/pinelinkd
```

后端环境变量、开发身份与 CLI 命令见[开发说明](infra/README.md)。开发 Compose 使用已知的本地测试凭据，默认仅绑定 loopback。跨设备验证需显式设置绑定地址与 ICE/TURN 地址。

```sh
go -C cli run ./cmd/pinelink --help
cd web
npm ci
npm run dev
```

Flutter 在 `mobile/` 目录执行 `flutter pub get`、`flutter run`。Windows 可使用 `infra/scripts/setup-flutter.ps1` 安装项目独立 SDK，再通过 `infra/scripts/flutter.ps1` 调用。

## 检查与构建

```sh
go -C protocol test ./...
go -C backend test ./...
go -C cli test ./...
npm --prefix web test
npm --prefix web run build
```

Flutter 执行 `flutter analyze` 与 `flutter test`；Windows 字体金图依赖 Microsoft YaHei，Linux CI 明确跳过这组字体专用参考图。完整数据库/API/双端传输检查通过 `bash infra/scripts/integration.sh` 启动服务后运行，`infra/integration` 需要相应环境变量，不能当作无服务的单元测试执行。

Releases 预览包附带源代码版本、SHA-256 校验文件和第三方许可声明；构建脚本见 [release-preview.ps1](infra/scripts/release-preview.ps1)。

## 设计与协议

- [架构](infra/docs/architecture.md)
- [直连与中继送达](infra/docs/direct-relay-delivery.md)
- [离线局域网及本地配对](infra/docs/offline-local-delivery.md)
- [本地持久化与三方增量同步](infra/docs/local-history-sync.md)
- [Web 离线与 PWA](infra/docs/web-offline-pwa.md)
- [Web 无中心服务的局域网连接与本地启动包](infra/docs/web-serverless-lan.md)
- [已验收设计基线](designs/20-pinelink-direct-relay.fig)
- [服务模式与离线补充稿](designs/23-pinelink-service-and-offline.fig)：仍待原生应用重开检查和产品验收。

## 许可

Pinelink 使用 Apache-2.0，见 [LICENSE](LICENSE) 与 [NOTICE](NOTICE)。第三方组件保留各自许可证，构建包中的许可附件属于对应版本的一部分。
