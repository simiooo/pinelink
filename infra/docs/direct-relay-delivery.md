# 直连、中继与云策略：同一送达契约，独立暂存事实

2026-11-01 · OpenPencil v20设计与架构修订。P2P进度与回执基础已由SIM-844（维护者内部验收记录）实施并部署staging；云预上传与引用生命周期已由SIM-847实现（见[当前映射](cloud-storage-authority.md)）；v20双端界面仍在后续单元，以下完整契约仍包含后续提案。

本修订明确 v19 的适用范围：上行100%仅表示暂存完成，专指云上传阶段。直连不存在必须先完成的云上传阶段。在线中继仍是设备间交付，不应一律把接收设备替换为 Cloud。

## 路径、阶段和保留策略

沿用 ONLINE / ASYNC / BROADCAST 意图及 NONE / TEMP_24H / PERSISTENT 副本策略。传输路径、当前阶段、保留策略分别记录。路径迁移不能静默更改交付意图、保留权限和过期时间。

| 场景 | 主图 | 蓝线确认依据 | 100%之后 | 云副本 |
|---|---|---|---|---|
| DIRECT在线交付 | 本机 ↔ 对端设备 | 接收端可恢复的有效分块确认 | 校验、解密、本地提交、接收回执 | 不要求存在 |
| RELAY在线交付 | 本机 ↔ 对端设备 | 同一个接收端确认账本 | 同上，不以中继收完为准 | 按已声明策略独立暂存/封存 |
| 云上传，等待接收 | 本机 → Cloud | 对象持久写入并登记的唯一分块 | 验证完整清单并封存，显示Stored | 尚未代表对方收到 |
| 云端接收/恢复 | Cloud → 本机 | 接收端对应交付/拉取的有效确认 | 同样校验、本地提交、回执 | 当前grant、引用及期限均须有效 |

中继在线可一边上行一边下行；接收者同意前只允许发送者上行，禁止接收者取得内容/envelope。云下载作为实际数据来源时使用Cloud节点，在线中继使用对端设备节点。单个产品主图仍只有两个节点与一条线，不能为解释内部两段链路添加三节点、多进度仪表盘。

连接方式Direct / Via relay仅在二级详情显示。主图不随ICE候选或常规路径变化增加技术说明。标准TURN网络转发本身也不意味着存在应用云副本；云可用性只能由实际spool封存与授权决定。

## 三类后端事实

- `delivery_progress`：接收端的端到端确认量，是直连与在线中继共享的主进度。
- `spool_progress`：服务端持久分块确认及封存状态，只说明云副本。
- `route`：DIRECT / RELAY / CLOUD_PULL及其epoch、切换状态，是当前数据来源/连接事实，不是完成事实。枚举为拟议投影字段，现有网络路径契约仍保留兼容。

同时区分decision、delivery_state、spool_state。不要用一个RUNNING/COMPLETED枚举同时推导双方送达和云保管。首次DeliveryReceipt继续独立保存；后续PullSession不覆盖它。

示意为同一份快照，针对当前查看者过滤权限与能力：

```json
{
  "transfer_id": "transfer-1",
  "attempt_id": "attempt-1",
  "state_version": 7,
  "progress_revision": 61,
  "decision": "ACCEPTED",
  "route": {"kind": "RELAY", "epoch": 2, "state": "ACTIVE"},
  "phase": "LIVE_DELIVERY",
  "display_metric_id": "delivery:attempt-1:recipient-device",
  "delivery": {
    "state": "RUNNING", "basis": "RECIPIENT_CIPHERTEXT_ACKNOWLEDGED",
    "acknowledged_bytes": 320000000, "total_bytes": 512000000,
    "checkpoint_revision": 61, "sampled_at": "2026-11-01T10:00:00Z",
    "receipt_id": null
  },
  "spool": {
    "state": "SPOOLING", "acknowledged_bytes": 450560000,
    "total_bytes": 512000000, "object_generation": "generation-1",
    "revision": 42, "sampled_at": "2026-11-01T10:00:00Z", "sealed_at": null
  },
  "retention_class": "TEMP_24H",
  "capabilities": {"pause_delivery": true, "cancel_transfer": true}
}
```

所有进度沿用密文总量与唯一有效确认字节，不与明文大小混算。禁止相加、取平均、取两个数的最大值或选择“看起来更快”的进度来代表交付。spool100%只可推进封存；delivery100%只可推进校验/确认。未知总量仍为null。

各进度独立采样时间和revision，不能让spool的新样本刷新停滞delivery的流光。快照原子读取状态、授权、路线、选定进度及完成事实；迟到终态/版本冲突沿用原契约处理。

## DIRECT ↔ RELAY 在线切换

同一transfer、attempt、接收设备和内容身份共用接收端恢复账本。切换仅递增route_epoch，不重置delivery_progress，不制造新送达记录。

