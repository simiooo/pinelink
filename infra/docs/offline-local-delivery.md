# 无中心服务的本地直连契约

SIM-848 · 2026-11-01 · 用户新增硬要求：互联网和Pinelink API完全不可用，但设备网络可达时，仍能配对、确认接收并端到端传输。此能力独立于云账户登录与中心数据库。

现有DIRECT只把文件字节直连，仍依赖服务器完成公钥发现、请求、信令、进度和回执。WebRTC并不规定信令实现，需另外建立控制通道。[WebRTC文档](https://webrtc.org/getting-started/peer-connections)

## 交付条件

- 新设备无须先在线注册；身份和加密密钥在设备生成并持久保存。
- 同链路使用Bonjour/DNS-SD发现；mDNS是链路本地机制，跨子网/VPN或禁组播时，以扫码/粘贴完整连接码取得可达IP和设备公钥。[RFC 6762](https://www.rfc-editor.org/rfc/rfc6762.html)
- 发现的设备名和地址只是候选，不能作为信任。首次配对使用短期一次性邀请、完整公钥固定及双方确认；六位校验码只用于人工比较，不充当鉴权秘密。
- 已配对设备使用设备签名和内存短期会话鉴权。会话绑定进程启动和一次性challenge，重启后旧会话与重放请求失效；配对信任持久化，可显式撤销。
- 接收前禁止发送文件分块。拒收、取消、暂停、过期与完成在设备本地串行裁决；接收确认来自已落盘的连续密文字节。
- 文件继续使用DARE20与age应用层端到端加密。完整密文/明文校验、文件同步和本地提交后，才签发接收设备回执。
- 回执与历史先落本地；重新联网是后续同步动作，不是本地交付成功的条件。离线传输不虚构云副本或Restore能力。

## 传输及网络

DIRECT表示两端直接交换，控制位置和具体传输方式分别记录。本地快照包含`authority=LOCAL_DEVICE`、`authority_device_id=<接收设备公钥ID>`、`transport=LOCAL_HTTPS`；前端使用控制域和ID共同索引，不与在线UUID/中心回执混合。在线WebRTC仍保留；本地原生核心使用固定设备公钥的TLS 1.3 / HTTPS单端口控制与密文分块通道，在UDP/STUN/TURN不可用时也能交付。复用标准TLS/TCP和现有应用层加密，不实现自制穿透或传输重传协议。IP直连客户端禁用环境代理、HTTP重定向和外部DNS。

本地HTTPS监听器属于接收设备本身，不是第三台服务器。对端只要能到达该设备的本地监听端口，即可完成整个闭环。网络隔离/防火墙已阻断双方通信时不能承诺传输；发现失败也不等于设备不可达。

ONLINE请求期限仍为10分钟；每端按自己的本地权威计时，跨设备传递剩余期限，不要求双方时钟同步。接受与续传不重置原期限。配对邀请默认2分钟；已完成接收的用户文件不因邀请过期而删除。

## 附近发现与IoT经验

同链路默认自动mDNS发现，发现结果只提供候选设备。一次确认配对同时保存双方公钥及已运行接收端的地址；新会话必须重新证明私钥持有，撤销信任立即作用于两方向。CLI通过`lan discover`列出候选，当前已知地址变更可用`--address`覆盖，仍固定原设备密钥。

后续原生界面采用“自动局域网发现 → BLE辅助发现/安全配对 → 已配对设备自动重连”，二维码是兜底入口。协议选择留在连接层；自动接收文件与设备重连分别授权。BLE不承担大文件传输，也不能把公开广播的名称、短ID或六位数字当作设备信任。新的BLE配对需评审标准安全配对机制；现有连接码的256位一次性邀请通过二维码/粘贴交换，六位比较码由两端各自计算，仅供人工核对，不能直接改造成公开BLE PIN。

Matter将BLE和基于IP的发现用于不同阶段，可借鉴其流程；这里不引入Matter设备栈。Google Nearby的多传输发现也可作平台适配参考。BLE、后台发现、移动端权限及自动地址更新尚未在本工程单元实现。[Matter发现](https://developers.home.google.com/matter/primer/commissionable-and-operational-discovery)、[Nearby](https://developers.google.com/nearby/overview)。

## 跨端落地边界

- 桌面/移动原生客户端：本地监听、系统Bonjour/NSD或共享核心发现、固定公钥连接、相同分块与回执契约；权限拒绝须显示短原因和连接入口。iOS需声明本地网络用途与Bonjour服务。[Apple本地网络权限](https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy)
- Web：按已安装并预缓存资源的离线应用设计；首次取得应用资源需要可用来源。浏览器受监听、服务发现和证书能力限制，使用手动交换/扫码的本地WebRTC信令适配，不能把原生HTTPS监听器当作浏览器已经具备的功能。[Service Worker离线缓存](https://developer.mozilla.org/en-US/docs/Web/API/Service_Worker_API/Using_Service_Workers)
- 此工程单元先交付可复用本地协议、原生核心/CLI及隔离网络证据；Web/Flutter真实界面适配逐端验收。仅有设计稿或CLI不能称所有客户端已完成离线保证。

## UI与动效

OpenPencil v21补充本地设备、首次配对、无设备/权限入口、本地传输和本地完成。断开互联网不把仍可达的本地节点降为离线，也不放全屏错误页。主图仍是两个设备与一条曲线；数字和动作留在详情。蓝色为接收端持久确认量，沿用1600ms流光；暂停、校验、终态及reduced-motion停用。

首次配对在二级modal/bottom sheet展示设备名、校验码和短动作。连接码包含端点与完整公钥/一次性邀请，二维码在运行时生成；设计预览码无鉴权用途。描述文字不进入设备主图。设备进入160ms淡入、离开120ms淡出；网络变化只更新真正受影响的设备可达性，不重建本地传输。

## 必须验证

在Docker internal网络或同等明确断开公网的两端进程，禁用中心API、外部DNS、STUN、TURN与代理，完成首次配对和真实文件传输；验证两个独立进程的文件hash、接收前拒绝分块、身份固定、重放拒绝、取消/暂停/恢复、进程重启、本地历史、空文件、IPv4/IPv6和无组播手动连接。验收记录须标明实际覆盖的平台，原生验证不冒充浏览器验证。

分块先同步文件，再以SQLite WAL/FULL事务提交checkpoint；业务变化与增量日志/outbox同事务，详见[本地历史与三方同步](local-history-sync.md)。完成前还要同步接收文件并提交签名回执。此次验证覆盖进程重启，不包含突然断电或文件系统故障压力验收。

## CLI入口

两端均运行接收服务以支持双向发送。身份保存在`PINELINK_CONFIG_HOME`，一台设备使用一个稳定的owner/device组合；同时启动CLI不会覆盖另一进程刚生成的密钥。

```sh
pinelink lan serve --receive-dir ./Received --advertise https://192.168.1.20:19091
pinelink lan discover
pinelink lan pair 'pinelink://local/v1/…'
pinelink lan local requests
pinelink lan local approve <pair-request-id>
pinelink lan send ./example.pdf --peer <verified-peer-id>
pinelink lan local transfers
pinelink lan accept <transfer-id>
pinelink lan pause <transfer-id>
pinelink lan resume <transfer-id>
pinelink lan resume-send <transfer-id>
pinelink lan history
pinelink lan changes --after 0
pinelink lan storage
pinelink lan local forget <peer-id>
```

`--offer-only`只创建文件提议，之后用`resume-send`发送；暂停/取消可从发送端或接收端执行，接受/拒绝只在接收端执行。`lan local invite`刷新短期连接码；邀请和私钥不得提交或贴到公开日志。默认要求配对确认与文件接收确认；`--auto-pair`、`--auto-accept`是显式自动化开关。

历史直接读本地记录，即使对端关闭也能验证已保存的完成回执；活动条目是上次保存的快照，实时状态使用本地接收服务查询。接收目录中文件不因原尝试过期而回收；离线传输没有云端Restore副本。
