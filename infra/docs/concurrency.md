# 并发与生命周期约束

SIM-854审查采用可控延迟、重复建立/清理、并发首次打开、乱序响应与真实数据库并发。React入口全树启用StrictMode，开发时额外执行render、effect setup/cleanup；生产构建不增加这些重放。参考[React官方说明](https://react.dev/reference/react/StrictMode)。StrictMode不是锁，也不能证明后端原子性。

## 客户端生命周期

- 初始化按服务器、owner、设备选择与公钥pin单次并发执行；不同配置不能覆盖已有会话。失败可重试，关闭期间不启动新会话。打开失败释放数据库。
- React effect拥有worker的start/stop，数据库与前台请求由会话拥有。stop可重启；close终结会话，中止I/O、等待已接纳的前台请求与同步，再关闭数据库。关闭后的迟到成功不能回写缓存或返回成功。
- 每次worker启动使用代次；旧循环即使在stop→start后完成，也不能再安排定时器。监听器随stop解除。浏览器history在React外构造，热更新时销毁，避免StrictMode重复state initializer遗留全局监听器。
- Web同源标签页用Web Locks互斥同步。后台可跳过忙锁；前台必须等待，包含恰好加入了已跳过的后台尝试这一情况。关闭取消尚未获得的锁。无Web Locks环境仅保证实例内合并；持久事务与服务端CAS仍保护数据，不承诺跨上下文只有一个HTTP worker。
- Flutter API的懒初始化合并、失败后可重试，dispose/close后禁止再次初始化。HTTP采用AbortableRequest与覆盖签名/请求/响应体的总期限；超时中止底层请求。参考[http官方API](https://pub.dev/documentation/http/latest/http/AbortableRequest-class.html)。页面更换API时释放旧订阅，旧操作回调不更新新API页面；文件选择器返回时检查mounted。

## 缓存与复制提交

以前用毫秒时间比较请求先后，同一毫秒和时钟回拨都会失效。现在读取开始时在数据库事务中分配严格递增序号；成功控制在同一事务中推进屏障并更新已有实体，保留更高实体版本。旧读不能越过屏障，两个连接也不会获得相同序号。旧格式毫秒值仅用于初始化单调序号，之后不依赖墙上时间的顺序。

读取的响应不能覆盖后发读取；被控制操作淘汰的首个读取没有缓存可返回时，最多重新读取一次，持续冲突报RESPONSE_SUPERSEDED。不自动重试POST。React控制动作在点击时固定id/version，缓存回调从当前列表合并并比较版本，避免旧闭包替换整个列表。已完成更高版本不能被迟到确认回退。

Web reset通过目标行读改写事务保留并发字段；Flutter读改写事务先取得SQLite写入权，避免不同连接以旧快照升级写入。Web IndexedDB事务、Flutter inbox/投影/游标事务及提交时撤权复核保留；HTTP缓存不能成为文件校验/回执的权威。

## 后端与原生核心

服务端依靠PostgreSQL行锁、expected_version、稳定操作ID和不可变事件摘要；复制feed写入顺序、迟到outbox投影、引用/撤权/GC代次均有真实PG测试。新增12路相同上传重试与并发roster CAS测试。原生同一发送由OS锁合并，SQLite inbox/投影/游标原子提交；Go race detector覆盖服务层、HTTP鉴权、LAN核心和协议。

CLI发送方在协商前监听data channel关闭/错误及ICE失败；协商等待和发送、checkpoint等待共用错误信号。中继存储失败关闭通道，即使ICE仍连接也会退出，进度查询单次最多6秒，不能一直等到文件过期。接收端串行写入，完整前缀之后忽略迟到帧，保持连接直到明文校验与回执提交完成；校验失败不当作直连失败重走中继。发送端遇到终止信号再核对一次权威checkpoint，避免提交完成与通道关闭同时发生时误报失败。真实WebRTC双端测试覆盖仅关闭数据通道的情况，以及协商前已经进入Closing的情况。

进度采样worker关闭后不再处理排队tick；显式Flush的HTTP请求同时受调用者与worker生命周期取消。Close不能仅取消定时上报而留下Flush继续发送。对应测试覆盖进行中的定时请求和显式Flush两种关闭交错。

```powershell
./infra/scripts/test-concurrency.ps1 -DatabaseUrl '<独立测试PostgreSQL DSN>'
cd web
npm test
npm run build
cd ../mobile
../infra/scripts/flutter.ps1 analyze
../infra/scripts/flutter.ps1 test
```

PostgreSQL测试创建各自schema并清理，不清空共享库。SQLite多连接测试使用独立后台executor，避免同一Dart事件循环同步阻塞另一个持锁连接。真实浏览器冒烟包括两个标签页首次打开时持锁/排队、仅一次注册/绑定、关闭其中一页后另一页重开，以及Web/Flutter离线重开和自动补齐。

本轮staging磁盘耗尽曾使SeaweedFS卷只读、真实文件测试失败；清理无引用的可重建构建缓存并重启本项目存储服务后恢复，未删除数据库或数据卷。Go race脚本将编译/module临时数据绑定到构建机另一分区；Flutter集成测试为自己启动的CLI设置90秒期限并终止超时子进程。故障记录保留，恢复后重新验证真实文件路径。

检查覆盖可复现的关键交错，不等于所有竞态的形式化证明。Android/iOS后台、系统强制终止、磁盘故障及未来Web/Flutter字节数据面仍需对应平台验收。
