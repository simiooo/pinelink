import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/widgets/courier.dart';

void main() {
  testWidgets('courier supports all state-bound actions and reduced motion', (
    tester,
  ) async {
    for (final action in CourierAction.values) {
      await tester.pumpWidget(
        MaterialApp(
          home: MediaQuery(
            data: const MediaQueryData(disableAnimations: true),
            child: Courier(
              running: action == CourierAction.run,
              action: action,
              label: '状态 $action',
            ),
          ),
        ),
      );
      await tester.pump(const Duration(milliseconds: 600));
      expect(find.bySemanticsLabel('状态 $action'), findsOneWidget);
      expect(tester.takeException(), isNull);
    }
    await tester.pumpWidget(
      const MaterialApp(home: Courier(running: true, label: '传输中')),
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
}
