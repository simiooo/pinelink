# Pinelink 14：Meta 参考节点图设计

2026-11-01。本轮是设计交付，使用英文界面稿。最终入口为 `../14-pinelink-meta-graph.fig`，包含四页：Foundations、Desktop、Mobile、Components。桌面和移动端各八个状态；组件页有十二个可编辑组件。实际应用尚未按本稿实施。

总览预览：`desktop-board.png`、`mobile-board.png`。OpenPencil 实际导出与重开证据：`desktop-all.png`、`mobile-all.png`、`receive-verified.png`、`components-verified.png`。具体检查和文件哈希见 `verification.json`。

本稿以本轮最新要求为准：借鉴 Instagram、Threads、Muse 的产品观感，引入蓝色 primary，保留平面节点图。此前角色插画、暖棕色和禁止蓝色等历史方向不适用于本轮。中文备份在 `../13-blue-graph/chinese-backup.fig`。

## 调研与取舍

- [Carbon spacing](https://www.carbondesignsystem.com/building-blocks/foundations/spacing/overview)：有限的间距阶梯、由父容器管理组间距离，用距离表达关联与层级。
- [Atlassian spacing](https://atlassian.design/foundations/spacing/) 与 [typography](https://atlassian.design/foundations/typography/)：8px 基准、4px 细节；字号与行高成对定义。
- [GNOME Grid Views](https://developer.gnome.org/hig/patterns/containers/grid-views.html)：内容优先，减少网格装饰，限制网格宽度，在不同窗口宽度下检查。
- [Meta / Threads web](https://about.fb.com/news/2025/04/new-features-threads-web-experience/)：参考官方界面图中的轻量工具栏、克制分隔和内容层级。
- [How We Designed Muse](https://introducing.muse.ai/)：参考状态页和批准操作截图。采用浅灰操作底、黑色图标、胶囊确认按钮与明确的接受/拒绝；不沿用角色、聊天或其他产品特有功能。
- [Instagram identity](https://www.meta.com/design-at-meta/blog/the-new-instagram-brand-identity/)：仅作为字标与整体产品气质参考，不复制其专有字体或字标。

这些来源支持设计取舍；下列具体色值、容器尺寸与间距是 Pinelink 的选择，不是对 Meta 内部 token 的推断。

## 视觉规则

| 角色 | 值 |
|---|---|
| Primary / hover | `#0866FF` / `#005CE6` |
| Primary soft | `#EEF5FF` |
| 主文字 / 次文字 | `#101010` / `#6E6E6E` |
| 浅灰底 / 分隔 | `#F5F5F5` / `#EBEBEB` |
| 边框 / 文件轮廓 | `#DBDBDB` / `#808080` |
| 页面 | `#FFFFFF` |

工具栏默认只有图标；悬停才出现浅灰圆底，48px 命中区独立于图标。节点的圆形表达设备，普通节点用浅灰实底，选中用 2px 蓝色描边。关键确认使用 48px 高的胶囊按钮；一般窗口采用 24px 圆角，手机底部 sheet 只在顶部使用 28px 圆角。无渐变、伪 3D、厚底座或塑料阴影。

字体使用 Segoe UI；中文实施时按 Microsoft YaHei / 系统字体回退。字号与行高：字标 24/32、标题 20/28、正文及节点 15/20、辅助文字及手机文件名 13/18。主要字重 400、600。

## 间距与容器

间距阶梯：4 / 8 / 12 / 16 / 20 / 24 / 32 / 48 / 64。组内 8–16，容器与分组 20–32，大区块 48–64；节点到标签统一 12。不得把所有距离设成相同的值。

| 项目 | 桌面 | 移动 |
|---|---|---|
| 画板 | 1200×800 | 390×844 |
| 页面边距 | 32 | 20，另计系统安全区 |
| 设备 / 本机 / 选中直径 | 64 / 96 / 96 | 56 / 80 / 80 |
| 布局 | 中轴 600，节点横坐标偏移 ±104 / ±288 | 中轴 195，两列设备组 160 + 16 + 160 居中 |
| 文件记录 | 528×352 浮层，内边距 24 | 全屏二级页面 |
| 文件列 | 3×144，列距 24 | 3×106，列距 16 |
| 文件单元 / 行节奏 | 80 / 128 | 80 / 128 |

图区域限制在约 800px 宽的范围。窗口继续变宽时增加外侧留白，避免节点和文件无限拉开。手机顶部两列与下方本机独立排布，不等比缩小桌面稿。

## 状态与交互

八个状态：Devices、Selected device、Transferring、Transfer history、Receive request、Paused、Connection lost、Empty state。

点选设备后只出现 Send / Folder / More 三个操作。历史入口位于 More → Transfer history。其他节点与连线退至 0.13 透明度，当前设备不留重复的背景副本。图标动作有简短 tooltip、可访问名称与键盘焦点，焦点环 2px、外间距 4px，Esc 返回上一层。

传输画面仅保留目标、本机和单条曲线；灰色轨道 3px，蓝色活动段 4px，约占路径 12%，1.8 秒匀速循环。减少动态效果时固定该段。它只表示活动，不是真实完成百分比。暂停或断线时曲线静止，再显示简短原因和恢复操作。接收确认明确显示来源、文件以及 Decline / Accept。

文件条目只显示图标和单行文件名；过长时省略号截断，保留完整文件名供可访问名称或 tooltip 使用。小云标记只用于保管语义。

## 验证与交付边界

OpenPencil 中保留可编辑文本和矢量图层；颜色绑定到 Pinelink token，关键按钮使用原生 auto-layout Frame。十二个组件位于 Components 页。

桌面、移动及组件页的对比度、最小字号与触控检查为零错误、零警告；检查画板之间无重叠，并导出关键页面做视觉复核。最终 `.fig` 需关闭后重开并再次导出，确认保存不是空白或损坏的文件。

这是静态设计和交互规范，不宣称已创建可点击原型或完成应用实施。原有 SIM-840 的数据面、Git 与 staging 后续事项未因本轮设计而标记完成。
