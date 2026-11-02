import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/pages/history_page.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

class FakePinelinkApi extends PinelinkApi {
  var task = const TransferTask(
    'task-history',
    'DOWNLOAD',
    'CANCELLED',
    100,
    1000,
    referenceId: 'request-history',
    version: 2,
    errorCode: 'TRANSFER_CANCELLED',
    updatedAt: '2026-10-31T10:00:00Z',
  );
  final actions = <String>[];
  @override
  Future<List<TransferTask>> getTasks(
    String owner,
    String device, {
    bool includeDeleted = false,
  }) async => [task];
  @override
  Future<List<TransferRequest>> getTransfers(
    String owner,
    String device,
  ) async => [
    const TransferRequest(
      'request-history',
      'CANCELLED',
      2,
      '设计回顾.pdf',
      1000,
      'team-design',
      'macbook-pro',
    ),
  ];
  @override
  Future<void> controlTask(
    String id,
    String action,
    String owner,
    String device,
    int version,
  ) async {
    actions.add('$action/$version');
    task = TransferTask(
      task.id,
      task.kind,
      'DELETED',
      task.progress,
      task.total,
      referenceId: task.referenceId,
      version: task.version + 1,
      errorCode: task.errorCode,
    );
  }
}

void main() {
  testWidgets('history shows a reason and confirms logical deletion', (
    tester,
  ) async {
    final api = FakePinelinkApi();
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: Scaffold(body: HistoryPage(api: api)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('设计回顾.pdf'), findsOneWidget);
    expect(find.text('对方取消了这次传输。文件未标记为已收到。'), findsOneWidget);
    await tester.tap(find.text('从列表删除'));
    await tester.pump();
    await tester.tap(find.text('确认删除'));
    await tester.pumpAndSettle();
    expect(api.actions, ['delete/2']);
    expect(find.text('已从默认列表删除，历史记录仍然保留。'), findsOneWidget);
  });
}
