# 开源与预览发布

公开仓库为 [simiooo/pinelink](https://github.com/simiooo/pinelink)，采用 Apache-2.0。Web、Flutter、API、CLI、协议和部署工具在同一仓库维护，各自可独立构建。

源码、设计与部署示例保留在 Git；二进制、Web 静态包、SHA256SUMS、版本 manifest 和第三方许可放在对应 GitHub Release。构建缓存、设备身份、数据库、凭据与维护者内部验收记录不进入公开源码或安装包。

首个公开版本从整理后的源码快照起步，私有研发历史另行保留。文档中的 loopback 地址和测试凭据用于本地开发，不代表官方服务配置。

## Go 模块

模块路径为 `github.com/simiooo/pinelink/backend`、`cli`、`infra`、`protocol`。根目录 `go.work` 提供本地开发替换，模块 `go.mod` 不携带指向其他目录的 replace，便于独立消费已发布版本。

子模块使用目录前缀 tag，例如 `protocol/v0.1.0-preview.1` 与 `cli/v0.1.0-preview.1`。根 tag `v0.1.0-preview.1` 对应整套预览包；manifest 记录实际源版本。

## 发布步骤

1. 检查拟公开源码、Git 历史和附件，不包含密钥、真实凭据、本机数据库或操作记录。
2. 先发布协议模块依赖；检查 workspace 与 `GOWORK=off` 构建，运行相关测试和 staging 冒烟。
3. 提交最终源码并建立根 tag 和各模块 tag，构建版本必须与提交一致。
4. 使用 `infra/scripts/release-preview.ps1` 生成预览附件和校验文件，保留 Apache LICENSE/NOTICE 与第三方许可。
5. 创建 GitHub draft prerelease，上传并下载核对附件，最后发布 prerelease。生产与 iOS 发布仍需另行授权。

当前预览客户端不代表官方消费级客户端已经完成。正式服务域名为 `pinelink.sqmem.top`；正式受信配置、自动身份引导、服务切换与完整跨端文件数据面需完成后，才能发布官方正式版本。
