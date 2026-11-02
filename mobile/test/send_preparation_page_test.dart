import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/pages/send_preparation_page.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

void main() {
  testWidgets('file preparation copies a command without claiming delivery', (
    tester,
  ) async {
    String? command;
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (call) async {
          if (call.method == 'Clipboard.setData') {
            command = call.arguments['text'];
          }
          return null;
        });
    addTearDown(
      () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(SystemChannels.platform, null),
    );
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        home: const Scaffold(
          body: SendPreparationPage(
            name: '设计回顾.pdf',
            size: 1024,
            filePath: 'D:\\Transfers\\设计回顾.pdf',
          ),
        ),
      ),
    );
    await tester.enterText(find.byType(TextField).at(0), 'team-design');
    await tester.enterText(find.byType(TextField).at(1), 'macbook-pro');
    await tester.tap(find.text('复制发送命令'));
    await tester.pump();
    expect(
      command,
      "pinelink p2p send 'D:\\Transfers\\设计回顾.pdf' --to-owner 'team-design' --to-device 'macbook-pro' --path auto",
    );
    expect(find.text('已复制。请在已登记设备的终端执行；这里尚未发送文件。'), findsOneWidget);
    expect(find.text('接受'), findsNothing);
  });
}
