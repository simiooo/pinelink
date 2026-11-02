import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pinelink_mobile/main.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/i18n/controller.dart';
import 'package:pinelink_mobile/src/i18n/messages.g.dart';
import 'package:pinelink_mobile/src/widgets/file_icon.dart';

class FakePinelinkApi extends PinelinkApi {
  @override
  Future<List<TransferTask>> getTasks(
    String owner,
    String device, {
    bool includeDeleted = false,
  }) async => const <TransferTask>[];
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues(<String, Object>{});
  });

  test('both locales contain all message keys and format metadata', () {
    expect(enMessages.keys.toSet(), zhMessages.keys.toSet());
    expect(
      translateLocale(const Locale('en', 'US'), 'encryptedFile'),
      'Encrypted file',
    );
    expect(
      fileMetadataLabel('encrypted', 127, const Locale('en', 'US')),
      contains('127 B'),
    );
    expect(
      formatTime('2026-10-31T10:00:00Z', const Locale('en', 'US')),
      contains('PM'),
    );
    expect(
      formatTime('2026-10-31T10:00:00Z', const Locale('zh', 'CN')),
      isNot(contains('PM')),
    );
  });

  testWidgets('language menu switches and persists the preference', (
    tester,
  ) async {
    final controller = I18nController(initialPreference: 'en-US');
    await tester.pumpWidget(
      PinelinkApp(i18n: controller, api: FakePinelinkApi()),
    );
    await tester.pumpAndSettle();
    expect(find.text('Send files'), findsOneWidget);
    expect(find.text('Choose file'), findsOneWidget);
    await tester.tap(find.text('More'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('中文'));
    await tester.pumpAndSettle();
    expect(controller.preference, 'zh-CN');
    expect(find.text('发送文件'), findsOneWidget);
    final restored = I18nController();
    await restored.load();
    expect(restored.preference, 'zh-CN');
    expect(restored.hasSavedPreference, isTrue);
  });

  testWidgets('stored English is restored before rendering', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{
      'pinelink.locale': 'en-US',
    });
    final controller = I18nController();
    await controller.load();
    await tester.pumpWidget(
      PinelinkApp(i18n: controller, api: FakePinelinkApi()),
    );
    await tester.pumpAndSettle();
    expect(find.text('Send files'), findsOneWidget);
    expect(controller.locale, const Locale('en', 'US'));
    expect(
      Localizations.localeOf(tester.element(find.text('Send files'))),
      const Locale('en', 'US'),
    );
  });

  testWidgets('system language follows platform changes', (tester) async {
    tester.platformDispatcher.localeTestValue = const Locale('zh', 'CN');
    final controller = I18nController();
    addTearDown(() {
      tester.platformDispatcher.clearLocaleTestValue();
      controller.dispose();
    });
    expect(controller.locale, const Locale('zh', 'CN'));
    tester.platformDispatcher.localeTestValue = const Locale('en', 'US');
    await tester.pump();
    expect(controller.locale, const Locale('en', 'US'));
  });
}
