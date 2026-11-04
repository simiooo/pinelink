import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/motion/motion.dart';
import 'package:pinelink_mobile/src/motion/transfer_graph.dart';

void main() {
  testWidgets('a screen reader can open details separately from progress', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    try {
      var opened = 0;
      await tester.pumpWidget(
        MaterialApp(
          home: MediaQuery(
            data: const MediaQueryData(disableAnimations: true),
            child: MotionScope(
              child: Scaffold(
                body: TransferGraph(
                  value: .42,
                  running: false,
                  completed: false,
                  cloud: false,
                  incoming: true,
                  localLabel: 'This device',
                  peerLabel: 'Alex',
                  statusLabel: 'Paused',
                  detailsLabel: 'Transfer details',
                  onDetails: () => opened++,
                ),
              ),
            ),
          ),
        ),
      );
      final details = tester.getSemantics(
        find.bySemanticsLabel('Transfer details'),
      );
      expect(details.getSemanticsData().hasAction(SemanticsAction.tap), isTrue);
      tester.binding.rootPipelineOwner.visitChildren((owner) {
        owner.semanticsOwner?.performAction(details.id, SemanticsAction.tap);
      });
      await tester.pump();
      expect(opened, 1);
      final progress = tester.getSemantics(find.bySemanticsLabel('Paused'));
      expect(progress.getSemanticsData().value, '42%');
    } finally {
      handle.dispose();
    }
  });
}
