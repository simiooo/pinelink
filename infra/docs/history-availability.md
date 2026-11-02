# 历史可用性与云端恢复契约

2026-11-01，OpenPencil 17 版设计阶段修订；尚未接入应用代码或部署。继承 [真实进度契约](transfer-progress.md)。

v19 增补 [中继预上传与引用安全回收](relay-preupload.md)：未接受的投递可先上传，但不能读取。拒绝/到期保留历史并撤销本次授权，释放投递暂存引用；其他合法副本引用不受影响。

## 记录与副本

传输完成、文件副本可用、当前用户可重新使用，是三个独立事实。历史记录保留原传输事实，不随副本过期删除，也不因一次恢复覆盖首次交付时间。

本轮“一次性”按无可从历史复用的留存副本设计；不是下载次数限制。未来单次下载链接、剩余使用次数或按次购买若要引入，需独立授权策略，不能复用上传侧 `reuse_tokens` 或把现有一次性标签当成已实现的次数限制。

| 界面状态 | 条件 | 合适的下一步 |
|---|---|---|
| Reusable | 有有效访问授权及 SEALED/AVAILABLE 云副本，或当前端验证过的本地文件引用 | 云副本 Restore；本地文件 Send again / Open；按能力显示 Keep available |
| One-time | 历史存在，没有被留存为可复用来源的副本 | 查看记录；Choose files 新建传输 |
| Expired | 曾有留存副本，副本已过期 | 保留记录；Choose files；不提供 Restore 或延长已失效副本 |

权限被撤销、文件被删除、本地路径失效等属于 Unavailable 原因，不伪装成 Expired。可复用性必须由权威状态与当前访问权共同决定，不能只看 `Task.COMPLETED` 或过去的 DeliveryReceipt。

`TEMP_24H` 在有效期内也可以 Reusable；`PERSISTENT` 表示保留策略，不保证访问权永久有效。恢复不延长保留期。18 版文件网格仅保留图标与名称，以 16px 徽标表达可用性与来源；悬停、键盘聚焦或手机详情提供含义。具体期限、原因和操作留在详情 modal / bottom sheet。

## 当前架构需要调整的地方

`FileSnapshot` 已提供 AVAILABLE/EXPIRED/DELETED、retention_class、expires_at 和 version；`PromoteFile` 也验证有效期、SEALED、所有权及配额。这些作为副本事实继续复用。

`CreatePull` 对非所有者目前要求关联传输仍为 ACCEPTED。若传输已完成，收件人不能据此自然获得下一次恢复授权。不能简单把所有 COMPLETED 传输都放开，也不能把曾经收到文件等同于仍有访问权。

需要独立、可撤销、有限期的副本读取 grant，与传输请求状态分开。所有者本人按当前所有权校验；收件人按明确授予的可复用范围读取。每次恢复创建新的 PullSession，检查副本状态、grant、设备身份和可解密的 manifest envelope。grant 被撤销或过期时取消能力；历史仍可按参与者权限读取。

历史投影应组合记录、当前副本、当前授权及端侧本地引用，返回版本化能力，不将 UI 的 Reusable/One-time/Expired 变成另一个可以与数据库矛盾的持久状态机。

示意字段（未实现 API）：

```json
{
  "record_id": "record-uuid",
  "transfer_id": "transfer-uuid",
  "file_reference": "entry-or-local-reference",
  "reuse_state": "REUSABLE",
  "source": "CLOUD",
  "retention_class": "TEMP_24H",
  "expires_at": "2026-11-02T08:00:00Z",
  "availability_version": 3,
  "capabilities": {"restore": true, "resend": false, "promote": true}
}
```

文件名从端侧已解密 manifest / 本地历史元数据展示，不为了渲染网格扩大服务端明文元数据。来源为 LOCAL 时重新检查本地引用、权限与文件身份；文件丢失或变化时请用户重选，不用旧路径冒充可复用文件。

## 云节点

Cloud 是当前恢复操作的逻辑来源，使用浅灰圆节点和云轮廓图标，尺寸沿用普通设备节点（PC 64 / 手机 56），底部本机仍为 96 / 80。云节点不注册成 peer device，不展示在线状态点，也不宣称云端可解密内容。

点击有效历史项的 Restore 后，检查授权并创建 PullSession，再进入 Cloud → This device。蓝色弧长从云端向本机增长，灰色表示剩余确认字节；已确认蓝色范围内部使用已采纳的 1600ms 柔和流光。暂停保留弧长并停止流光，详情仍是同一 modal / bottom sheet。

100% 后显示 Verifying file；完成本机解密、内容校验及文件提交之后显示 Restored，勾显示在本机。任务完成消息不能早于本机校验，不能仅凭密文字节下载完毕显示成功。当前 CLI 的 `downloadToFile → finalizeDownload → CompletePull` 顺序可继续保留。

副本过期、被删除、设备或 grant 撤销时，恢复失败反馈具体原因，蓝线不继续增长；失败详情允许返回记录或选择本地文件。新发起的恢复没有权限时不生成可用云节点。活动 PullSession 的 lease、到期处理与 GC 协调应有明确服务端策略，界面不能用本机倒计时擅自删除副本或延长访问。

多次恢复保存独立 PullSession/操作记录；首次 DeliveryReceipt 不重写。文件网格可按不可变内容引用聚合，详情保留每次传输/恢复的审计事实，不用聚合覆盖历史。

## 保留与未来商业化

有效临时副本的所有者可从详情“更多”选择 Keep available，明确确认后复用 PromoteFile。后端在同一权威事务中检查身份、版本、有效期、SEALED、配额及 entitlement。仅在能力允许时显示该动作；已经持久保留时显示 Kept available。

未来套餐、保留时长、额度或单次使用权从策略层决定 capabilities；当前设计不硬编码价格、套餐或付费拦截。缺失或已过期的字节不能因为购买权益而承诺可恢复。提升成功前不提前移除到期信息；失败保持原有期限并返回简短原因与下一步。

Remove record 与 Delete cloud copy 必须区分；前者只改可见历史，后者影响后续恢复，需要明确确认及权限检查。云端副本删除不能悄悄清空过去的交付事实。

## 后续实施验收

覆盖完成后的收件人重复拉取授权、grant 撤销、临时副本到期/提升、配额不足、本地引用失效、恢复中 GC/expiry 竞争、解密失败、首次回执不变以及历史删除和副本删除的隔离。完成实现后按项目约定推送、实际部署 staging 并用双端真实文件冒烟；静态设计与演示不是部署证据。
