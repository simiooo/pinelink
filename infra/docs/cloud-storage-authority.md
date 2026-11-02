# 云预上传、授权与回收：实施映射

SIM-847 · 2026-11-01 · 对应已验收OpenPencil v20。此单元交付后端、CLI及staging验证；Web/Flutter新界面另行实施。

发送者显式选择 `p2p send --path relay` 时，PENDING即可上传密文。发送命令以持久分块校验、seal、entry/envelope绑定事务成功为Stored，并返回独立的 `stored` / `delivered` 字段。之后发送进程可以退出。直连和AUTO仍先等待同意，AUTO只在直连失败后走中继，不暗改ONLINE意图或期限。

接收者Accept后才取得下行、manifest和内容权限。在线中继读取已登记的持久分块，不等待整文件封存；离线发送端的接收与再次恢复使用同一持久spool。最终Delivered仍须接收端密文校验、解密、明文校验、本地提交与回执。

| 权威数据 | 当前实现 |
|---|---|
| `relay_spools` | 在首块前绑定transfer、upload、不可变version；seal事务绑定entry |
| `object_references` | UPLOAD pin、LIBRARY、DELIVERY、PERSISTENT_KEEP；引用来源键唯一，释放不可复活 |
| `file_access_grants` | DELIVERY要求ACCEPTED及原请求期限；RESTORE仅在真实完成回执后签发，按文件原期限到期 |
| `transfer_requests.revocation_epoch` | Reject/Cancel/Expire与撤权、投递entry释放、任务取消在同一事务更新 |
| `file_versions.storage_generation` | 唯一存储代际；COLLECTED后新上传建立新version，不改写历史root或复用旧键 |
| `object_io_operations` | PUT前持久写意图；45秒租约；PUT后重验状态、代际、epoch和任务；30秒单次对象I/O |
| `object_gc_jobs` / `object_gc_objects` | 固定清单、token与worker claim；分页删除及持久checkpoint；全部确认后仅释放一次配额 |

所有附加/释放引用、seal、Keep、租约和GC选取均遵守：对象父行（排序）→transfer→task/reference/pull。关联在取锁后重读；事务中不执行对象存储网络I/O。下载任务控制与完成均按task→pull排序。

Reject立即撤销本次投递权限并释放DELIVERY引用；私有库与明确Keep不随之删除。文件已被合法接收的字节不能远程收回。HTTP按64KiB块重验访问；中继按持久块读取并在I/O后再验授权。已排入传输层的字节受在途窗口限制。接收前的Get/List/Sync及冲突响应不泄漏内容密钥信封。

`promote ENTRY --transfer TRANSFER` 可在有效同意/恢复授权下建立独立的私有PERSISTENT_KEEP entry，返回新的entry ID。普通库文件仍沿用原有提升行为。原投递删除或过期，不影响独立Keep；Keep不延长原投递期限。

完整TEMP副本仍从封存起24小时；未完成上传按最后活动24小时；ONLINE请求10分钟，ASYNC/BROADCAST请求24小时。Accept、恢复和路径切换不续期。文件自然过期保留15分钟GC宽限；拒收/取消即时成为候选，实际删除仍等待其他引用与租约释放。

中继不保存整文件frames。每次处理64KiB分块，DataChannel发送缓冲阈值256KiB，服务端SCTP接收上限1MiB。seal清单与GC清单分页128项；GC每轮最多128项/约2秒工作预算（在途单次I/O最多30秒），跨worker续做。孤儿写入tombstone每5分钟复查，覆盖DELETE后迟到PUT才成功的情况。

中继物理空间统一计入发送者 `relay-owner:<owner>` 配额，沿用默认1TiB限额；每发送者最多4个活跃上行spool，配额行锁串行裁决。公用prepare接口禁止使用内部spool/配额命名空间。上行与下行可由不同relay实例处理；offer claim和reply_to隔离连接重试，不依赖进程内文件缓存或独占transfer实例。

进度快照增加 `revocation_epoch`、`spool.generation`、`capabilities.restore`。真实上行确认与接收确认各自独立。上传任务进度只来自已登记分块；校验失败显示FAILED，不能显示Stored。无效云副本不撤回已确认的Delivered。

迁移013–017包含引用、grant、代际、I/O、发送者配额及分页GC清单。CLI同时修复了默认一秒HTTP服务超时、完成后合法恢复、空文件加密往返与本地缓存跨身份混用。空密文仅在解密认证后的manifest明确承诺空文件时接受；仅凭key的解密仍拒绝空流。

验证入口：`PINELINK_MIDDLEWARE_HOST=127.0.0.1 infra/scripts/integration.sh` 会在独立随机PostgreSQL schema执行服务级并发测试，再运行真实API/CLI集成。`infra/scripts/smoke-staging.ps1 -Version <commit>` 验证实际staging版本、签名设备双端路径、离线发送端接收、重复恢复、独立Keep及空文件。回执按entry、owner、device去重，P2P与云恢复共用首次交付记录。验收版本与时间见原issue交付记录。
