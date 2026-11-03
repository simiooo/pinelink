# 动效调研与 Pinelink 的取舍

调研日期：2026-11-03。范围为官方设计指南、框架文档和项目实际源码。下面的具体时长、幅度和覆盖方案是 Pinelink 提案，不冒称 Instagram／Threads 的内部参数，也不照搬组件库外观。

| 一手来源 | 借鉴点 | 在 Pinelink 中的应用 |
| --- | --- | --- |
| [Meta Origami：Spring Animation](https://origami.design/documentation/patches/builtin.springanimation)、[Gesture](https://origami.design/documentation/patches/builtin.layer.gesture) | 交互中的位置、速度和松手后的运动衔接；支持中途改变目标 | 节点按压、选择、拖拽靠近／退出、sheet 松手回位；快速反向时从当前位置继续 |
| [Meta Engineering：Pop](https://engineering.fb.com/2014/04/28/ios/introducing-pop-the-animation-engine-behind-paper/) | 2014年的历史工程案例，说明动态弹簧、衰减、可中断动画与手势速度的关系 | 采用这些原则，不引入这个旧的 iOS 库，不声称它是当前 Instagram／Threads 的实现 |
| [Material 3：Motion physics](https://m3.material.io/styles/motion/overview/how-it-works) | 将空间运动与颜色／透明度效果分开；统一少量运动 token | 仅节点、拖拽、选中等空间反馈可有微小弹性；透明度、蓝灰进度和文字颜色不越界、不回弹 |
| [Apple HIG：Motion](https://developer.apple.com/design/human-interface-guidelines/motion) | 明确因果、简短反馈、允许取消；沿来时方向退场，保留平台习惯 | 面板靠近来源，sheet 从底部入退；系统文件选择器、键盘和系统菜单由平台处理，不叠加第二层进场 |
| [W3C WCAG 2.3.3](https://www.w3.org/WAI/WCAG22/Understanding/animation-from-interactions) | 用户应能禁用非必要交互运动；该条为 AAA，不能把它误写成 AA | 跟随系统减少动态效果，关掉位移、缩放、弹簧和循环流光，状态与操作结果仍完整可见 |
| [MDN：Web Animations API](https://developer.mozilla.org/en-US/docs/Web/API/Web_Animations_API/Using_the_Web_Animations_API) | 明确播放、暂停、反向、取消和清理的生命周期 | 评审原型用原生 WAAPI；进入后台或离开可见区停止循环，旧动画不能覆盖新目标 |
| [Flutter animations](https://docs.flutter.dev/ui/animations/overview)、[disableAnimationsOf](https://api.flutter.dev/flutter/widgets/MediaQuery/disableAnimationsOf.html) | 区分隐式、显式动画及 controller；系统设置变化应更新相关视图 | 后续产品使用 Flutter 自带动画和 spring simulation，controller 按所有者释放，后台禁用 ticker，减少运动时直接呈现状态 |

## 设计决定

增加丰富但有层级的反馈：常用操作轻而快，改变设备关系、接收完成和拖拽落点的反馈更明显。每个适用组件均有进入、交互、状态变化、退出和中断的处理结论；共享按钮、tooltip、tab 等复用同一套运动，不为每个实例创造新节奏。

基础节奏拟定为 80ms（立即反馈）、120ms（小状态）、160ms（控件与小浮层）、220ms（sheet／布局关系）、280ms（完整二级页面）；跟手状态不播放预设入场。空间弹簧只允许小幅回弹。流光沿既有真实确认弧段循环，暂停不播放；联网、验证、授权和送达结果必须先真实提交，动画才表达结果。

按用户既定风格保留 Nunito、#0866FF、白灰底、语义间距、局部左对齐、平面层级。旧 Courier 吉祥物、密集说明和传统卡片堆不追加装饰性动画，列入待淘汰清单。静态正文、文件名、验证码、错误原因和焦点环保持稳定；它们的容器可以做必要过渡，不逐字跳动。

## 检查范围

统计当前 Web／Flutter 源码的自定义组件、交互实例、视觉原语及已有动画；再统计最新 v28 原生文档的页面、画板和语义组件。旧设计文件仅作为演变参考，不与最新稿重复计数。源码已有、最新设计待实现、系统原生负责、静态保持和待淘汰分别标记，不能把“有设计”或“框架默认 ink”写成新产品动效已完成。
