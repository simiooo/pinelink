const page = figma.currentPage
page.name = '01 新建传输'
const frame = figma.createFrame()
frame.name = '桌面 · 新建传输'
frame.x = 40
frame.y = 70
frame.resize(1000, 790)
frame.fills = [{ type: 'SOLID', color: { r: 1, g: 1, b: 1 } }]
frame.cornerRadius = 18
page.appendChild(frame)
const title = figma.createText()
title.name = '页面标题'
title.characters = '发送文件'
title.x = 36
title.y = 104
title.fontSize = 28
title.fills = [{ type: 'SOLID', color: { r: 0.19, g: 0.21, b: 0.24 } }]
frame.appendChild(title)
