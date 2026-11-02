# 官方 / 自定义服务补充稿

设计提案，未实施、未验收。14个桌面/移动端画面覆盖官方菜单、自定义连接、高级配置、完整SHA-256指纹核对、连接失败、已连接自定义服务与活动传输切换。

主色#0866FF，白灰界面，语义分组、局部左对齐与48px操作区域。源稿由`../build-22-service-modes.py`生成，manifest记录布局和入口；交互及profile隔离契约见[service-modes.md](../../infra/docs/service-modes.md)。

[可编辑.fig](../23-pinelink-service-and-offline.fig)包含本页14帧与Web离线提案22帧。OpenPencil core 0.16.0 / CanvasKit 0.41.1独立引擎写入后重新解析，确认2页、14/22帧及355个非空可编辑文本节点。`*-openpencil-headless.png`为该文件的读回渲染；原生应用重开和产品验收仍待完成。

桌面浮层按内容取360/380/592/648/408/284px高度，最后一组到主按钮32px；恢复官方入口与高级项间24px。移动端20px页边距，复制指纹和恢复官方入口均为48px操作区域，系统栏保持正常对比度。
