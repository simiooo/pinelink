# Pinelink 角色与竞品调研结论

## 竞品观察

| 产品 | 主要视觉路线 | 可借鉴 | 不采用 |
|---|---|---|---|
| OpenAI / ChatGPT | 以产品标志、语音 orb 和状态反馈为主，没有固定人格快递员 | 克制、状态清晰、动画不抢信息 | 把松鼠变成纯 loading orb |
| Grok Companions | Ani、Rudi 等完整角色，人格、语音和亲密关系驱动 | 角色轮廓鲜明、表情和动作有记忆点 | 强人格、亲密度、暧昧或情绪化叙事 |
| Claude / Claude Duet | 强调可信任、判断、批准与重要决策留给用户 | 状态事实优先、敏感操作需明确同意 | 用可爱动画掩盖权限和保管变化 |
| Google Duet / Gemini | 从功能符号发展到可定制 Live Avatar，强调多模态和品牌形象 | 角色可保持统一外观并适配多场景 | 让角色人格凌驾于任务之上 |

来源： [Claude Duet 官方说明](https://claude.com/resources/articles/claude-duet)、[Gemini Live Avatar 官方说明](https://blog.google/innovation-and-ai/models-and-research/gemini-models/gemini-3-8-live-with-live-avatar/)、[Grok Companion 报道与界面示例](https://gigazine.net/gsc_news/en/20250715-grok-app-companion/)、[OpenAI 用户对 Voice Orb 的讨论](https://community.openai.com/t/optional-animated-avatar-for-chatgpt-voice-mode/1391160)。

## 定案：栗小递是快递员，不是聊天机器人

角色职责只有三项：谁正在送、送到哪了、对方是否拿到。角色不能替系统声称成功，也不能用动作掩盖异常。

三个不可变记忆点：

1. 大尾巴，尾尖始终顺时针卷曲。
2. 腮帮上的几何坚果壳纹理，不画写实毛发。
3. 头顶一撮呆毛，固定右倾 20°。

传输中不使用配饰，不根据直连或中继改变颜色、速度或角色。只有真实暂停、完成、失败、权限和保管状态改变动作。

## 视觉与动效原则

- 米白、浅木、柔灰为底，焦糖与栗色为角色点缀；禁止科技蓝、霓虹紫和网络拓扑图。
- 低饱和暖纸感、圆润几何形、无锐角，保证小尺寸剪影可识别。
- 角色动画不能决定真实时长，进度只来自真实字节与确认。
- 状态变化用短过渡，默认 200–600ms；错误动作短而克制，不弹跳、不滑稽。
- 支持减少动态效果、键盘焦点、屏幕阅读器文本和非动画状态表达。

## 点对点传输产品调研

| 产品 | 成熟交互模式 | 对 Pinelink 的约束 |
|---|---|---|
| LocalSend | 选择内容后点选附近设备，无账号、无服务器，平台一致 | 首页只突出文件与接收设备，减少设置和角色干扰 |
| AirDrop | 从系统分享入口选择接收者，接收方接受或拒绝，自设备可自动接收 | 同意是内容发送前的明确边界，设备身份与来源说明必须清楚 |
| Google Quick Share | 附近设备列表、发送/接收职责明确、接受/拒绝和整体百分比进度 | 发送与接收入口不混杂，进度直接而克制 |
| KDE Connect | 先配对信任，再从文件或系统菜单发送，后台完成并通知 | 已信任设备与临时设备分层，历史与系统通知可复用 |
| PairDrop | 临时房间、持久配对、整体进度、多文件打包下载 | 临时与持久关系不混淆，多文件任务保持单一进度语义 |

来源：[LocalSend 官方说明](https://localsend.org)、[AirDrop 官方指南](https://support.apple.com/guide/iphone/use-airdrop-to-send-items-to-nearby-devices-iphcd8b9f0af/ios)、[Quick Share 官方说明](https://blog.google/products-and-platforms/platforms/android/how-to-use-quick-share-android/)、[KDE Connect 文件传输指南](https://userbase.kde.org/Tips/Transfer_files_between_phone_and_PC_using_Wi-Fi)、[PairDrop 功能说明](https://github.com/schlagmichdoch/PairDrop)。

## 专业版式与操作数硬约束

- 主屏可见操作不得超过三个：首页为“选择文件 / 通知 / 更多”；次要功能进入“更多”。
- 接收页为“拒绝 / 接受 / 更多”；任务详情为“暂停或继续 / 取消 / 更多”。
- 采用中性浅灰 `#F5F6F7`、白底容器、主文字 `#30363D`、次文字 `#626C76`、细边框 `#E2E6EA`。
- 深栗色只用于主要动作和关键状态，不整页铺暖色，不使用营销落地页式大卡片。
- 间距按 4/8/12/16/24/32 组织，组内紧凑、组间拉开；桌面与移动共享同一信息层级，而不是机械缩小。
- 松鼠只在有传输语义的状态中出现，不作为首页装饰或营销插画。

## OpenPencil 原型能力边界

使用 Linux 构建机上的官方 `@open-pencil/cli` v0.16.0。122 个编辑器工具中没有点击原型工具；向原生 Frame 写入 Figma `reactions` 后，重新打开 `.fig` 回读为空数组，未形成可点击链接。因此 `08-interaction-flow.fig` 是静态交互流程稿，不能冒充可点击原型。真实点击、键盘操作、状态版本和失败反馈由 TanStack Web 与 Flutter 的交互测试及实际页面验收。

## 数据面与发送准备边界

当前双端界面接入真实通知、接受/拒绝、任务控制、历史、设备、容量和保管提升。文件选择只生成已登记 CLI 的发送命令，不宣称 Web/Flutter 已执行加密数据面发送。实际加密、直连/中继、交付和重复下载继续由 Go CLI 与后端完成，并通过真实文件内容校验。双端原生数据面及设备签名身份引导不在本轮 UI 验收中冒充完成。

## i18n 国际化约束

- 支持 `zh-CN` 与 `en-US`；中文以 `zh` 前缀识别，其他系统语言回退到英文。
- 默认跟随系统；用户可在“更多”中显式选择中文或 English。选择不增加主屏操作。
- Web 使用 `localStorage` 保存 `pinelink.locale`，Flutter 使用 `shared_preferences` 保存同一键；系统语言变化也会更新界面。
- `i18n/zh-CN.json` 与 `i18n/en-US.json` 是唯一文案源，Flutter 通过 `dart run tool/sync_i18n.dart` 生成字典。状态、异常、隐私说明、时间和无障碍文本均须翻译，不能将服务端内部编码直接作为界面文案。
- `09-i18n.fig` 审查英文长文案和语言菜单，实际渲染另见 `mobile/test/goldens/09-i18n-home.png` 与 `09-i18n-menu.png`。
