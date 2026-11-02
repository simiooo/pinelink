# Pinelink 开发基础设施

## 启动中间件

```bash
docker compose -f infra/compose.yaml up -d
```

| 服务 | 构建机端口 | 开发凭据 |
|---|---|---|
| PostgreSQL | `55432` | 用户/数据库 `pinelink`，密码 `pinelink_dev` |
| NATS | `54222` | 无认证，仅开发 |
| NATS 监控 | `58222` | JetStream 已启用 |
| SeaweedFS S3 | `59000` | 用户 `pinelink`，密码 `pinelink_dev_secret` |
| SeaweedFS Master / Filer | `59333` / `58888` | Bucket `pinelink-dev` 由后端确保存在 |
| coturn STUN/TURN | `53478` UDP/TCP | 用户 `pinelink`，密码 `pinelink_dev_turn` |
| TURN relay 端口段 | `49160-49180/udp` | 仅限可信开发网络 |

Compose 默认只绑定 loopback。跨设备验证时显式设置 PINELINK_DEV_BIND 和 PINELINK_TURN_EXTERNAL_IP，并使用设备可达地址；仅在可信开发网络使用测试凭据。公开服务必须配置独立身份授权、外部密钥、TLS、NATS 授权和数据库访问控制。

## 运行后端

从 `backend` 目录执行，并按实际构建机地址调整环境变量：

```bash
export PINELINK_DATABASE_URL='postgres://pinelink:pinelink_dev@127.0.0.1:55432/pinelink?sslmode=disable'
export PINELINK_NATS_URL='nats://127.0.0.1:54222'
export PINELINK_S3_ENDPOINT='127.0.0.1:59000'
export PINELINK_HTTP_ADDR='127.0.0.1:8080'
export PINELINK_DEVICE_AUTH_REQUIRED=true
# 仅当 Web/Flutter Web 与 API 不同源时设置；不要对公网放开任意来源。
export PINELINK_CORS_ORIGINS='http://127.0.0.1:5173,http://localhost:5000'
go run ./cmd/pinelinkd
```

后端启动时执行 Goose 迁移，确保 S3 Bucket 和 JetStream Stream 存在，并启动 Outbox、生命周期维护器及 WebRTC 应用中继。`PINELINK_DEVICE_AUTH_REQUIRED` 在本地明文联调时可设为 `false`，验签集成测试始终覆盖开启状态。

STUN/TURN 以 JSON 注入 CLI 和后端：

```bash
export PINELINK_ICE_SERVERS='[
  {"urls":["stun:127.0.0.1:53478"]},
  {"urls":["turn:127.0.0.1:53478?transport=udp"],"username":"pinelink","credential":"pinelink_dev_turn"}
]'
```

## CLI 常用流程

首次使用各终端先登记设备。CLI 会在配置目录生成 age/X25519 加密身份和 Ed25519 签名身份：

```bash
go run ./cmd/pinelink device register
```

云暂存、通知与下载：

```bash
go run ./cmd/pinelink upload /path/to/file --retention TEMP_24H
go run ./cmd/pinelink send ENTRY_ID --to-owner RECIPIENT --to-device DEVICE
go run ./cmd/pinelink notifications
go run ./cmd/pinelink accept TRANSFER_ID
go run ./cmd/pinelink download ENTRY_ID /path/to/output --transfer TRANSFER_ID
```

任务控制与历史：

```bash
go run ./cmd/pinelink tasks list --include-deleted
go run ./cmd/pinelink tasks pause TASK_ID
go run ./cmd/pinelink tasks resume TASK_ID
go run ./cmd/pinelink download ENTRY_ID /path/to/output --pull-id PULL_ID
go run ./cmd/pinelink tasks projection TASK_ID
```

广播、持久化与 P2P：

```bash
go run ./cmd/pinelink broadcast ENTRY_ID --to-owner RECIPIENT
go run ./cmd/pinelink promote ENTRY_ID
go run ./cmd/pinelink p2p send /path/to/file --to-owner RECIPIENT --to-device DEVICE --path auto
go run ./cmd/pinelink p2p receive TRANSFER_ID /path/to/output --path auto
```

接收 P2P 前必须先 `accept TRANSFER_ID`。`--path` 支持 `direct`、`relay`、`auto`。Windows 首次运行时需要按组织安全策略允许专用网络上的 WebRTC 入站连接，否则直连会回退到中继。

文件下载先校验密文，再在终端解密并校验明文。每次完整下载都会创建新的 `PullSession`；`--resume` 使用 `.part`，P2P 使用 `.p2p`，只有完整校验通过才生成目标文件。

## UI 开发预览

先按上节启动后端，并将 `PINELINK_CORS_ORIGINS` 设置为实际页面来源，例如 `http://127.0.0.1:5173,http://localhost:5001`。Web 与 Flutter Web 联调可将 `PINELINK_DEVICE_AUTH_REQUIRED` 设为 `false`；真实 CLI 数据面仍使用设备签名身份，生产部署不得沿用无验签配置。

```bash
cd web
npm install
npm run dev

cd mobile
flutter pub get
flutter run -d chrome --web-port=5001
```

可通过 `VITE_PINELINK_API_URL`、`VITE_PINELINK_OWNER`、`VITE_PINELINK_DEVICE`、`VITE_PINELINK_SCOPE` 配置 Web；Flutter 使用对应的 `--dart-define=PINELINK_API_URL=...`、`PINELINK_OWNER`、`PINELINK_DEVICE`、`PINELINK_SCOPE`。容量作用域必须与 CLI 的 `--scope` 一致，默认均为 `dev-scope`。移动端原生包需要 HTTP 网络权限，Android 已在主清单中声明。

双端界面覆盖通知、接受/拒绝、任务控制、历史、设备、容量与保管提升。原生加密数据面和设备签名身份引导未接入本轮 UI；发送准备只生成真实 CLI 命令，不能视为已在 Web/Flutter 内发送。OpenPencil 静态流程稿见 `designs/`，当前工具不持久化点击原型，真实交互由双端测试验收。UI 跟踪见 `SIM-840`。

界面支持 `zh-CN` 与 `en-US`，默认跟随系统语言。“更多”中的语言选择使用 `pinelink.locale` 持久化：Web 使用 `localStorage`，Flutter 使用 `shared_preferences`。唯一文案源为 `i18n/zh-CN.json` 和 `i18n/en-US.json`；修改文案后在 `mobile/` 执行 `dart run tool/sync_i18n.dart` 生成 Flutter 字典，再运行双端测试。

## 集成测试

在可信开发网络中，从工作区根目录执行：

```bash
PINELINK_MIDDLEWARE_HOST=127.0.0.1 bash infra/scripts/integration.sh
```

脚本构建三个项目，启动普通后端和强制设备验签后端，并验证：多分块与重复分块、上传/下载续传、域内秒传、跨域隔离、通知与同意/拒绝、状态冲突、暂停/继续/取消/删除/历史、广播、提升、GC、配额、设备验签、服务端密文、稳定投影、JetStream，以及 Pion 直连、中继、自动回退和中途路径切换。

测试使用唯一作用域，不重置开发数据库；临时二进制和测试输出在结束后删除。可用 `PINELINK_TEST_FILTER=<测试名>` 只运行指定用例。

停止中间件：

```bash
docker compose -f infra/compose.yaml down
```

只有明确需要清空全部开发数据时才使用 `down -v`。

架构决策见 `infra/docs/architecture.md`，功能验收矩阵见 `infra/docs/features.md`，实施与 Nuto 跟踪见 `SIM-838`。
