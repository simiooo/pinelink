import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/pages/tasks_page.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

class FakePinelinkApi extends PinelinkApi {
  var task = const TransferTask(
    'task-1',
    'DOWNLOAD',
    'RUNNING',
    1500000,
    2400000,
    referenceId: 'request-1',
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
      'request-1',
      'ACCEPTED',
      2,
      '设计回顾.pdf',
      2400000,
      'team-design',
      'macbook-pro',
      recipientDevice: 'dev-device',
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
      action == 'pause'
          ? 'PAUSED'
          : action == 'resume'
          ? 'RUNNING'
          : 'CANCELLED',
      task.progress,
      task.total,
      referenceId: task.referenceId,
      version: task.version + 1,
    );
  }
}

void main() {
  testWidgets('task controls keep real progress and versions', (tester) async {
    final api = FakePinelinkApi();
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: MediaQuery(
          data: const MediaQueryData(disableAnimations: true),
          child: Scaffold(body: TasksPage(api: api)),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('62%'), findsNothing);
    await tester.tap(find.text('接收中'));
    await tester.pumpAndSettle();
    expect(find.text('设计回顾.pdf'), findsOneWidget);
    expect(find.text('62%'), findsOneWidget);
    expect(find.text('macbook-pro'), findsNWidgets(2));
    await tester.tap(find.text('暂停'));
    await tester.pumpAndSettle();
    expect(find.text('已暂停，进度已保留。'), findsOneWidget);
    await tester.tap(find.text('继续'));
    await tester.pumpAndSettle();
    expect(find.text('已继续传输。'), findsOneWidget);
    await tester.tap(find.text('取消'));
    await tester.pump();
    await tester.tap(find.text('确认取消'));
    await tester.pumpAndSettle();
    expect(find.text('已取消任务。'), findsOneWidget);
    expect(api.actions, ['pause/1', 'resume/2', 'cancel/3']);
    expect(find.text('62%'), findsOneWidget);
  });
}
