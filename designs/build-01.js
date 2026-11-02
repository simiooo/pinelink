const C = {
  bg: '#F5F6F7', paper: '#FFFFFF', text: '#30363D', muted: '#626C76',
  line: '#E2E6EA', accent: '#6B4A34', soft: '#E9E2D9'
}
const rgb = hex => {
  const n = parseInt(hex.slice(1), 16)
  return { r: ((n >> 16) & 255) / 255, g: ((n >> 8) & 255) / 255, b: (n & 255) / 255 }
}
const fill = color => [{ type: 'SOLID', color: rgb(color) }]
const rect = (parent, name, x, y, w, h, color, radius = 0, stroke = false) => {
  const node = figma.createRectangle()
  node.name = name
  node.x = x
  node.y = y
  node.resize(w, h)
  node.fills = fill(color)
  node.cornerRadius = radius
  if (stroke) {
    node.strokes = [{ type: 'SOLID', color: rgb(C.line) }]
    node.strokeWeight = 1
  }
  parent.appendChild(node)
  return node
}
const text = async (parent, name, value, x, y, size, color = C.text, weight = 'Regular', width = 600) => {
  const node = figma.createText()
  node.name = name
  node.x = x
  node.y = y
  node.characters = value
  node.fontSize = size
  node.fills = fill(color)
  node.fontName = { family: 'Inter', style: weight }
  node.resize(width, Math.ceil(size * 1.6))
  parent.appendChild(node)
  return node
}
const frame = (parent, name, x, y, w, h, color = C.paper, radius = 0, stroke = true) => {
  const node = figma.createFrame()
  node.name = name
  node.x = x
  node.y = y
  node.resize(w, h)
  node.fills = fill(color)
  node.cornerRadius = radius
  if (stroke) {
    node.strokes = [{ type: 'SOLID', color: rgb(C.line) }]
    node.strokeWeight = 1
  }
  node.clipsContent = true
  parent.appendChild(node)
  return node
}
const button = async (parent, label, x, y, w, primary = false) => {
  const container = frame(parent, `${label}按钮`, x, y, w, 46, primary ? C.accent : C.bg, 12, !primary)
  await text(container, `${label}按钮文字`, label, 20, 13, 15, primary ? C.paper : C.muted, 'Medium', w - 40)
}
const build = async (root, mobile = false) => {
  const m = mobile
  const pad = m ? 22 : 36
  const inner = m ? 264 : 928
  await text(root, '品牌', 'Pinelink', pad, 32, m ? 23 : 25, C.text, 'Bold', 220)
  const chipX = m ? 150 : 834
  rect(root, '通知按钮背景', chipX, 28, 56, 34, C.bg, 12, true)
  await text(root, '通知', '通知', chipX + 12, 36, 13, C.muted, 'Regular', 50)
  rect(root, '更多按钮背景', chipX + 72, 28, 58, 34, C.bg, 12, true)
  await text(root, '更多', '更多', chipX + 84, 36, 13, C.muted, 'Regular', 50)
  rect(root, '分隔线', pad, 78, inner, 1, C.line)
  await text(root, '页面标题', '发送文件', pad, 104, m ? 23 : 28, C.text, 'Bold', inner)
  await text(root, '页面说明', m ? '对方同意前不会发送内容。' : '选择文件后，再选择接收设备。对方同意前不会发送内容。', pad, m ? 134 : 148, m ? 12 : 15, C.muted, 'Regular', inner)

  rect(root, '文件拖放区', pad, m ? 180 : 206, inner, m ? 306 : 300, C.bg, 16, true)
  rect(root, '上传图标底', pad + (m ? 22 : 32), (m ? 180 : 206) + (m ? 28 : 36), m ? 50 : 56, m ? 50 : 56, C.soft, 16)
  await text(root, '上传图标', '↓', pad + (m ? 34 : 43), (m ? 180 : 206) + (m ? 34 : 42), m ? 26 : 30, C.accent, 'Regular', 40)
  await text(root, '拖放标题', '把文件拖到这里', pad + (m ? 22 : 112), (m ? 180 : 206) + (m ? 100 : 52), m ? 16 : 18, C.text, 'Bold', inner - 100)
  await text(root, '拖放说明', '或点击选择文件', pad + (m ? 22 : 112), (m ? 180 : 206) + (m ? 128 : 84), m ? 12 : 14, C.muted, 'Regular', inner - 100)
  await button(root, '选择文件', pad + (m ? 22 : 32), (m ? 180 : 206) + (m ? 166 : 128), m ? 168 : 180, true)
  await text(root, '拖放备注', m ? '发送前可核对大小与设备。' : '支持任意类型；发送前可核对文件大小与接收设备。', pad + (m ? 22 : 32), (m ? 180 : 206) + (m ? 236 : 208), m ? 12 : 13, C.muted, 'Regular', inner - 50)

  rect(root, '临时副本说明底', pad, m ? 508 : 538, inner, m ? 112 : 74, C.bg, 12)
  await text(root, '临时副本说明', '经过中转时，会保留一份 24 小时后删除的加密临时副本。', pad + (m ? 14 : 24), (m ? 508 : 538) + (m ? 14 : 20), m ? 12 : 13, C.muted, 'Regular', inner - (m ? 28 : 48))
  await text(root, '持久化说明', '默认点对点；持久化保存需另行选择。', pad + (m ? 14 : 24), (m ? 508 : 538) + (m ? 42 : 44), m ? 12 : 13, C.muted, 'Regular', inner - (m ? 28 : 48))
  await text(root, '最近任务标题', '最近任务', pad, m ? 656 : 662, m ? 16 : 17, C.text, 'Bold', inner)
  rect(root, '最近任务底', pad, m ? 682 : 688, inner, m ? 70 : 64, C.bg, 12)
  await text(root, '最近任务空态', m ? '暂无任务。确认后会显示在这里。' : '暂无任务。文件与接收设备确认后会出现在这里。', pad + (m ? 14 : 20), (m ? 682 : 688) + (m ? 16 : 22), m ? 12 : 14, C.muted, 'Regular', inner - (m ? 28 : 40))
}

await figma.loadFontAsync({ family: 'Inter', style: 'Regular' })
await figma.loadFontAsync({ family: 'Inter', style: 'Medium' })
await figma.loadFontAsync({ family: 'Inter', style: 'Bold' })
figma.currentPage.name = '01 新建传输'
for (const child of [...figma.currentPage.children]) child.remove()
rect(figma.currentPage, '页面背景', 0, 0, 1440, 900, C.bg)
await text(figma.currentPage, '桌面版标注', '01 · 新建传输 / 主屏仅三个操作', 40, 28, 18, C.text, 'Bold', 700)
await text(figma.currentPage, '移动端标注', '移动端 390 × 844', 1092, 28, 18, C.text, 'Bold', 300)
const desktop = frame(figma.currentPage, '桌面 · 新建传输', 40, 70, 1000, 790, C.paper, 16, true)
await build(desktop, false)
const mobile = frame(figma.currentPage, '移动端 · 新建传输', 1092, 70, 308, 790, C.paper, 24, true)
await build(mobile, true)
figma.currentPage.selection = [desktop, mobile]
