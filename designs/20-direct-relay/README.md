# Pinelink v20 / Direct and relay

2026-11-01。基于v19原生文档增补双路径说明，保留原有18页。

- 新增第18页：直连传输/详情、在线中继传输/详情、直连转中继、已送达但云副本不可用，共12个PC/移动端画面。
- 新增第19页：路径/阶段语义对照和路径切换/完成契约，2张说明板。
- 修正原第17页云预上传说明板的适用范围，并将原第16页的云端收取画面名称明确为Cloud phase；v19原文件不修改。
- `../20-pinelink-direct-relay.fig`为原生可编辑交付；`manifest.json`为新画面与原生按钮坐标；`baseline-adjustments.json`记录继承页面的修订。
- 源文件：`../build-20-direct-relay.py`。SVG不包含动作按钮，导入后使用原生Frame/Text补入48px按钮。
- 后端状态、进度投影、路由迁移和验收条件：`../../infra/docs/direct-relay-delivery.md`；相关architecture、transfer-progress、relay-preupload文档已关联修正。

直连与在线中继均显示两个设备，主进度来自同一接收端账本；Cloud节点只用于实际云上传/云端接收。送达、云封存和路线是独立事实。路径切换保持同一进度身份，阶段切换明确更换指标和节点身份。

沿用v18语义间距、局部左对齐、16px徽标、#0866FF、蓝灰曲线及流光规范。二级详情中显示Direct/Via relay和独立云副本状态，主图不堆叠内部链路信息。

本轮为设计/架构提案，没有修改或部署后端和客户端实现。
