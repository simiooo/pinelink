import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/main.dart';
import 'package:pinelink_mobile/src/i18n/controller.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';

class EmptyApi extends PinelinkApi {
  @override
  Future<List<TransferTask>> getTasks(
    String owner,
    String device, {
    bool includeDeleted = false,
  }) async => [];
}

void main() {
  testWidgets('application opens the new transfer page', (tester) async {
    await tester.pumpWidget(
      PinelinkApp(
        i18n: I18nController(initialPreference: 'zh-CN'),
        api: EmptyApi(),
      ),
    );
    expect(find.text('发送文件'), findsOneWidget);
    expect(find.text('选择文件'), findsOneWidget);
    await tester.pump();
  });
}
