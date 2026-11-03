# 预览操作

打开 `motion-lab.html` 或 staging 的 `/designs/motion-v29.html`。所有确认都标记为模拟事件；不会向服务器发起传输、读文件内容或写剪贴板。

1. 选择 `M048 · Transfer flow`：观察灰轨、蓝色确认段和1.8s流光；点连接线打开详情，暂停确认后流光停止。数字只放详情，主图没有进度卡片。
2. 选择 `M023 · Select a device`：Replay后立即Reverse，检查节点从当前位置回位；不是每次从起点跳回。
3. 搜索 `Approach` 或 `Switch between targets`：把示例文件拖到节点或附近24px，检查只有一个halo；跨目标、离开、Escape均能取消。Replay也提供无需拖拽的比较入口。
4. 搜索 `History filter`：All/Reusable切换，已有条目保持身份与选择，内容移动到新位置。搜索 `Availability badge` 看文件角标的变化。
5. 搜索 `Mobile bottom sheet`，切Mobile：拖动grip后松手，或按Close/Escape；查看位置与释放速度衔接、焦点返回。
6. 搜索 `Files and Text`：切换输入模式后再回来，草稿仍在。搜索 `Copy text` 与 `Copy denied` 比较成功确认和保留可选原文。
7. 把 System motion 改为 Reduced，再重复以上步骤；几何位置立即切换，运行/暂停、确认、错误和焦点仍可辨认。

Before/During/After用于比较同一配方，Replay/Reverse用于观察中断，Playback可调慢。多个审计目标会复用同一代表性场景；目标特有的权威和减少动态效果要求在右侧及 `catalog.json`，最终业务接入不在本评审原型内。

OpenPencil看 `27–31` 页，原生是可编辑关键帧；HTML是可播放补充，二者用途不同。v27账号、v28跨用户/文本和v29动效各自保留采用状态。
