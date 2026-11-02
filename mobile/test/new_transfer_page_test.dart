import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/components/app_shell.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

class FakePinelinkApi extends PinelinkApi {
  @override
  Future<List<TransferTask>> getTasks(
    String owner,
    String device, {
    bool includeDeleted = false,
  }) async => const <TransferTask>[];
}

void main() {
  testWidgets('new transfer view keeps exactly three actions', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: PinelinkShell(api: FakePinelinkApi()),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('选择文件'), findsOneWidget);
    expect(find.text('通知'), findsOneWidget);
    expect(find.text('更多'), findsOneWidget);
    expect(find.byType(ElevatedButton), findsOneWidget);
    expect(find.byType(PopupMenuButton<String>), findsOneWidget);
    expect(
      find.text('暂无任务。文件与接收设备确认后会出现在这里。', skipOffstage: false),
      findsOneWidget,
    );
    await tester.tap(find.text('更多'));
    await tester.pumpAndSettle();
    expect(find.text('当前任务'), findsOneWidget);
    expect(find.text('小窝与云仓'), findsOneWidget);
  });
}
