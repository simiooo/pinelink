# Pinelink v25 / Rounded English typography

2026-11-03，基于已验收的 v20 原生文档更新英文字体。用户已明确采用这一版 Nunito，运行资源已接入 Web 与 Flutter。Fedora Web 43 项、Flutter 25 项测试及零分析问题、两端构建通过；staging 已部署运行 `52ad25a`。

## 字体选择

使用 **Nunito**。圆润的笔画适合 Pinelink 亲和、安静的界面；Duolingo 的[官方字体规范](https://design.duolingo.com/identity/typography)也将 Nunito 列为替代字体。保留现有白灰底、`#0866FF`、图标系统、语义间距与密集内容局部左对齐。

| 角色 | 字重 | 使用方式 |
| --- | --- | --- |
| 正文、文件名、辅助信息 | Regular 400 | 保留原字号与文件名单行省略策略 |
| 操作、选中标签、本机标签 | SemiBold 600 | 用字重表达当前操作与层级 |
| 品牌、标题、主要进度数字 | Bold 700 | 保持紧凑，避免过多强调 |

仅切换英文字体。中文继续使用客户端现有系统字体回退，不随包分发系统字体。Nunito 未覆盖的 `→`、`↔` 在设计说明中使用 Segoe UI 符号字形；对应 14 个文本层的 22 个符号范围，英文仍为 Nunito。

## 设计交付

- [原生可编辑 OpenPencil 文件](../25-pinelink-rounded-typography.fig)：20 个非空设计页面，保留 v20 的设备、传输、拖拽、历史、恢复、状态及契约。
- [桌面历史](desktop-history.png)、[手机历史](mobile-history.png)、[手机设备图](mobile-devices.png)。
- [桌面传输详情](desktop-transfer-details.png)、[手机传输详情](mobile-transfer-details.png)。
- [来源与字体哈希](font-source.json)、[设计检查记录](review-evidence.json)。

共更新 1,452 个文本层。按新字体重新测量文字边界，并修正 371 个居中或右对齐层的水平位置；527 个相关文本层的中心或右边界与原稿一致。保留文件图标、文件名及状态徽标的语义分组。

在 OpenPencil 中原生保存、关闭并重新打开后，20 页的字体检查均为可用、无替代。未发现 major/critical 级文字父容器溢出；人工检查桌面和手机历史、设备图及详情。预览由重新打开后的文档导出。自动检查不代替产品验收。

## 来源和许可

字体来自 [Nunito 官方仓库](https://github.com/googlefonts/nunito)，固定版本 3.602 和提交 `8c6a9bb9732545b9ed53f29ec5e1ab0ff53c4e6f`。原始可变字体及生成的静态 400、600、700、800 字重位于 `fonts/`；当前界面仅使用 400、600、700。

字体采用 [SIL Open Font License 1.1](fonts/OFL.txt)，允许免费商用与随应用分发；必须保留字体版权和许可，不能将字体单独出售。字体继续遵循 OFL，Pinelink 源码的 Apache-2.0 许可不变。

Web 与 Flutter 共享同一上游可变字体文件（SHA-256 见 font-source.json），按 400/600/700 使用。Web 源资源位于 `web/src/assets/fonts/`，构建后产生带内容哈希的字体 URL，并进入 PWA 预缓存；授权文件通过 `/fonts/OFL.txt` 提供并缓存。Flutter 通过 pubspec 注册本地字体，OFL 进入应用许可证包。两端均不通过 Google Fonts 下载 Nunito；中文保留原有平台回退。

接入时还修正了旧运行主题残留的棕色，将 Web/Flutter 基础颜色对齐已验收的蓝白灰规范。本轮是字体和基础主题接入；节点图、完整文件数据面与账号功能仍按各自工程单元继续实施。

实际 staging 浏览器检查使用 Chromium 156，确认了自定义 Nunito 字形、400/600/700 可变字重、本地字体与许可缓存、关闭浏览器后的离线冷启动、历史页离线路由及重新联网。320/390/1280px 无横向溢出，未请求外部字体服务。Flutter 两项跳过分别为 Windows 系统字体旧参考图及需单独 staging 环境的集成测试；不将它们记为已执行。
