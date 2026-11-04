import { useI18n } from '../i18n'
const en = {
  connection: 'Connection', direct: 'Direct', switching: 'Switching connection', reconnecting: 'Reconnecting',
  removeOffline: 'Go online to remove this connection.',
  files: 'Files', devices: 'Devices', all: 'All', active: 'Active', savedFilter: 'Saved', issues: 'Issues', notifications: 'Notifications', unread: 'Pending notifications', noNotifications: 'You’re all caught up', wantsConnect: 'Wants to connect', deviceActions: 'Device actions', rename: 'Rename', remove: 'Remove connection', removeTitle: 'Remove connection?', removeHint: 'Files on this device are kept.', nameLabel: 'Name on this device', save: 'Save', nameInvalid: 'Use a name between 1 and 64 characters.', connectionGone: 'This connection is no longer available.', connectionBusy: 'Finish the transfer first.', viewTransfer: 'View transfer', emptyFilter: 'No matching files', backFiles: 'Back to files', sourceAccount: 'Account file', accountFilesError: 'Account files unavailable. Try again.', accountNotificationsError: 'Notifications unavailable. Try again.', confirm: 'Confirm', deleted: 'Removed from the list',
  continueGuest: 'Continue without an account',
  localFiles: 'On this device',
  fileExpired: 'Transfer expired',
  declined: 'Connection declined',
  changed: 'State changed. Try again.',
  connect: 'Connect a device', confirmPair: 'Connect this device?', connectAction: 'Connect', showCode: 'Show code', enterCode: 'Enter code', code: 'Connection code', copyLink: 'Copy link', copied: 'Copied', expires: 'Expires in 10 min', expired: 'Code expired', newCode: 'New code', close: 'Close', local: 'This device', guest: 'Using without an account', file: 'Choose a file', history: 'History', details: 'Transfer details', send: 'Send', receive: 'Receive', decline: 'Decline', waiting: 'Waiting for confirmation', waitingFile: 'Waiting for recipient', connecting: 'Connecting', running: 'Transferring', paused: 'Paused', verifying: 'Verifying', complete: 'Delivered', saved: 'Saved on this device', stored: 'Temporarily stored', stopped: 'Transfer ended', cloud: 'Cloud', download: 'Save file', another: 'Send another', retry: 'Try again', pause: 'Pause', resume: 'Resume', cancel: 'Cancel transfer', offline: 'Offline', unavailable: 'Connection interrupted. Try again.', invalid: 'Check the code and try again.', storage: 'Storage unavailable', storageHint: 'Allow browser storage, then try again.', tooLarge: 'This file exceeds the guest transfer limit.', limit: 'Transfer limit reached. Try again later.', empty: 'No files yet', keepOpen: 'Keep both devices open.', selectDevice: 'Select a device', once: 'One-time transfer', oneFile: 'Choose one file', device: 'Device', done: 'Done', prepared: 'Preparing', received: 'Received', upload: 'Cloud copy', signIn: 'Sign in', waitingConnection: 'Waiting for the other device', fileLost: 'Choose the file again.', fileInvalid: 'File verification failed. Try sending again.', preparing: 'Encrypting file', onlineOnly: 'Connect to the internet to link a device.',
}
type Copy = typeof en
const zh: Copy = {
  connection: '连接方式', direct: '直连', switching: '正在切换连接', reconnecting: '正在重新连接',
  removeOffline: '联网后可移除此连接。',
  files: '文件', devices: '设备', all: '全部', active: '进行中', savedFilter: '本机', issues: '异常', notifications: '通知', unread: '有待处理通知', noNotifications: '暂无待处理通知', wantsConnect: '请求连接', deviceActions: '设备操作', rename: '修改备注', remove: '移除连接', removeTitle: '移除连接？', removeHint: '本机文件仍会保留。', nameLabel: '本机显示的名称', save: '保存', nameInvalid: '请输入 1–64 个字符。', connectionGone: '此连接已失效。', connectionBusy: '请先结束当前传输。', viewTransfer: '查看传输', emptyFilter: '暂无符合条件的文件', backFiles: '返回文件', sourceAccount: '账号文件', accountFilesError: '暂时无法读取账号文件，请重试。', accountNotificationsError: '暂时无法读取通知，请重试。', confirm: '确认', deleted: '已从列表移除',
  continueGuest: '无需账号，继续使用',
  localFiles: '本机文件',
  fileExpired: '传输已过期',
  declined: '对方已拒绝连接',
  changed: '状态已变化，请重试。',
  connect: '连接设备', confirmPair: '连接此设备？', connectAction: '连接', showCode: '显示连接码', enterCode: '输入连接码', code: '连接码', copyLink: '复制链接', copied: '已复制', expires: '10 分钟内有效', expired: '连接码已过期', newCode: '重新生成', close: '关闭', local: '本机', guest: '无需账号使用中', file: '选择文件', history: '历史', details: '传输详情', send: '发送', receive: '接收', decline: '拒绝', waiting: '等待确认', waitingFile: '等待对方接收', connecting: '正在连接', running: '正在传输', paused: '已暂停', verifying: '正在校验', complete: '已送达', saved: '已保存在本机', stored: '已暂存', stopped: '传输已结束', cloud: '云端', download: '保存文件', another: '继续发送', retry: '重试', pause: '暂停', resume: '继续', cancel: '取消传输', offline: '离线', unavailable: '连接已中断，请重试。', invalid: '请检查连接码后重试。', storage: '无法在本机保存', storageHint: '请允许浏览器保存数据后重试。', tooLarge: '文件超出访客传输上限。', limit: '已达到传输上限，请稍后重试。', empty: '暂无文件', keepOpen: '请保持两台设备开启。', selectDevice: '选择设备', once: '一次性传输', oneFile: '请选择一个文件', device: '设备', done: '完成', prepared: '正在准备', received: '已接收', upload: '云端暂存', signIn: '登录', waitingConnection: '等待对方确认连接', fileLost: '请重新选择文件。', fileInvalid: '文件校验失败，请重新发送。', preparing: '正在加密文件', onlineOnly: '联网后即可连接设备。',
}
export function useGuestCopy() { return useI18n().locale === 'zh-CN' ? zh : en }
export function guestErrorText(error: string | undefined, c: Copy) {
  if (!error) return ''
  if (error === 'DEVICE_NAME_INVALID') return c.nameInvalid
  if (error === 'CONNECTION_GONE') return c.connectionGone
  if (error === 'CONNECTION_BUSY') return c.connectionBusy
  if (navigator.onLine === false) return c.onlineOnly
  if (['NOT_FOUND', 'INVALID_REQUEST', 'EXPIRED'].includes(error)) return error === 'EXPIRED' ? c.expired : c.invalid
  if (error === 'FILE_TOO_LARGE') return c.tooLarge
  if (error === 'FILE_EXPIRED') return c.fileExpired
  if (['AUTH_RATE_LIMITED', 'GUEST_LIMIT_REACHED'].includes(error)) return c.limit
  if (error === 'FILE_SOURCE_MISSING') return c.fileLost
  if (error === 'ONE_FILE_ONLY') return c.oneFile
  if (error === 'CONNECTION_DECLINED') return c.declined
  if (error === 'STATE_CONFLICT' || error === 'FILE_JOB_CHANGED') return c.changed
  if (/^(FILE_(CHECKSUM|FRAMING|NONCE|MANIFEST|FRAME|TRUNCATED)|OperationError)/.test(error)) return c.fileInvalid
  return c.unavailable
}
