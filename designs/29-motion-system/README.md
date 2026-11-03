# Pinelink v29 · 微交互与动效提案

2026-11-03 · SIM-898 · 用户已采用 v29；Web/Flutter 的共享实现与现有功能绑定见[客户端实施记录](runtime-implementation.md)。本目录原型中的模拟确认、收件回执和复制结果仍为演示事件，不能当作产品功能完成证据。后续新增的 shadcn 组件源码遵循同一视觉规范。

- [OpenPencil 可编辑文件](../29-pinelink-motion-system.fig)：保留 v28 的27个非空页面，新增5页32张关键帧，共298张画板。新增页从 `27 / Motion system` 到 `31 / Motion accessibility`。
- [可播放原型](motion-lab.html)：自包含 HTML，离线打开即可播放；支持桌面/移动、系统/完整/减少动态效果、反向、慢放、暂停和阶段比较。左侧109项是状态/交互审计目标，共享29种配方，不代表109个已实现产品组件或109套独立业务场景。
- [调研与取舍](research.md)、[完整覆盖表](coverage.md)、[动画与状态契约](motion-contract.md)、[演示操作说明](preview-guide.md)。
- [staging 可播放评审](https://staging-pinelink.sqmem.top/designs/motion-v29.html)：沿用现有预览认证。2026-11-03 13:16（Asia/Shanghai）实际部署 Helm12，设计来源 `ad0dd5b`；公网浏览器20项检查通过。应用运行版本仍为 `1daeffa`，仅新增隔离评审路径。证据见 `evidence/staging-preview-deployment.json` 和 `evidence/staging-preview-verification.json`。

## 盘点口径

| 范围 | 数量 | 定位和方法 |
| --- | ---: | --- |
| React 组件 | 12 | 11个含 JSX 的组件与1个非可视 ClientWorker；13个 TSX 源文件、265处 JSX 使用 |
| Flutter Widget 声明 | 21 | 17个 Dart 文件；另列9个 State、1个 CustomPainter，不混称组件 |
| 交互/语义控制声明 | 70 | Web 47处、Flutter 23处，均有文件和行号、处理结论与目标映射 |
| v28 原生容器实例 | 5,142 | 27页、266画板中的 FRAME/GROUP/COMPONENT/INSTANCE；完整 ID 索引，不能当作5,142种组件 |
| 交互/状态目标 | 109 | 101项适合动画、4项由平台处理、4项内容保持稳定；29种共享配方 |

源码基线为 `545ee8425aa6ad1af6258b722efeea1212e460d0`。扫描结果见 `web-inventory.json`、`flutter-inventory.json`、`coverage.json`、`design-inventory.json`。Web 使用 TypeScript AST；Flutter 是排除注释、字符串和生成文件后的 token 盘点，不冒充 Dart 类型分析。旧 Courier 吉祥物与 CLI 准备界面列为淘汰/替换；i18n AnimatedBuilder 只是语言重建监听，不算视觉动画。新增 v29 文档型容器不重复计入基线。

## 已验证与尚待验证

Fedora 隔离容器中的 Chromium 156 已通过151项原型检查：109项挂载、快速反向、真实确认边界、暂停/迟到 ACK、取消旧结果、嵌套 dragleave、邻近/多目标 drop、焦点回收、草稿保留、动态系统偏好、可见性循环停止和20次销毁/重建。原型没有网络请求或页面异常。测试记录与6张浏览器截图在 `evidence/`；不是产品 React StrictMode、Flutter 手势、真实传输或设备帧率验收。

原生文件已保存、关闭、重开；5个新增页面 Nunito 字体忠实可用，父容器溢出为0，控件同级碰撞为0。图形轨道/确认线、节点分层、drop halo、状态角标的50组重叠为有意叠加，逐项见 `native-verification.json`，没有将检查器的全部警告笼统忽略。`previews/` 提供15组原生 PNG/SVG，原生关键帧自身不能播放动画。

采用后按功能单元串行接入 Web/Flutter；实际 React StrictMode、Dart 生命周期、IME/键盘、低端设备、大列表和业务竞态验证仍是交付条件。账号 v27、跨用户/文本 v28 仍各自需要设计采用，本次动效提案不代替其验收。

## 复跑与来源

在构建机已有依赖环境中运行，不新增产品依赖：

```sh
node designs/29-motion-system/scan-source.mjs "$PWD" /tmp/web-inventory.json
python3 designs/29-motion-system/scan-flutter.py "$PWD" /tmp/flutter-inventory.json
node designs/29-motion-system/build-catalog.mjs
node designs/29-motion-system/build-preview.mjs
node designs/29-motion-system/build-frames.mjs /tmp/motion-v29-frames.json
PINELINK_MOTION_OUT=file:///tmp/pinelink-motion-proof/ node designs/29-motion-system/verify-preview.mjs
python3 infra/scripts/check-design-preview.py --helm /path/to/helm --chart infra/helm/pinelink --output /tmp/helm-preview.json
```

扫描输出默认写到指定路径；重新生成 catalog 前应明确是否要用新盘点覆盖本次基线。浏览器检查默认使用 `web/node_modules/playwright-core` 与 `/usr/bin/chromium`，可用 `PINELINK_PLAYWRIGHT_MODULE`、`PINELINK_CHROMIUM` 指定现有安装。原生 JSX 通过 OpenPencil render API 写入文档，HTML 生成器不会冒充原生保存。

字体使用项目已采用的 Nunito，原始 TTF 与 OFL 1.1 随 HTML 离线嵌入，保留字体版权。Google 登录图形沿用 v27/v28 的官方中性按钮资产（`google-approved.svg`）；按原始180×40呈现，外围48px触控区，等待状态在品牌图形外，不改变标志。图标为已有风格的简单矢量路径。项目源码 Apache-2.0，第三方字体和品牌资产遵守各自许可/指南。
