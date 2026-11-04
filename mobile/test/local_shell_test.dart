import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/local/controller.dart';
import 'package:pinelink_mobile/src/local/conversation.dart';
import 'package:pinelink_mobile/src/local/connect.dart';
import 'package:pinelink_mobile/src/local/shell.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';
import 'package:pinelink_mobile/src/motion/motion.dart';

class _Local extends LocalController {
  @override
  List<LocalMap> peers = [
    {'root': 'peer', 'name': 'MacBook Pro'},
  ];
  @override
  List<LocalMap> jobs = [
    {
      'id': 'file',
      'name': 'Design source.fig',
      'created': 1,
      'plainSize': 65536,
      'cipherSize': 65568,
      'expires': 9999999999999,
      'direction': 'send',
      'peer': {'root': 'peer', 'name': 'MacBook Pro'},
      'state': {
        'state': 'ACCEPTED',
        'version': 2,
        'revision': 1,
        'received': 32768,
        'size': 65568,
        'paused': false,
      },
    },
  ];
  @override
  List<LocalMap> nearby = [];
  @override
  LocalMap pairing = {'phase': 'idle'};
  @override
  String? discoveryError;
  @override
  bool discovering = true;
  @override
  bool reachable(String root) => true;
  @override
  Future<void> discover() async {}
  @override
  Future<void> connectNearby(String id) async {}
  @override
  Future<String?> invite([String? expected]) async => null;
  @override
  Future<String?> join(String code, [String? expected]) async => null;
  int confirmations = 0;
  @override
  Future<void> confirm() async {
    confirmations++;
    pairing = {...pairing, 'phase': 'waiting'};
    notifyListeners();
  }

  @override
  Future<void> cancelPairing() async {
    pairing = {'phase': 'idle'};
    notifyListeners();
  }

  @override
  Future<String> send(Uint8List bytes, String name, LocalMap peer) async =>
      'test';
  @override
  Future<void> action(String id, String action) async {}
  @override
  Future<Uint8List?> plaintext(String id) async => null;
  @override
  Future<void> rename(String root, String name) async {}
  @override
  Future<void> remove(String root) async {}
  @override
  Future<void> foreground(bool active) async {}
  @override
  Future<void> close() async {}
}

class _TextLocal extends _Local {
  String? sentText;
  String? sentSensitivity;
  @override
  bool supportsText(String root) => true;
  @override
  Future<String> sendText(
    Uint8List bytes,
    LocalMap peer,
    String sensitivity,
  ) async {
    sentText = String.fromCharCodes(bytes);
    sentSensitivity = sensitivity;
    return 'new-text';
  }

  @override
  Future<({String value, bool sensitive})?> text(String id) async =>
      (value: '# Private\n\n**secret**', sensitive: true);
}

