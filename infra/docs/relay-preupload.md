# 中继预上传、接收同意与引用安全回收

2026-11-01 · OpenPencil v19/v20设计契约。后端与CLI已由SIM-847实现，当前实施映射见[云存储授权](cloud-storage-authority.md)；v20 Web/Flutter界面仍在后续单元。

v20范围澄清见[直连、中继与云策略](direct-relay-delivery.md)：本文Cloud主图专指云上传/云端接收阶段。在线中继与直连一样展示两台设备，主进度取接收端确认量；云暂存进度独立，不能替代送达。

## 1. 产品行为

选择中继路线后，发送者可以在接收者同意前，把端到端加密分块上传至服务器。接收者同意控制下行读取权限，不阻塞发送者上行。拒绝、取消或到期时撤销本次投递权限、释放本次投递引用；其他有效引用仍存在时保留底层对象。

这不改变 ONLINE / ASYNC / BROADCAST 意图，也不改变现有过期策略。直连仍保留接收同意门槛。临时保留不能静默提升为持久化。

| 场景 | 上行 | 接收者下行 | UI |
|---|---|---|---|
| PENDING，选择 RELAY | 可上传密文 | 禁止；仅请求所需元数据 | This device → Cloud；蓝色为持久上行确认量 |
| PENDING，副本 SEALED | 上行完成，发送者可离线 | 仍禁止 | Cloud 时钟徽标；Stored / awaiting recipient，不能称 Delivered |
| 上传未完成，接收者 Accept | 继续上传 | 同意事务提交后，可转发已持久确认分块 | Cloud → This device；独立下行进度 |
| SEALED 后 Accept | 无需发送者在线 | 从完整副本拉取 | 同上；完成须经校验、落盘、回执 |
| PENDING → REJECTED | 停止该投递的上传授权 | 立即撤权 | 保留历史，无 Restore；后台回收 |
| 发送者取消 / 请求到期 | 撤销对应投递授权 | 撤销对应投递授权 | 停止流光、退出连接线；历史保留 |

Decline 仅作用于尚未接受的请求。已接受的传输沿用取消/撤权策略，不把迟到 Decline 解释为另一种操作。Accept 与 Decline 并发由同一传输版本裁决，失败者取得当前快照。

上行完成须完整密文清单验证、对象持久化和封存事务成功；接收完成须客户端解密、内容校验、本地提交和首次交付回执。`UPLINK_SEALED` 与 `DELIVERY_RECEIPT` 分别通知。上行封存后不再占用发送端数据连接，不受现有等待接收超时影响。

已同意的在线接收者可边上传边接收，不额外等待整文件封存；离线恢复和 Keep available 仍要求 SEALED。封存前同意不等于已可离线恢复。

## 2. 权限与引用分开建模

建议在现有模型上增加引用账本和存储代际；以下字段为设计名，实施时与迁移统一。

| 实体 | 权威职责 |
|---|---|
| TransferRequest | 接收决定、意图、参与者、原始请求期限和业务版本 |
| DeliverySpool | 首块写入前绑定 transfer、upload、version / object_generation；拒绝时能找到尚无 FileEntry 的上行 |
| ObjectReference | 指向不可变对象代际的引用：类型、持有者、来源操作、期限、released_at、唯一操作键 |
| AccessGrant | 可撤销的接收/恢复能力，绑定参与者、设备、引用、期限、revocation_epoch |
| ObjectGeneration | LIVE / DELETING / COLLECTED，封存清单、gc_after、delete_token 和版本 |
| IOLease / retention hold | 有界读写/提升活动，绑定引用、代际、期限和 fencing epoch |

引用区分 DELIVERY_STAGING（仅为本次投递暂存）、LIBRARY（原有私有文件库）、PERSISTENT_KEEP（明确 Keep available 后的独立保留权）。分享原有文件只增加分享引用；拒绝不得删除原文件库 entry。Keep 仍须授权、有效期、完整性和配额校验。

当前 CompleteUpload → insertEntry 自动创建发送者 AVAILABLE FileEntry。新中继暂存须将其标为投递所有，或推迟创建普通文件库 entry；拒绝/取消/到期一并释放投递所有的 entry。只删 transfer.entry_id 或减一个计数，会留下隐式引用。

账本是权威；ref_count 可为同事务维护的缓存，但须可校验和修复。释放通过条件状态变更（released_at IS NULL），按实际受影响行数调整计数，重复请求/事件不能重复减计数。引用按数据库时间到期，不能因异步扫描尚未执行而续权。

