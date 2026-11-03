import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/motion/motion.dart';
import 'package:pinelink_mobile/src/motion/surface.dart';
import 'package:pinelink_mobile/src/motion/transfer_graph.dart';

Widget app(Widget child, {bool reduced = false, bool visible = true}) => MaterialApp(home: MediaQuery(
  data: MediaQueryData(disableAnimations: reduced), child: MotionScope(child: TickerMode(enabled: visible,
    child: Scaffold(body: child)))));

Widget graph({double value = .42, bool running = true, bool complete = false}) => TransferGraph(
  value: value, running: running, completed: complete, cloud: false, incoming: true,
  localLabel: 'This device', peerLabel: 'Alex', statusLabel: running ? 'Receiving' : 'Paused',
  detailsLabel: 'Transfer details', onDetails: () {});

void main() {
  testWidgets('application background stops activity until the app resumes', (tester) async {
    await tester.pumpWidget(app(graph())); await tester.pump(const Duration(milliseconds: 50));
    expect(tester.binding.transientCallbackCount, greaterThan(0));
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused); await tester.pump();
    expect(tester.binding.transientCallbackCount, 0);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed); await tester.pump();
    expect(tester.binding.transientCallbackCount, greaterThan(0));
    await tester.pumpWidget(const SizedBox.shrink()); expect(tester.binding.transientCallbackCount, 0);
  });
  testWidgets('rapid taps create one operation and late completion is safe after disposal', (tester) async {
    final finished = Completer<void>(); var calls = 0;
    await tester.pumpWidget(app(Center(child: MotionButton.primary(onPressed: () { calls++; return finished.future; }, child: const Text('Send')))));
    await tester.tap(find.text('Send')); await tester.tap(find.text('Send')); await tester.pump();
    expect(calls, 1);
    expect(tester.widget<ElevatedButton>(find.byType(ElevatedButton)).onPressed, isNull);
    await tester.pumpWidget(const SizedBox.shrink()); finished.complete(); await tester.pump();
    expect(tester.takeException(), isNull);
    expect(tester.binding.transientCallbackCount, 0);
  });

  testWidgets('changing reduced motion stops a real pending spinner without completing its action', (tester) async {
    final finished = Completer<void>(); var calls = 0;
    Widget fixture(bool reduced) => app(Center(child: MotionButton(onPressed: () { calls++; return finished.future; }, child: const Text('Keep'))), reduced: reduced);
    await tester.pumpWidget(fixture(false)); await tester.tap(find.text('Keep')); await tester.pump(const Duration(milliseconds: 80));
    expect(tester.binding.transientCallbackCount, greaterThan(0));
    await tester.pumpWidget(fixture(true)); await tester.pumpAndSettle();
    expect(tester.binding.transientCallbackCount, 0);
    expect(calls, 1); expect(finished.isCompleted, isFalse);
    finished.complete(); await tester.pumpAndSettle();
  });

  testWidgets('flow stops when hidden or paused and a late acknowledgement stays static', (tester) async {
    await tester.pumpWidget(app(graph())); await tester.pump(const Duration(milliseconds: 100));
    expect(tester.binding.transientCallbackCount, greaterThan(0));
    await tester.pumpWidget(app(graph(), visible: false)); await tester.pump();
    expect(tester.binding.transientCallbackCount, 0);
    await tester.pumpWidget(app(graph())); await tester.pump(const Duration(milliseconds: 80));
    expect(tester.binding.transientCallbackCount, greaterThan(0));
    await tester.pumpWidget(app(graph(value: .66, running: false))); await tester.pump();
    expect(tester.binding.transientCallbackCount, 0);
    await tester.pumpWidget(const SizedBox.shrink());
    expect(tester.takeException(), isNull);
  });

  testWidgets('full bytes without a receipt do not draw a delivery check or continue transfer flow', (tester) async {
    await tester.pumpWidget(app(graph(value: 1, complete: false))); await tester.pump();
    expect(find.byIcon(Icons.check), findsNothing);
    expect(tester.binding.transientCallbackCount, 0);
    await tester.pumpWidget(app(graph(value: 1, complete: true, running: false))); await tester.pump();
    expect(find.byIcon(Icons.check), findsOneWidget);
  });

  testWidgets('reduced-motion surfaces remain dismissible through the platform focus route', (tester) async {
    await tester.pumpWidget(app(Builder(builder: (context) => Center(child: TextButton(onPressed: () {
      showMotionSurface<void>(context, title: 'Alex', builder: (_) => const Text('Stored locally'));
    }, child: const Text('Open')))), reduced: true));
    await tester.tap(find.text('Open')); await tester.pumpAndSettle();
    expect(find.text('Stored locally'), findsOneWidget);
    await tester.tap(find.byTooltip('Close')); await tester.pumpAndSettle();
    expect(find.text('Stored locally'), findsNothing);
    expect(tester.binding.transientCallbackCount, 0);
  });

  testWidgets('state refresh preserves editable content and does not leave an entrance ticker', (tester) async {
    final text = TextEditingController(text: 'draft'); addTearDown(text.dispose);
    Widget fixture(String identity) => app(MotionReveal(identity: identity, child: TextField(controller: text)));
    await tester.pumpWidget(fixture('a')); await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextField), 'unchanged draft');
    await tester.pumpWidget(fixture('b')); await tester.pump(const Duration(milliseconds: 220));
    expect(text.text, 'unchanged draft');
    await tester.pumpWidget(const SizedBox.shrink());
    expect(tester.binding.transientCallbackCount, 0); expect(tester.takeException(), isNull);
  });
}
