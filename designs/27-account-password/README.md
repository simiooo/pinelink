# Pinelink v27 / Google 与账号密码

2026-11-03。根据用户反馈修改的 OpenPencil 设计稿。Google、账号密码注册/登录与邮件恢复已接入 Web、Flutter 和服务端；实际发布与验证记录随版本交付。v26 的邮箱免密登录提案不采用。账号关联、修改密码与完整设备图仍属于后续范围。

- **Google**：直接授权，首次成功时自动建号，随后进入设备图。不要求另填邮箱、密码或邮箱验证码。
- **账号密码**：邮箱和密码登录；新用户填写邮箱和一次密码，再验证邮箱。支持显示密码、密码管理器、粘贴与自动填充。
- **访问恢复**：忘记密码、验证码错误/过期/重发、发送失败、Google 取消、离线、会话过期、退出及跨设备打开邮件均有独立状态。验证后直接回到设备图或原操作，不加昵称、教程或成功页。
- **激活邮件**：按钮和八位码二选一。自有域名 `/a/` 加 22 位随机字符，正式域名完整链接为 51 字符；GET/扫描不消耗，确认通过 POST。

Google 按钮独立于密码表单，不读、提交或校验其中的邮箱和密码。账号设置中的可选登录方式关联要求单独证明，不成为普通 Google 登录的必经步骤，也不按邮箱字符串静默合并账号。

## 原生文件与预览

[OpenPencil 文件](../27-pinelink-account-password.fig) 保留 v25 已验收的 20 页，并将 v26 的三页账号提案替换为本版：

| 页面 | 新增画板 |
| --- | ---: |
| 20 / Account entry and verification | 35：密码/Google 入口、注册、邮箱证明、错误、320px、键盘和设备图菜单 |
| 21 / Account recovery and sessions | 36：Google 回调恢复、密码重置、短链、账号、关联、离线及会话 |
| 22 / Account contract | 4：注册邮件、重置邮件、流程和动效契约 |

共 75 个新增画板。[manifest.json](manifest.json) 使用原生保存、关闭并重开后的 ID；再次重开可能重新编号，应按页面与画板名称核对。

- [桌面登录](desktop-sign-in.png)、[手机登录](mobile-sign-in.png)、[320px 登录](mobile-sign-in-320.png)。
- [手机注册](mobile-sign-up.png)、[320px 注册](mobile-sign-up-320.png)、[密码错误](mobile-sign-in-error.png)、[短密码](mobile-sign-up-weak.png)。
- [Google 等待](mobile-google-wait.png)、[验证码和键盘](mobile-verify-keyboard.png)、[忘记密码](mobile-forgot.png)、[新密码](mobile-reset-password.png)、[跨设备邮件](mobile-link-other-device.png)。
- [账号](mobile-account.png)、[设备图入口](mobile-graph-entry.png)、[注册邮件](email-signup.png)、[重置邮件](email-reset.png)、[流程](flow-contract.png)、[动效](motion-contract.png)。

在 OpenPencil 中完成修改、原生保存和重开，再导出 18 张预览。三张新增页面字体均可用、无替代；FRAME/TEXT 父容器越界与同级重叠检查为 0。人工检查登录、注册、错误、320px、键盘、Google 无邮箱输入、跨设备邮件、设备图入口和邮件。检查范围与文件哈希见 [review-evidence.json](review-evidence.json)。这些是设计检查，不能代替运行代码测试。

## 实施边界

[account-contract.md](account-contract.md) 记录账号密码、Google、短链、邮件、设备绑定、会话、租户边界、并发和验收条件。产品验收该具体稿后再实施 UI；后端及客户端必须测试、推送、实际部署 staging 并完成相关路径冒烟后才算工程交付。当前 staging 的完整用户登录注册仍未接通。

`node designs/27-account-password/build-frames.mjs <output.json>` 生成 73 个表单/契约画板的 JSX。导入后，将 `Intrinsic /` 文本设为 `WIDTH_AND_HEIGHT`，给同名 Google 图层填入官方图像；另两个设备图画板从继承页面原生复制，保留矢量路径，再补账号菜单。动效是原生文档中的交互规范，尚未实现成可运行原型。

字体沿用已验收的 Nunito，颜色为 `#0866FF`、白灰黑；8px 标签关系、16px 字段/操作、24–32px 功能分组，密集表单局部左对齐。Google 资产遵循[官方品牌规则](https://developers.google.com/identity/branding-guidelines)，来源见 [brand-source.json](brand-source.json)，不因项目 Apache-2.0 而重新许可。
