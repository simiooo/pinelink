import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/pages/notifications_page.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

class FakePinelinkApi extends PinelinkApi {
  final requests = <TransferRequest>[
    const TransferRequest(
      'request-1',
      'PENDING',
      2,
      '设计回顾.pdf',
      2516582,
      'team-design',
      'macbook-pro',
    ),
  ];
  final responses = <String>[];
  bool conflict = false;
  @override
  Future<List<TransferRequest>> getPending(String owner, String device) async =>
      List.of(requests);
  @override
  Future<void> respond(
    String id,
    String action,
    String owner,
    String device,
    int version,
  ) async {
    responses.add('$id/$action/$version');
    if (conflict) throw const ApiException(409, 'STATE_CONFLICT');
    requests.clear();
  }
}

void main() {
  testWidgets('notification shows real source and accepts once', (
    tester,
  ) async {
    final api = FakePinelinkApi();
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: Scaffold(body: NotificationsPage(api: api)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('macbook-pro'), findsOneWidget);
    expect(find.text('team-design'), findsOneWidget);
    expect(find.text('2.4 MB'), findsOneWidget);
    await tester.tap(find.text('接受'));
    await tester.pumpAndSettle();
    expect(api.responses, ['request-1/accept/2']);
    expect(find.text('已接受文件请求。'), findsOneWidget);
    expect(find.text('没有待处理的文件请求'), findsOneWidget);
  });

  testWidgets('notification conflict is not a success', (tester) async {
    final api = FakePinelinkApi()..conflict = true;
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: Scaffold(body: NotificationsPage(api: api)),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('拒绝'));
    await tester.pumpAndSettle();
    expect(find.text('请求状态已变化，正在为你刷新。'), findsOneWidget);
    expect(find.text('已拒绝文件请求。'), findsNothing);
  });

  testWidgets('empty notifications do not show acceptance actions', (
    tester,
  ) async {
    final api = FakePinelinkApi()..requests.clear();
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: Scaffold(body: NotificationsPage(api: api)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('没有待处理的文件请求'), findsOneWidget);
    expect(find.text('接受'), findsNothing);
  });
}
