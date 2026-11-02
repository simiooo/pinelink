# Web PWA与完整离线目标

2026-11-02用户要求Web支持PWA、完整离线应用与恢复联网后自动同步。完整目标包括本地数据、待同步操作和本地文件，不以首页或只读缓存代替完整离线传输产品。

## SIM-857工程范围

本单元补齐可安装PWA及**现有页面的离线读取与恢复同步**，沿用已验收v20标记，没有新增未经验收的产品界面。浏览器文件数据面、离线写入outbox和待同步交互仍待后续单元，不在本单元宣称完成。

- manifest提供稳定id/scope、standalone、192/512px图标、maskable及Apple图标。图标直接取自OpenPencil v20页头已验收的线性标记；`build-pwa-icons.mjs`从对应SVG提取原路径，不沿用旧松鼠favicon。
- Workbox injectManifest预缓存整个应用资源，所有既有页面可离线冷启动与路由导航。API、文件内容与version.json不进入外壳缓存；HTTP API请求也使用no-store。
- Dexie schema2升级保留身份、日志和游标。启动即预取任务、通知、历史、设备、容量和任务引用详情，不要求先逐页访问。尚未取得的数据保留pending标记，不返回伪造的空成功。
- 浏览器离线提示不阻止本地查询；已知缓存可立即读取。navigator.onLine不是网络权限判断，仍允许尝试实际可达的局域网/自托管服务。
- 资源按持久checked轮转，单项失败不阻塞其他项；网络错误保留最后成功值及待重试标记。服务器明确401/403/404/410后，保存失效标记，断网重开也不把旧值当作可用。
- API缓存内容修订通过Dexie通知活跃页面和其他标签页，仅内容变化触发查询刷新，时间戳刷新不造成循环。
- 页面打开、online/pageshow/前台恢复与有界轮询消费验签后的快照/delta。支持Background Sync时，同一个不可导出身份、Web Lock和原子游标可在Service Worker内补同步；每个事件最多3轮、轮间检查20秒预算，保留sweep位置供OS重试。不复制待重放的过期HTTP签名，不在后台补造身份。
- 新SW等待旧窗口全部关闭后激活；不skipWaiting、不clientsClaim、不强制重载传输中的页面。安装后申请浏览器持久存储，失败不阻止使用。

## 完整离线产品尚需交付

1. 在Web文件数据面中持久保存草稿、待发送文件及加密分块、已下载文件和真实checkpoint；容量不足或文件权限失效时保留明确状态，不只保存不可恢复的File对象引用。
2. 本地意图与服务器状态分离。outbox同事务记录操作ID、服务/身份、聚合ID、预期版本及本地顺序；离线显示“待同步”，确认回执后才显示服务器已执行。允许取消尚未发送的操作。
3. 服务端以同一操作ID原子去重并返回稳定结果；网络响应丢失可查询回执。冲突、过期、拒收和撤权不以自动重试覆盖最新状态，不凭相同目标状态猜测某次操作成功。
4. 恢复联网后先校对授权/状态，再有界上行outbox、下行delta并合并投影；文件任务从真实字节checkpoint恢复，完成仍需校验及接收回执。
5. 新增“本机已保存/待同步/冲突/空间不足”等交互先在OpenPencil完成设计与验收，再接入运行客户端。迁移到自定义服务时，前台与SW都必须按活动profile及信任代次隔离并排空旧任务，见[服务模式](service-modes.md)。

这些步骤属于v20完整Web文件数据面交付条件，保留在整体实施队列中。现有页面的离线远端控制会明确失败，不悄悄放进React Query内存队列，更不伪造暂停/接受/完成成功。

## 浏览器边界与验收

首次备齐资源需要联网。用户清除站点数据、无痕模式或浏览器拒绝持久存储时不能承诺永久保留。Background Sync并非所有主流浏览器支持，OS也可能终止或推迟后台任务；可靠的通用途径是前台恢复或重开后继续。PWA安装不会额外获得原生UDP发现或无条件后台大文件传输权限，不以PWA标签替代原生离线LAN数据面的验收。

单元检查覆盖v1迁移、离线查询、坏资源不饿死其他资源、拒绝/过期不复活、跨连接通知、后台身份与有界sweep重启。实际浏览器脚本：

```powershell
node infra/scripts/smoke-web-pwa.mjs <deployed-commit>
node infra/scripts/smoke-client-persistence.mjs <deployed-commit>
```

PWA脚本使用实际staging产物、真实签名API和加密CLI文件；loopback代理只注入网络故障和SW版本标记，验证未访问页面离线打开、无online事件的服务恢复、无应用页的后台签名消费、多窗口安全升级、浏览器离线重开、联网无刷新补齐，并检查实际HTTPS部署的安装条件。CDP触发真实sync事件只证明该处理路径，不能宣称OS在所有平台均会按时唤醒。

当前staging IP使用自签证书，Chrome的安装检查报告`not-from-secure-origin`，即使测试忽略证书错误也不能当作正常安装通过。研发验收可运行`node infra/scripts/preview-pwa-staging.mjs <deployed-commit>`，通过`http://localhost:18684`访问实际部署的页面与同源API。桥接只监听127.0.0.1，以现有staging公开证书作为独立TLS信任根，保留证书/主机名检查；不改系统信任库，也不转发其他localhost应用的cookie或bearer凭据。正式对外入口仍需有效域名与受信TLS。运行PWA冒烟前设置`PINELINK_PWA_PREVIEW_URL=http://localhost:18684`；原始自签入口的安装限制与本机预览的零安装错误分别记录，不混报。

参考：[Vite injectManifest](https://vite-pwa-org.netlify.app/guide/inject-manifest)、[Workbox预缓存](https://developer.chrome.com/docs/workbox/modules/workbox-precaching)、[安装条件](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Guides/Making_PWAs_installable)、[Background Sync兼容性](https://developer.mozilla.org/en-US/docs/Web/API/Background_Synchronization_API)、[持久存储请求](https://developer.mozilla.org/en-US/docs/Web/API/StorageManager/persist)。