void main() {
  testWidgets(
    'device conversation hides sensitive Markdown and sends only after a tap',
    (tester) async {
      final r = _TextLocal();
      r.jobs = [
        {
          'id': 'text',
          'created': 1,
          'direction': 'receive',
          'peer': {'root': 'peer'},
          'contentKind': 'TEXT',
        },
      ];
      await tester.pumpWidget(
        MaterialApp(
          home: LocalConversation(
            controller: r,
            root: 'peer',
            onJob: (_, _) {},
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Private'), findsNothing);
      await tester.tap(find.byTooltip('Reveal'));
      await tester.pumpAndSettle();
      expect(
        find.textContaining('Private', findRichText: true),
        findsOneWidget,
      );
      await tester.tap(find.byType(Switch));
      await tester.enterText(find.byType(TextField), 'hello');
      await tester.pump();
      expect(r.sentText, isNull);
      await tester.tap(find.byTooltip('Send'));
      await tester.pumpAndSettle();
      expect(r.sentText, 'hello');
      expect(r.sentSensitivity, 'SENSITIVE');
    },
  );

  testWidgets(
    'smart connection presents one code or link input and keeps offline pairing',
    (tester) async {
      final r = _SmartLocal();
      tester.view.physicalSize = const Size(320, 844);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SingleChildScrollView(child: LocalConnect(controller: r)),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('5V9A VDFJ'), findsOneWidget);
      await tester.tap(find.text('Enter code'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), 'PL2:local-package');
      await tester.tap(find.text('Connect'));
      await tester.pumpAndSettle();
      expect(r.joined, 'PL2:local-package');
      await tester.tap(find.text('Show code'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Connect without internet'));
      await tester.pumpAndSettle();
      expect(r.offlineInvited, isTrue);
      expect(find.text('Scan on the other device'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  Future<void> app(WidgetTester tester, _Local controller) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(
      MaterialApp(
        theme: pinelinkTheme(),
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(context).copyWith(disableAnimations: true),
          child: MotionScope(child: child!),
        ),
        home: LocalShell(controller: controller),
      ),
    );
    await tester.pump();
  }

  testWidgets(
    'dismissing a sheet preserves the mounted graph and its real acknowledgement',
    (tester) async {
      final r = _Local();
      await app(tester, r);
      final finder = find.byKey(const ValueKey('native-transfer-graph')),
          graph = tester.state(finder);
      await tester.tap(find.byTooltip('Add device').last);
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text('Nearby'), findsOneWidget);
      expect(find.byTooltip('Close'), findsOneWidget);
      await tester.tap(find.byTooltip('Close'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.pump();
      expect(identical(tester.state(finder), graph), isTrue);
      expect(r.jobs.single['state']['received'], 32768);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'device actions expand from the stationary node and open one conversation',
    (tester) async {
      final r = _Local()..jobs = [];
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        MaterialApp(
          theme: pinelinkTheme(),
          home: MotionScope(child: LocalShell(controller: r)),
        ),
      );
      await tester.pumpAndSettle();
      final node = find.byIcon(Icons.computer_outlined);
      final before = tester.getCenter(node);
      await tester.tap(node);
      await tester.pump(const Duration(milliseconds: 30));
      final early = tester.getCenter(find.byTooltip('Conversation'));
      expect((early - before).distance, lessThan(90));
      await tester.pumpAndSettle();
      expect(tester.getCenter(node), before);
      expect(find.byTooltip('Rename'), findsOneWidget);
      expect(find.byTooltip('Remove device'), findsOneWidget);
      await tester.tap(find.byTooltip('Conversation'));
      await tester.pumpAndSettle();
      expect(find.byType(LocalConversation), findsOneWidget);
      expect(find.byTooltip('Attach file'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets('logo returns an active transfer to the device graph', (
    tester,
  ) async {
    final r = _Local();
    await app(tester, r);
    expect(find.byKey(const ValueKey('native-transfer-graph')), findsOneWidget);
    await tester.tap(find.text('Pinelink'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('native-transfer-graph')), findsNothing);
    expect(find.text('MacBook Pro'), findsOneWidget);
    expect(r.jobs.single['state']['received'], 32768);
  });
  testWidgets(
    'incoming pair opens one surface and one confirmation becomes a waiting state',
    (tester) async {
      final r = _Local();
      await app(tester, r);
      r.pairing = {'phase': 'verify', 'peer': 'Pixel 9', 'code': '248 617'};
      r.notifyListeners();
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      r.notifyListeners();
      await tester.pump();
      expect(find.byTooltip('Close'), findsOneWidget);
      expect(find.text('248 617'), findsOneWidget);
      await tester.tap(find.text('Numbers match'));
      await tester.pump();
      expect(find.text('Waiting for confirmation'), findsOneWidget);
      expect(r.confirmations, 1);
      await tester.tap(find.byTooltip('Close'));
      await tester.pump(const Duration(milliseconds: 400));
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'a dismissed online request remains reachable from notifications',
    (tester) async {
      final r = _SmartLocal();
      r.jobs = [];
      await app(tester, r);
      r.request = {'id': 'candidate', 'device_name': 'Pixel 9'};
      r.showPairRequest('candidate');
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text('Pixel 9'), findsWidgets);
      expect(find.text('Connect'), findsOneWidget);
      await tester.tap(find.byTooltip('Close'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.byTooltip('Notifications'));
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text('Connection request'), findsOneWidget);
      await tester.tap(find.text('Pixel 9'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text('Connect'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}

class _SmartLocal extends _Local {
  String? joined;
  bool offlineInvited = false;
  LocalMap? request;
  @override
  bool get smartConnection => true;
  @override
  List<LocalMap> get pairRequests => request == null ? [] : [request!];
  @override
  void showPairRequest(String id) {
    pairing = {
      'phase': 'online-request',
      'candidate': id,
      'peer': request?['device_name'],
    };
    notifyListeners();
  }

  @override
  Future<String?> invite([String? expected]) async {
    pairing = {
      'phase': 'online-show',
      'value': 'https://example.test/connect#abcdefghijklmnopqrstuv',
      'code': '5V9AVDFJ',
    };
    notifyListeners();
    return pairing['value'];
  }

  @override
  Future<String?> inviteOffline() async {
    offlineInvited = true;
    pairing = {'phase': 'show', 'value': 'PL2:local-package'};
    notifyListeners();
    return pairing['value'];
  }

  @override
  Future<String?> join(String code, [String? expected]) async {
    joined = code;
    return null;
  }
}