## 3. 原子终止事务

拒绝、取消和过期共用终止操作，同一次 PostgreSQL 提交包含：

1. 校验操作者、expected version、允许转换及原始期限。
2. 更新业务终态和版本；撤销本次 grant、增加 revocation_epoch。
3. 幂等释放本次投递引用及投递所有 FileEntry；中止其上传/下载任务、租约续期。
4. 写 Outbox：停止会话、通知状态、GC 候选事件。

事务成功后访问失效；对象物理删除是另一项可重试工作，不能与数据库假装为一个原子操作。消息延迟不能绕过权威状态检查。信令、manifest/envelope、下行分块和拉取入口均校验当前 grant、期限、epoch；缓存订阅撤权并关闭对应通道。

未接受前不提供内容密钥 envelope、分块或对象读取 URL。长效无状态预签名 URL 不满足即时撤权，此路径应采用校验 grant 的分块服务/代理；如采用短效 URL，须明确其撤权窗口。合法取得的字节和密钥无法远程收回。

## 4. 并发与 GC

附加/释放引用、创建/续期有效 I/O 租约、Keep、封存、选取 GC 的所有路径，争用同一个对象代际父行锁。基于 READ COMMITTED，先锁父行，再查询引用/租约并重新校验，不在锁前旧快照上判零。[PostgreSQL 行锁与锁顺序](https://www.postgresql.org/docs/17/explicit-locking.html)提供事务互斥语义；以下生命周期为本项目设计选择。

全局锁顺序：对象代际（多对象按 ID 排序）→ 传输（多传输排序）→ 引用 / grant / 任务 / 租约。调用先读关联 ID 时，取锁后重读并校验关联与版本，必要时回滚重试。尚无 entry 的上行也遵守同一协议。事务内不执行对象存储网络 I/O。

GC 分四步：

1. 父行锁内检查：已过既有宽限期、无有效引用、无保留锁、无有效读写/提升租约。
2. CAS 为 DELETING，生成稳定 delete_token，固定该代际对象键清单，写 Outbox 并提交。此后禁止该代际新增引用、Keep、写续租和封存。
3. Worker 幂等删除固定清单；失败保留 tombstone 并重试，不恢复访问权。
4. 完成事务仅接受匹配 generation + delete_token。配额释放、COLLECTED 事件只记一次；物理回收未确认前不虚减占用。

对象键含不可重用的存储代际，例如 versions/{version_id}/generations/{generation_id}/chunks/...。新上传不复用已进入删除状态的代际或旧键，即使内容相同。迟到旧 GC 因而只能删旧代际。

租约过期不代表旧 PUT 已停止：上传先登记写意图，PUT 后再次验证代际、write_epoch、授权，才挂接分块。拒绝/到期/GC 已获胜时，迟到 PUT 不得成为有效分块；由同代际清理与 orphan sweep 回收。删除记录保留扫描信息，覆盖“删除完成后旧 PUT 才成功”。数据库 epoch 无法阻止存储端已在飞行的写入，须代际隔离并回收孤儿。

Keep 与 GC 同锁竞争：Keep 先合法提交则建立独立引用/保留锁，阻止 GC；GC 已 tombstone 则 Keep 失败，不复活对象。用户重新提供文件时建新代际。拒绝已赢得的分享也不能再授予 Keep。

## 5. 过期时钟保持不变

- 完整 TEMP_24H：封存起 24 小时；未完成副本：最后活动 24 小时后回收。终止请求不能被迟到活动刷新。
- 请求沿用 ONLINE 10 分钟、ASYNC/BROADCAST 24 小时的独立期限；选 RELAY 不切换意图。
- Accept、重试、路径切换、恢复不延长原期限；每次授权检查数据库时间。
- 请求失效会提前结束本次投递访问和引用，即使对象保留期未到；其他独立引用适用各自期限。
- 既有文件过期 GC 宽限期保留。拒绝立即成为 GC 候选，物理回收仍受引用、租约及重试规则约束。

投递完成后，若允许重复恢复，应建立历史契约中的独立可撤销副本引用/grant；首次回执不是永久引用或访问权。不保留可复用副本的投递完成后释放暂存引用。未接受的上传计入发送者空间/并发配额，受会话期限和限速约束。

## 6. UI 与进度

云端阶段主图展示当前一段：上传时 This device → Cloud，从云端接收时 Cloud → This device。直连和在线中继主图均为两台设备，蓝线跟随同一接收端账本。上行100%不冒充送达；切换确认依据须明确改变阶段/metric身份，不将不同进度补间成一次倒退。详情显示当前指标及独立云副本状态。

沿用蓝色前缀、灰色余量、1600ms 流光（前缀内裁切，最长整条路径6%）。暂停80ms消退，继续120ms淡入；未知总量、陈旧样本、后台、校验、终态停用。边界最多180ms插值，数字即时更新。

- 封存：停止流光80ms，曲线180ms退出；Cloud时钟表示等待回复，不显示Delivered。
- 同意：事务确认后锁徽标120ms退出，下行连接180ms进入。已接受但尚无分块时显示等待，不伪造增长。
- 拒绝/到期：权威终态立即停流光，曲线120ms退出；历史保留，无Restore。重新发送创建新请求和授权。
- 收完字节：静态校验状态；校验、落盘、回执后才Received。
- reduced-motion：即时状态切换，无流光、位移、缩放。

建议 relay_projection 携带 transfer version、progress revision、grant epoch、object_generation、各段 acknowledged bytes/total、spool state、decision、sample time、原始 deadlines。客户端拒绝过期版本、错误代际和终态后样本。Outbox 负责业务转移；高频进度沿用原有节流通道。

## 7. 源码核对与改动范围

以下表格保留实施前的差距记录；SIM-847已按[当前实现](cloud-storage-authority.md)处理这些路径，不应再据此判断现有源码。

| 位置 | 现状与必要调整 |
|---|---|
| backend/internal/service/p2p.go / AddSignal | 所有通道要求ACCEPTED。上行改为独立purpose-scoped能力；下行仍要求同意，不能信任客户端自报Role。 |
| cli/internal/p2p/transfer.go / RunSender | 先等接受才开始，发完仍等送达。中继须支持PENDING上行，以durable seal独立结束连接。 |
| backend/internal/service/transfer.go / RespondTransfer | 接受绑定entry时要求AVAILABLE；拒绝/取消未释放完整云引用。须支持封存前同意及统一原子终止。 |
| backend/internal/service/relay.go | frames整文件驻留内存；finish即使封存失败仍置uploadDone，绑定/envelope错误处理不完整；DONE绑定双向完成。改为有界缓存+持久spool，成功封存后确认，分离上行/送达。 |
| backend/internal/service/p2p.go / SetTransferEntry | 独立更新指针，缺少终态/代际锁协议。封存须原子绑定，拒绝/到期后不能迟到挂接。 |
| backend/internal/service/upload.go | 隐式sender entry；PUT后登记前未重查终态；重开collected版本，对象键按version/index/hash。须投递所有权、写意图/fencing、独立不可重用存储代际。 |
| backend/internal/service/maintenance.go | 有零AVAILABLE entry/persistent_hold检查；提交后删存储，再提交COLLECTED；请求过期未释放完整引用。须完整引用/租约模型、代际/token、幂等配额和重试。 |
| backend/internal/service/manifests.go / pull.go | 非owner要求ACCEPTED。保留同意检查并增加grant/epoch；完成后恢复使用独立授权。 |

## 8. 实施验收条件

1. 接收端未打开时可完成上行并退出；之后接受可完整接收，hash匹配。未接受者拿不到分块/envelope/可绕过授权的URL，伪造Role不开下行。
2. 封存前可同意并接收持久分块；上传失败不伪报Stored/Delivered。
3. Reject vs PUT/seal/entry绑定的每种顺序，不复活请求、不留AVAILABLE投递entry或可读副本；迟到对象最终回收。
4. 两条有效引用拒绝一条，另一条仍可读；最后引用及租约/保留锁释放后才物理删除。
5. 重复拒绝/过期、双GC worker、删除失败后重启，不负计数、不重复释放配额、不影响其他分享。
6. Keep vs GC两种先后顺序均确定；旧GC/PUT不影响新代际。
7. Accept/重试/路径切换不续期；过期边界的迟到事件返回权威快照。
8. 客户端区分上传100%、等待接受、下载100%、校验和回执；暂停及reduced-motion不假装传输。

设计文件：designs/19-pinelink-relay-consent.fig。新增16 / Relay before consent与17 / Atomic lifecycle，保留v18其他页面。
