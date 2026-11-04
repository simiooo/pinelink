import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/local/controller.dart';
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

void main() {
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
}
