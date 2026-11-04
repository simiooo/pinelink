import 'package:flutter/widgets.dart';

String lc(BuildContext context, String en, String zh) =>
    Localizations.localeOf(context).languageCode == 'zh' ? zh : en;
String localError(BuildContext c, Object error) {
  final code = error is StateError ? error.message.toString() : '$error';
  if (code.contains('NETWORK_UNAVAILABLE')) {
    return lc(c, 'Local network unavailable', '本地网络不可用');
  }
  if (code.contains('CODE_USED') || code.contains('CODE_EXPIRED')) {
    return lc(c, 'Code expired. Create a new one.', '连接码已失效，请重新生成');
  }
  if (code.contains('CODE_INVALID') || code.contains('MESSAGE_INVALID')) {
    return lc(c, 'Invalid connection code', '连接码无效');
  }
  if (code.contains('IDENTITY_CHANGED')) {
    return lc(c, 'Device identity changed', '设备身份已变化');
  }
  if (code.contains('OTHER_WINDOW')) {
    return lc(c, 'Open in another window', '另一窗口正在使用此连接');
  }
  if (code.contains('BUSY') || code.contains('LIMIT')) {
    return lc(c, 'Finish the current transfer first', '请先完成当前传输');
  }
  if (code.contains('TOO_LARGE')) {
    return lc(c, 'Choose a file under 64 MB', '请选择小于 64 MB 的文件');
  }
  if (code.contains('STORAGE')) {
    return lc(c, 'Not enough local storage', '本机存储空间不足');
  }
  if (code.contains('SELF')) return lc(c, 'Choose another device', '请选择另一台设备');
  if (code.contains('PERMISSION')) {
    return lc(c, 'Allow local network access in Settings', '请在系统设置中允许访问本地网络');
  }
  if (code.contains('CHECKSUM')) {
    return lc(c, 'File verification failed', '文件校验失败');
  }
  if (code.contains('SOURCE_MISSING')) {
    return lc(c, 'File is no longer available', '文件已不可用');
  }
  return lc(c, 'Reconnect to continue', '重新连接后继续');
}

String jobStatus(BuildContext c, Map<String, dynamic> job, bool connected) {
  if (job['intent'] == 'cancel') return lc(c, 'Cancelling', '正在取消');
  if (job['expired'] == true) return lc(c, 'Expired', '已过期');
  final s = job['state'] as Map?;
  if (job['authority'] == 'GUEST_SERVICE' &&
      job['direction'] == 'send' &&
      s?['state'] == 'PENDING' &&
      ((job['progress'] as Map?)?['completion'] as Map?)?['stored'] == true) {
    return lc(c, 'Stored in cloud', '已暂存云端');
  }
  return switch (s?['state']) {
    'COMPLETED' =>
      job['direction'] == 'send'
          ? lc(c, 'Sent', '已发送')
          : lc(c, 'Received', '已接收'),
    'REJECTED' => lc(c, 'Declined', '已拒绝'),
    'CANCELLED' => lc(c, 'Cancelled', '已取消'),
    'EXPIRED' => lc(c, 'Expired', '已过期'),
    'FAILED' => lc(c, 'Could not transfer', '传输失败'),
    'ACCEPTED' =>
      !connected || job['disconnected'] == true
          ? lc(c, 'Disconnected', '连接已断开')
          : s?['paused'] == true || job['intent'] == 'pause'
          ? lc(c, 'Paused', '已暂停')
          : s?['received'] == s?['size']
          ? lc(c, 'Verifying', '正在校验')
          : lc(c, 'Transferring', '正在传输'),
    _ => lc(c, 'Waiting', '等待接收'),
  };
}

String localBytes(int bytes) => bytes >= 1048576
    ? '${(bytes / 1048576).toStringAsFixed(1)} MB'
    : bytes >= 1024
    ? '${(bytes / 1024).toStringAsFixed(1)} KB'
    : '$bytes B';
