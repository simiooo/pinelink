import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/pages/settings_page.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

class FakePinelinkApi extends PinelinkApi {
  var file = const FileSnapshot(
    'entry-1',
    '设计回顾.pdf',
    'AVAILABLE',
    4,
    'TEMP_24H',
    null,
  );
  var device = const Device('dev-device', 'ACTIVE', '2026-10-31T10:00:00Z');
  int? promotedVersion;
  String? revoked;
  @override
  Future<List<Device>> getDevices(String owner) async => [device];
  @override
  Future<Quota> getQuota(String owner) async => const Quota(1000, 900);
  @override
  Future<List<TransferRequest>> getTransfers(
    String owner,
    String device,
  ) async => [
    const TransferRequest(
      'request-owned',
      'ACCEPTED',
      2,
      '设计回顾.pdf',
      1000,
      'dev-owner',
      'dev-device',
      entryId: 'entry-1',
    ),
  ];
  @override
  Future<FileSnapshot> getFile(String id, String owner) async => file;
  @override
  Future<void> promoteFile(
    String id,
    String owner,
    String device,
    int version,
  ) async {
    promotedVersion = version;
    file = FileSnapshot(
      file.id,
      file.fileName,
      file.state,
      file.version + 1,
      'PERSISTENT',
      null,
    );
  }

  @override
  Future<void> revokeDevice(String owner, String id) async {
    revoked = id;
    device = Device(id, 'REVOKED', device.lastSeenAt);
  }
}

void main() {
  testWidgets('settings promotes a file with its current version', (
    tester,
  ) async {
    final api = FakePinelinkApi();
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: MediaQuery(
          data: const MediaQueryData(disableAnimations: true),
          child: Scaffold(body: SettingsPage(api: api)),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('900 B / 1000 B'), findsOneWidget);
    await tester.scrollUntilVisible(
      find.text('长期保存'),
      200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.tap(find.text('长期保存'));
    await tester.pumpAndSettle();
    expect(api.promotedVersion, 4);
    expect(find.text('已长期保存；之后仍可重复获取。'), findsOneWidget);
    expect(find.text('空间即将用完，请先清理不再需要的文件。'), findsOneWidget);
  });

  testWidgets('device revocation requires a second confirmation', (
    tester,
  ) async {
    final api = FakePinelinkApi();
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: MediaQuery(
          data: const MediaQueryData(disableAnimations: true),
          child: Scaffold(body: SettingsPage(api: api)),
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.scrollUntilVisible(
      find.text('管理设备'),
      200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.tap(find.text('管理设备'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('撤销设备'));
    await tester.pumpAndSettle();
    expect(api.revoked, isNull);
    await tester.scrollUntilVisible(
      find.text('管理设备'),
      200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.tap(find.text('管理设备'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('确认撤销'));
    await tester.pumpAndSettle();
    expect(api.revoked, 'dev-device');
    expect(find.text('设备已撤销，其未完成的传输已停止。'), findsOneWidget);
  });
}
