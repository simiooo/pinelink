# Pinelink v26 / 登录注册与简短激活

**已被取代，未获采用。** 用户要求同时支持 Google 和账号密码，且 Google 不再要求填写邮箱。以下免密提案仅保留为历史记录；当前验收稿与实施契约见 [v27](../27-account-password/README.md)。

2026-11-03。当前为 OpenPencil 设计验收稿，尚未实现或部署这套账号流程。

## 核对结果

运行版本主要提供设备签名认证，没有完整的用户登录、注册页面和账号会话支持。之前的 Google OAuth 工作保留在本地 WIP 提交 `daf4c89`：包含部分 OIDC、账号和设备引导逻辑及迁移草稿，HTTP 接口、Web/Flutter 接入、完整验证与实际部署尚未完成。不能把设备登记视为用户注册，也不能把这份 WIP 当作已上线功能。

## 本次方案

- **统一入口**：Google 登录和邮箱验证码；首次验证成功自动创建账号，老用户使用同一路径登录或恢复访问。本提案不要求设置密码，也不增加密码重置页面。
- **直接使用**：验证与设备绑定完成后返回设备图或刚才的操作。不追加昵称、服务器配置、教程、付款或激活成功页面。图上的 More 菜单提供登录入口；本地能力不以云账号作为前提。
- **就地恢复**：保留邮箱和原目的，补齐错误码、过期、重发、发信失败、Google 取消、会话过期、退出、额外身份验证及其他设备打开邮件的状态。
- **两种邮件继续方式**：短链接按钮和可整段粘贴的八位验证码。链接采用自有域名 `/a/` 加 22 位随机 URL-safe 字符，正式域名下完整 URL 为 51 个字符，不堆叠 JWT 或多层跳转参数。打开链接不会直接消耗凭据或创建会话。

排版沿用用户已确认的 Nunito、`#0866FF`、白灰底、语义分组与局部左对齐。Google 按钮使用官方预批准图像，保持其字形、标志和比例，48px 操作区承载原尺寸图像。

## 原生交付和预览

[OpenPencil 原生文件](../26-pinelink-account-onboarding.fig) 继承 v25 的 20 页，并新增三页：

| 页面 | 内容 |
| --- | --- |
| 20 / Account entry and verification | 26 帧：正常入口、验证码及错误恢复、邮件确认、小屏、键盘、设备图上的账号入口 |
| 21 / Account recovery and sessions | 22 帧：短链恢复、Google 取消/等待、离线、会话过期、账号、额外证明、退出 |
| 22 / Account contract | 邮件、最短流程与动效/交互说明，共 3 帧 |

共 51 个新增画板；[manifest.json](manifest.json) 记录原生保存重开后的页面和节点 ID。重开后 IDs 可能重新编号，核对时以页面和画板名对应，不能沿用编辑前 ID。

- [桌面入口](desktop-entry.png)、[手机入口](mobile-entry.png)、[320px 入口](mobile-entry-320.png)。
- [验证码](mobile-code.png)、[验证码错误](mobile-code-wrong.png)、[验证码过期](mobile-code-expired.png)、[确认中](mobile-code-checking.png)、[键盘打开](mobile-code-keyboard.png)。
- [发信失败](mobile-send-failed.png)、[跨设备邮件](mobile-link-other-device.png)、[短链过期](mobile-link-expired.png)、[Google 取消](mobile-google-cancelled.png)、[离线](mobile-offline.png)。
- [账号](mobile-account.png)、[设备图入口](mobile-graph-account-entry.png)、[邮件](email-preview.png)、[流程](short-flow-contract.png)、[动效契约](motion-contract.png)。

在 OpenPencil 中完成原生保存、关闭和重开。三张新增页面的字体全部可用、无替代，major/critical 文字父容器越界为 0；人工检查核心页面、错误、320px、键盘和设备图入口。预览均来自重开后的文档。这些是设计检查，不能代替客户端或后端测试。

## 实施和复现

[account-contract.md](account-contract.md) 明确账号、短链、邮件发送、会话、身份关联、并发、租户边界和验收条件。产品验收后，继续完成后端/API/邮件与客户端接入，使用 Fedora 测试与构建，推送并实际部署 staging。仍需真实 Google 回调和受控邮箱的实际收信验证。

`node designs/26-account-onboarding/build-frames.mjs <output.json>` 生成供 OpenPencil `render` 使用的 JSX。导入后，将名为 `Intrinsic /` 的文本设为 `WIDTH_AND_HEIGHT`；Google 图像填入对应同名图层。设备图画板从继承页面原生复制、保留矢量路径，再补菜单；动效说明为原生 Frame/Text。最终以原生文档和重开后的 manifest 为准。

Google 按钮图像来自[官方品牌资产](https://developers.google.com/identity/branding-guidelines)，见 [brand-source.json](brand-source.json)。Google 标志和资产遵循其品牌规则，不因 Pinelink 的 Apache-2.0 而重新许可。Nunito 的来源与 OFL 许可沿用 [v25 字体记录](../25-rounded-typography/font-source.json)。
