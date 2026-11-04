import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pinelink_mobile/src/motion/cloud_transfer_graph.dart';

Widget graph({
  required bool incoming,
  required double spool,
  required double delivery,
  bool completed = false,
}) => MaterialApp(
  home: Scaffold(
    body: Center(
      child: RepaintBoundary(
        key: const ValueKey('graph'),
        child: SizedBox(
          width: 320,
          child: CloudTransferGraph(
            incoming: incoming,
            spoolValue: spool,
            deliveryValue: delivery,
            spoolRunning: false,
            deliveryRunning: false,
            completed: completed,
            localLabel: 'This device',
            peerLabel: 'Other device',
            cloudLabel: 'Cloud',
            spoolLabel: 'Cloud copy',
            deliveryLabel: 'Delivered',
            detailsLabel: 'Details',
            localIcon: Icons.computer,
            peerIcon: Icons.phone,
            peerDisconnected: false,
            onDetails: () {},
          ),
        ),
      ),
    ),
  ),
);

Future<({int upper, int lower})> bluePixels(WidgetTester tester) async {
  final boundary = tester.renderObject<RenderRepaintBoundary>(
    find.byKey(const ValueKey('graph')),
  );
  return (await tester.runAsync(() async {
    final image = await boundary.toImage(pixelRatio: 1);
    final bytes = (await image.toByteData(format: ui.ImageByteFormat.rawRgba))!;
    var upper = 0, lower = 0;
    for (var y = 75; y < 395; y++) {
      for (var x = 0; x < image.width; x++) {
        final offset = (y * image.width + x) * 4;
        final r = bytes.getUint8(offset),
            g = bytes.getUint8(offset + 1),
            b = bytes.getUint8(offset + 2);
        if (r < 80 && g > 65 && g < 160 && b > 220) {
          if (y < 195) upper++;
          if (y > 280) lower++;
        }
      }
    }
    image.dispose();
    return (upper: upper, lower: lower);
  }))!;
}

void main() {
  testWidgets(
    'cloud and delivery progress use separate legs in the actual direction',
    (tester) async {
      await tester.pumpWidget(graph(incoming: true, spool: 1, delivery: 0));
      await tester.pumpAndSettle();
      final incoming = await bluePixels(tester);
      expect(incoming.upper, greaterThan(100));
      expect(incoming.lower, 0);
      expect(find.text('Other device'), findsOneWidget);
      expect(find.text('Cloud'), findsOneWidget);
      expect(find.text('This device'), findsOneWidget);
      expect(find.byIcon(Icons.check), findsNothing);

      await tester.pumpWidget(graph(incoming: false, spool: 1, delivery: 0));
      await tester.pumpAndSettle();
      final outgoing = await bluePixels(tester);
      expect(outgoing.lower, greaterThan(100));
      expect(outgoing.upper, 0);
      expect(find.byIcon(Icons.check), findsNothing);
    },
  );

  testWidgets(
    'independent progress is exposed to screen readers and receipt requires completion',
    (tester) async {
      final handle = tester.ensureSemantics();
      await tester.pumpWidget(graph(incoming: true, spool: 1, delivery: .25));
      await tester.pumpAndSettle();
      expect(
        tester.getSemantics(find.bySemanticsLabel('Cloud copy')).value,
        '100%',
      );
      expect(
        tester.getSemantics(find.bySemanticsLabel('Delivered')).value,
        '25%',
      );
      expect(find.byIcon(Icons.check), findsNothing);
      await tester.pumpWidget(
        graph(incoming: true, spool: 1, delivery: 1, completed: true),
      );
      await tester.pumpAndSettle();
      expect(find.byIcon(Icons.check), findsOneWidget);
    handle.dispose();
    },
  );
}
