# Pinelink v25 / Rounded English typography

2026-11-03，基于已验收的 v20 原生文档更新英文字体。当前为字体设计验收稿；Web、Flutter 和 PWA 的运行代码尚未修改。

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

设计验收后，Web 与 Flutter 将使用本地打包的字体资源；Web 会将字体纳入 PWA 离线资源，不依赖 Google Fonts 在线服务。届时在 Fedora 构建机进行实现验证与构建，再部署 staging 并检查真实离线冷启动。