1. 分块以稳定内容身份及chunk_index标识；传输通道序号与内容分块身份分开。接收端记录已验证且可恢复的分块/checkpoint；断线重连先核对账本。
2. 新路径只补接收端缺失的数据；重传和来自两条路径的重复块不重复累计。当前顺序追加实现可先沿用连续确认前缀，不能把最远offset当作所有已接收字节。
3. route_epoch排序路径遥测。迟到旧路径事件不能把当前路线切回去；但同一有效attempt内合法、可验证的接收ACK仍可幂等合并，不能仅因旧路径epoch丢掉真实完成的接收数据。
4. 在当前授权/任务状态内更新接收账本、关联任务及progress_revision，状态事务和幂等约束沿用进度契约。拒绝、取消和过期后不得继续推进。
5. DIRECT已交付的前缀可能尚未上传至spool，因此切到RELAY时delivery可以大于spool；两者不能设置通用大小不变量。中继须从发送端补持久副本缺块，接收端仅补自己的缺块。
6. 发送者离线后，只有spool已包含接收端所需的剩余数据且授权仍有效，才能继续交付；只有完整SEALED副本才允许新的离线恢复/Keep。没有可用数据时明确等待/失败，不靠云图标假装能继续。

UI保留对端节点、蓝线边界和当前确认量。只有实际观测中断时才停流光并展示等待/重连徽标；无缝切换且样本新鲜则继续。80ms淡出、120ms恢复，数值不归零、不倒退。路径名称只更新详情。

## 云上传 → 在线交付属于阶段变化

尚未同意时，发送端可显示本机→Cloud及spool_progress。接受后若转为在线交付，目标改为真实接收设备，display_metric_id改为delivery身份；两种确认量不是同一个数。

不能把原云上传蓝线直接从88%补间退到接收端12%。旧线120ms退出，目标节点200ms更换身份，新阶段连接180ms进入；详情明确当前为Sending而非Uploading to cloud，同时保留独立云副本状态。云封存等待阶段原本无活动曲线，接收开始时按新阶段进入。减少动态时直接替换状态和身份。

后续从云端恢复已完成的历史文件创建新PullSession与metric身份；首次交付中途改从spool续传，则继续同一接收账本，不伪造新尝试或丢掉接收前缀。只有文件/账本已丢失、确认量不再可信等实际重新尝试情形，才按既有规则创建新attempt。

## 完成和失败彼此独立

| 事实 | 送达结果 | 云保管结果 |
|---|---|---|
| 直连接收全部字节，尚未校验 | VERIFYING | NOT_REQUIRED |
| 中继上行已完整封存，对端仍未接受 | 尚未送达 | SEALED |
| 中继已收取并校验回执，云封存仍在进行 | DELIVERED | SPOOLING / VERIFYING |
| 有效送达回执已提交，云封存失败 | 仍DELIVERED | FAILED；不提供Restore |
| 云封存有效，接收端校验失败 | 未送达，可按权限重试 | 原有效副本不因接收校验失败自动抹掉 |
| 云副本之后过期/被撤权 | 原送达历史不变 | 当前副本不可用 |

所有路径均以接收端完整验证、解密、本地提交和有效回执为最终完成条件。数据通道DONE只代表传输层/上行握手事实，不得替代应用送达回执。可选spool失败不能使已完成送达倒退；spool尚未封存也不能以delivery回执假报SEALED。

暂停、取消能力按作用范围投影。Pause delivery不能暗中删除云副本；暂停接收时云暂存是否继续由已声明策略决定。全局取消/拒绝/到期仍走v19原子撤权、释放相应引用和GC，不影响其他合法引用。不要把某个接收端DOWNLOAD任务的控制权粗放授权给发送者。

## 实现核对与验收

当前UpdateP2PProgress仅维护transfer.received_bytes且缺少任务/终态同步，projection缺少上述路线/双进度事实；CLI先尝试DIRECT，再通过相同partPath走relay，已有连续分块去重基础。relay把uploadDone与receiverDone绑定DONE，并有封存失败置uploadDone的问题。保留这些可复用基础，实施统一投影和独立确认，不让UI拼凑多个不一致接口。

验收必须覆盖：DIRECT不依赖spool完成；RELAY在线同时上传/接收且两种进度不同；DIRECT62%切RELAY仍62%并仅补缺块；spool落后于DIRECT接收前缀；旧route事件与合法迟到ACK；接收回执成功而spool失败；spool成功而接收校验失败；等待/暂停/后台/减少动态；前置Accept/拒绝/过期与路径迁移竞争。测试真实文件hash、数据库任务/回执、云副本状态及双端画面，不以静态设计检查替代后端验收。

OpenPencil v20新增18 / Direct and relay、19 / Route continuity。v19第16页为云预上传分支，v20已修正原说明板的范围，避免误读为所有中继统一使用Cloud节点。
