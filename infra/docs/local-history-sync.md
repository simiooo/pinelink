# 本地历史与三方增量同步

2026-11-01，根据客户端完全离线与请求历史持久化要求，将发送端、接收端和服务器视为三个持有不同权威事实的副本。SQLite是客户端数据库，不是服务器的缩小镜像。

## 存储选择与边界

- Go原生核心/CLI：已使用`database/sql`与`modernc.org/sqlite v1.61.1`，无CGO依赖，当前工具链Go 1.26。每个稳定设备profile一份本地`state.sqlite`，WAL、`synchronous=FULL`、外键及有界busy timeout。
- Flutter：Drift是后续适配候选，负责类型化查询、迁移与事务。复用Go核心时，由核心独占业务写入，Flutter通过SDK消费投影；不能让Drift和Go分别推进同一条权威状态机。
- Web：Dexie封装IndexedDB，是后续适配候选；业务投影与outbox放同一个读写事务，网络请求在事务外执行。React Query继续管理内存中的网络请求状态。应用资源需预缓存；浏览器存储额度、清理与隐私模式另行验收，不能承诺首次访问离线可用。

[modernc官方文档](https://pkg.go.dev/modernc.org/sqlite)、[Drift事务迁移](https://drift.simonbinder.eu/migrations/api/)、[Dexie事务](https://dexie.org/docs/Dexie/Dexie.transaction%28%29)。

## 当前原生数据库

`database_meta`保存设备公钥ID、数据库世代`stream_id`和迁移状态；`records`保存可查询的类型、设备、状态版本、进度序号和JSON投影；`change_log`是仅追加的业务日志；`sync_outbox`与业务投影同事务写入；`sync_inbox`去重已经验证的接收回执；`sync_cursors`预留各目的端的消费位置。

事件使用随机`event_id`，游标为`source_device_id + stream_id + sequence`。序号来自数据库提交顺序，时间戳只用于展示。重启保留stream和游标；数据库重建后生成新stream，旧游标不能直接跳过新日志。初次接入读取快照再接增量；后续压缩需保留未确认outbox、终态事实和最小可用游标，不直接删除历史后继续沿用旧游标。

初始请求、接受、拒绝、暂停、取消、过期、校验及送达状态写业务日志；逐块字节确认只更新当前checkpoint，避免生成庞大的同步日志。日志不包含内容密钥、密钥信封、会话或邀请秘密。文件内容留在受控文件目录；私钥不进入数据库。

同一事务更新投影、追加业务日志和插入outbox；任一步失败全部回滚。签名回执验证后，把inbox去重与本地完成事实一起提交。同一事实ID不同payload hash拒绝，重复相同回执不重复应用。旧版本和较小确认量不能覆盖新checkpoint；接收端信任检查与分块/完成checkpoint同事务，撤权提交后不能产生更晚的进度确认。

JSON原型记录首次打开时导入一次并保留原文件，后续只读SQLite，避免旧文件恢复已撤销的信任。当前schema为2，增加稳定密文publication、各目的端状态、复制inbox/history与对端上传游标；更高版本拒绝由旧客户端打开。

## 三方复制契约

发送端拥有发送意图和本地尝试记录；接收端拥有接受/拒绝、持久接收量、校验与已签名送达；服务器拥有云对象封存、授权、期限、配额和引用回收。相同字段的权威来源明确后按聚合版本应用，不做通用最后写入胜出或CRDT合并。

传输ID与控制域一起索引。本地送达事实不能直接当作中心服务创建过的传输；云端接入先验证设备公钥归属、共享范围、签名和关联ID。文件名等私密历史元数据在离开设备前按账户/会话加密；当前本地增量接口不是可直接上传的公开payload。

同步worker按已授权的目的端投递outbox，确认收到后才推进该端游标；超时重试相同event ID。接收端在事务内完成inbox去重、版本校验和投影更新，然后返回确认。删除历史以本人的视图墓碑表达，不等价于撤销另一方的送达事实或删除云对象；云权限始终由服务器当前grant裁决。

登录/重新联网不应重置本地设备身份、日志世代或历史。应用账户绑定与离线身份分离；CLI的owner/device是显式profile选择，切换它们会选择另一份设备身份。已送达后的历史同步失败不回退送达状态，云暂存成功也不能补造接收回执。

## 已实现与后续

原生SQLite事务、业务日志和已验证回执由SIM-848建立。SIM-850增加三方云端delta上传/拉取、签名info/绑定、双方配对共享名单、固定上界快照、稳定密文、原子消费游标、持久退避，以及离线发送意图恢复。`UNBOUND`表示尚未生成稳定publication，`PREPARED`表示已生成，只有验证云端ACK后才记为`CLOUD_ACK`；对端ACK不冒充云确认。

`lan history`、`lan changes`、`lan storage`保留本机诊断；`lan sync bind/status/once/run/history/disable`负责复制。设备公钥绑定是精确设备授权，不能替代未来账户登录/成员管理。详见[复制契约](history-replication.md)。Web Dexie、Flutter Drift及应用资源离线适配紧接实施；当前原生结果不等价于跨端产品全部完成。
