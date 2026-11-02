# Pinelink 17 · 历史可用性与云节点恢复

主文件 `../17-pinelink-history-cloud.fig`。保留 16 版全部页面，新增 18 张 PC / 手机状态画板和一张契约板；原 Desktop / Mobile 的历史主画板也同步更新。

## 历史

保留文件管理器式三列网格。文件图标、文件名下只增加一行状态：Reusable、One-time、Expired；云端来源用小云标记。筛选采用轻量文字标签，不加卡片堆叠、大小、日期或长说明。PC 仍从节点展开浮层，手机进入全屏二级页面。

本轮 One-time 定义为无可从历史复用的留存副本。一次性或过期不删除历史；它们的下一步为 Choose files。有效云副本 Restore，本机有效引用 Send again；具体到期时间与保留策略放进详情，权限和可用性重新校验。

临时副本也可以在到期前重复使用。详情“更多”预留 Keep available，需确认后提升保留策略；不编造套餐、价格或对已丢失字节的恢复承诺。可用性、期限、访问权与未来商业权益分别建模。

## 云节点

恢复时用云轮廓节点作为真实数据来源：Cloud → This device。它是逻辑存储来源，不是在线设备，不显示在线点。PC / 手机普通节点分别 64 / 56，本机仍为 96 / 80。

蓝灰曲线展示真实确认弧长；已采纳的 1600ms 柔和流光限制在蓝色内部，暂停、校验、过期观测和减少动态时停止。数字和操作继续放在 modal / bottom sheet。

100% 后校验，完成解密、完整性检查和本机文件提交后显示 Restored，完成标记在本机。保留原历史，新的恢复对应新的 PullSession，首次交付回执不覆盖。

## 页面

- 12 / History availability：PC、手机各 5 张，涵盖全部记录、可复用、一次性、过期与保留确认。
- 13 / Cloud restore：双端各 4 张，涵盖恢复中、暂停、校验、已恢复。
- 14 / Retention contract：来源、权限、保留与商业能力的映射。

后端契约见 [历史可用性与云端恢复](../../infra/docs/history-availability.md)。当前 CreatePull 对收件人的 ACCEPTED 限制需要通过独立副本读取 grant 补齐；不能直接放开所有已完成传输。本轮为设计与契约修订，应用代码、推送与 staging 部署不在本轮已完成声明中。

验证入口：`check-17-history.cjs`、`motion-verification.json` 与 `openpencil-verification.json`。演示全部使用本地样本，不写入实际云存储。
