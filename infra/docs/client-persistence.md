# Web / Flutter持久化与复制消费

SIM-852承接SIM-850的三方复制，接入既有客户端的数据读取；不在UI缓存中重建接收状态机。页面布局和新文件数据面属于后续v20单元。

## 身份与库

Web使用Dexie 4.4.7 / IndexedDB，Ed25519及X25519私钥为不可导出的WebCrypto CryptoKey，首次并发打开只保留先提交的身份。Flutter使用Drift 2.35.3 / SQLite，原生WAL/FULL，私钥由flutter_secure_storage提供的平台vault保存；Flutter Web使用同源SQLite wasm/OPFS或IndexedDB。两者按服务器、owner和显式设备选择隔离库，缺省生成稳定设备ID。重开不重置身份或stream；库已有身份但vault丢失时停止，不悄悄新建身份。

Flutter按用户要求使用官方最新stable，当前固定3.47.9 / Dart 3.13.7，官方发布commit `c197ca5199e9ee5661e865596c9c07294dfab51c`。`.fvmrc`及项目独立SDK脚本避免依赖机器上的旧SDK。SDK升级后的图像基线已逐项检查；差异是Flutter边缘栅格化，未调整页面结构。

## 消费与缓存

签名info、设备注册/绑定、版本化配对名单、快照与delta遵循共享协议。Go工具每次用临时私钥生成互通向量，只输入/输出公钥和密文，不保存测试私钥；Web和Flutter均验证Go签名、age元数据及中文/JSON转义。客户端包只应用已确认设备的事实，提交时再次检查撤权；inbox、投影和游标同事务。旧快照/响应不回退位置，重复ID不同内容拒绝。Web兼容的数值范围为安全整数，超界在提交前拒绝，不舍入。

HTTP资源缓存与已验证的历史事实分开存储。任务、通知、传输、文件快照、设备等既有读取接入缓存；网络失败时可回读，明确401/403及其他非重试错误会继续报错。成功控制的返回值同时更新已有资源缓存，迟到的旧读取不能覆盖它。离线操作不伪造暂停/接收/完成成功。

启动和恢复连接后有界消费快照/delta并刷新已读资源，退避2–60秒持久化；Web在online/pageshow/可见性恢复时唤醒，Flutter在resume时唤醒，原生测试也覆盖无UI生命周期的自动worker。Flutter以持久资源变化通知当前页面刷新，避免必须重新进入页面。关闭中止请求、拒绝迟到成功并等待前台及同步操作，再关闭数据库。SIM-854补充StrictMode重放、单调缓存序号、双连接事务与锁等待；详见[并发约束](concurrency.md)。

## 离线资源与运维

Web由Workbox预缓存应用外壳；SIM-857进一步补齐可安装manifest、schema2资源预取/公平重试、离线查询、跨标签缓存通知及有界后台同步，见[Web PWA与完整离线目标](web-offline-pwa.md)。Flutter构建使用本地web资源，SQLite wasm及worker同源部署，独立SW按完整版本预缓存。API从不进入service worker缓存，升级不强制重载正在使用的客户端。首次访问需要联网，浏览器清理站点数据或隐私模式不作持久保证。

staging提供独立HTTPS端口18682（Web）、18683（Flutter Web），同源`/api`代理到18680。自签证书只用于staging，未改系统信任库。COOP/COEP允许SQLite OPFS；没有依赖第三方CDN。生产/iOS未发布。

```powershell
./infra/scripts/setup-flutter.ps1
cd mobile
../infra/scripts/flutter.ps1 pub get
../infra/scripts/flutter.ps1 analyze
../infra/scripts/flutter.ps1 test
```

真实链路测试可设置`PINELINK_STAGING_API`后运行`test/persistence_staging_test.dart`。`build-clients.ps1`构建并校验SQLite wasm；`deploy-clients.ps1 -Version <HEAD>`部署已提交版本。原生核心仍独占字节接收/回执账本；本单元不把Flutter Web测试当作Android/iOS后台或端到端文件数据面的验收。

[Dexie事务](https://dexie.org/docs/Tutorial/Best-Practices)、[Drift平台配置](https://drift.simonbinder.eu/platforms/)、[Flutter官方归档](https://docs.flutter.dev/install/archive)、[age TypeScript实现](https://github.com/FiloSottile/typage)、[dartage](https://pub.dev/packages/dartage)。
