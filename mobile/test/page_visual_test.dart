import 'dart:io';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/components/app_shell.dart';
import 'package:pinelink_mobile/src/i18n/controller.dart';
import 'package:pinelink_mobile/src/pages/send_preparation_page.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

class VisualApi extends PinelinkApi {
  @override
  Future<List<TransferRequest>> getPending(String owner, String device) async =>
      [
        const TransferRequest(
          'request-cloud',
          'PENDING',
          1,
          'encrypted',
          127,
          'team-design',
          'macbook-pro',
        ),
      ];

  @override
  Future<List<TransferTask>> getTasks(
    String owner,
    String device, {
    bool includeDeleted = false,
  }) async => [
    TransferTask(
      'task-1',
      'DOWNLOAD',
      includeDeleted ? 'CANCELLED' : 'RUNNING',
      1500000,
      2400000,
      referenceId: 'request-1',
      version: 2,
      errorCode: includeDeleted ? 'TRANSFER_CANCELLED' : null,
      updatedAt: '2026-10-31T10:00:00Z',
    ),
  ];

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
    const TransferRequest(
      'request-owned',
      'ACCEPTED',
      2,
      '设计回顾.pdf',
      2400000,
      'dev-owner',
      'dev-device',
      entryId: 'entry-1',
    ),
  ];

  @override
  Future<List<Device>> getDevices(String owner) async => [
    const Device('dev-device', 'ACTIVE', '2026-10-31T10:00:00Z'),
  ];

  @override
  Future<Quota> getQuota(String scope) async => const Quota(1000, 900);

  @override
  Future<FileSnapshot> getFile(String id, String owner) async =>
      const FileSnapshot(
        'entry-1',
        '设计回顾.pdf',
        'AVAILABLE',
        4,
        'TEMP_24H',
        null,
      );
}

Future<void> pumpScreen(WidgetTester tester, Widget screen) async {
  tester.view.physicalSize = const Size(390, 844);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  final base = pinelinkTheme();
  await tester.pumpWidget(
    MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        fontFamily: 'Microsoft YaHei',
        scaffoldBackgroundColor: base.scaffoldBackgroundColor,
        colorScheme: base.colorScheme,
        textTheme: base.textTheme.apply(fontFamily: 'Microsoft YaHei'),
        elevatedButtonTheme: ElevatedButtonThemeData(
          style: base.elevatedButtonTheme.style!.copyWith(
            textStyle: const WidgetStatePropertyAll(
              TextStyle(
                fontFamily: 'Microsoft YaHei',
                fontSize: 15,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ),
        outlinedButtonTheme: base.outlinedButtonTheme,
      ),
      home: MediaQuery(
        data: const MediaQueryData(disableAnimations: true),
        child: screen,
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  if (!Platform.isWindows) {
    test(
      'Windows font reference goldens',
      () {},
      skip: 'Reference images require the Windows Microsoft YaHei font.',
    );
    return;
  }
  TestWidgetsFlutterBinding.ensureInitialized();
  setUpAll(() async {
    final bytes = await File('C:/Windows/Fonts/msyh.ttc').readAsBytes();
    final font = FontLoader('Microsoft YaHei')
      ..addFont(Future<ByteData>.value(ByteData.sublistView(bytes)));
    await font.load();
    final configFile = File('.dart_tool/package_config.json').absolute;
    final config =
        jsonDecode(await configFile.readAsString()) as Map<String, dynamic>;
    final flutter = (config['packages'] as List<dynamic>)
        .cast<Map<String, dynamic>>()
        .singleWhere((package) => package['name'] == 'flutter');
    final packageRoot = configFile.uri.resolve(flutter['rootUri'] as String);
    final sdk = Directory.fromUri(packageRoot).parent.parent;
    final iconBytes = await File.fromUri(
      sdk.uri.resolve(
        'bin/cache/artifacts/material_fonts/materialicons-regular.otf',
      ),
    ).readAsBytes();
    final icons = FontLoader('MaterialIcons')
      ..addFont(Future<ByteData>.value(ByteData.sublistView(iconBytes)));
    await icons.load();
  });

  testWidgets('home visual review', (tester) async {
    await pumpScreen(tester, PinelinkShell(api: VisualApi()));
    await expectLater(
      find.byType(PinelinkShell),
      matchesGoldenFile('goldens/01-home.png'),
    );
  });

  testWidgets('notifications visual review', (tester) async {
    await pumpScreen(
      tester,
      PinelinkShell(api: VisualApi(), initialPage: AppPage.notifications),
    );
    await expectLater(
      find.byType(PinelinkShell),
      matchesGoldenFile('goldens/02-notifications.png'),
    );
  });

  testWidgets('task visual review', (tester) async {
    await pumpScreen(
      tester,
      PinelinkShell(api: VisualApi(), initialPage: AppPage.tasks),
    );
    await expectLater(
      find.byType(PinelinkShell),
      matchesGoldenFile('goldens/03-task.png'),
    );
  });

  testWidgets('history visual review', (tester) async {
    await pumpScreen(
      tester,
      PinelinkShell(api: VisualApi(), initialPage: AppPage.history),
    );
    await expectLater(
      find.byType(PinelinkShell),
      matchesGoldenFile('goldens/04-history.png'),
    );
  });

  testWidgets('settings visual review', (tester) async {
    await pumpScreen(
      tester,
      PinelinkShell(api: VisualApi(), initialPage: AppPage.settings),
    );
    await expectLater(
      find.byType(PinelinkShell),
      matchesGoldenFile('goldens/05-settings.png'),
    );
  });

  testWidgets('send preparation visual review', (tester) async {
    await pumpScreen(
      tester,
      Scaffold(
        appBar: PinelinkAppBar(page: AppPage.home, onNavigate: (_) {}),
        body: const SendPreparationPage(
          name: '设计回顾.pdf',
          size: 2400000,
          filePath: 'D:\\Transfers\\设计回顾.pdf',
        ),
      ),
    );
    await expectLater(
      find.byType(Scaffold),
      matchesGoldenFile('goldens/07-send-preparation.png'),
    );
  });

  testWidgets('English home visual review', (tester) async {
    await pumpScreen(
      tester,
      PinelinkI18n(
        controller: I18nController(initialPreference: 'en-US'),
        child: PinelinkShell(api: VisualApi()),
      ),
    );
    await expectLater(
      find.byType(PinelinkShell),
      matchesGoldenFile('goldens/09-i18n-home.png'),
    );
  });

  testWidgets('English language menu visual review', (tester) async {
    await pumpScreen(
      tester,
      PinelinkI18n(
        controller: I18nController(initialPreference: 'en-US'),
        child: PinelinkShell(api: VisualApi()),
      ),
    );
    await tester.tap(find.text('More'));
    await tester.pumpAndSettle();
    await expectLater(
      find.byType(MaterialApp),
      matchesGoldenFile('goldens/09-i18n-menu.png'),
    );
  });
}
