import 'dart:math' as math;

import 'package:flutter/material.dart';

/// A relay has two independently confirmed progress ledgers. The curve for
/// each ledger starts at the true sender and ends at the true receiver.
class CloudTransferGraph extends StatelessWidget {
  const CloudTransferGraph({
    super.key,
    required this.spoolValue,
    required this.deliveryValue,
    required this.spoolRunning,
    required this.deliveryRunning,
    required this.incoming,
    required this.completed,
    required this.localLabel,
    required this.peerLabel,
    required this.cloudLabel,
    required this.spoolLabel,
    required this.deliveryLabel,
    required this.detailsLabel,
    required this.localIcon,
    required this.peerIcon,
    required this.peerDisconnected,
    required this.onDetails,
  });
  final double spoolValue, deliveryValue;
  final bool spoolRunning,
      deliveryRunning,
      incoming,
      completed,
      peerDisconnected;
  final String localLabel,
      peerLabel,
      cloudLabel,
      spoolLabel,
      deliveryLabel,
      detailsLabel;
  final IconData localIcon, peerIcon;
  final VoidCallback onDetails;

  double _fraction(double value) => value.isFinite ? value.clamp(0, 1) : 0;

  @override
  Widget build(BuildContext context) {
    final spool = _fraction(spoolValue), delivery = _fraction(deliveryValue);
    final reduced =
        MediaQuery.disableAnimationsOf(context) ||
        !TickerMode.valuesOf(context).enabled;
    Widget node(
      IconData icon,
      double size,
      String label, {
      bool receipt = false,
      bool disconnected = false,
    }) => Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Stack(
          clipBehavior: Clip.none,
          children: [
            Container(
              width: size,
              height: size,
              decoration: const BoxDecoration(
                shape: BoxShape.circle,
                color: Color(0xfff5f5f5),
              ),
              child: Icon(icon, size: size / 2, color: const Color(0xff101010)),
            ),
            if (receipt || disconnected)
              Positioned(
                right: 0,
                bottom: 0,
                child: DecoratedBox(
                  decoration: const BoxDecoration(
                    shape: BoxShape.circle,
                    color: Colors.white,
                  ),
                  child: Padding(
                    padding: const EdgeInsets.all(3),
                    child: Icon(
                      disconnected ? Icons.wifi_off_outlined : Icons.check,
                      size: 14,
                      color: disconnected
                          ? const Color(0xff737373)
                          : const Color(0xff0866ff),
                    ),
                  ),
                ),
              ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          label,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontSize: 13),
        ),
      ],
    );
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = constraints.maxWidth;
        return SizedBox(
          width: width,
          height: 500,
          child: Stack(
            alignment: Alignment.topCenter,
            children: [
              Positioned.fill(
                child: ExcludeSemantics(
                  child: TweenAnimationBuilder<double>(
                    tween: Tween(end: spool),
                    duration: reduced || !spoolRunning
                        ? Duration.zero
                        : const Duration(milliseconds: 180),
                    builder: (context, animatedSpool, _) =>
                        TweenAnimationBuilder<double>(
                          tween: Tween(end: delivery),
                          duration: reduced || !deliveryRunning
                              ? Duration.zero
                              : const Duration(milliseconds: 180),
                          builder: (context, animatedDelivery, _) =>
                              CustomPaint(
                                painter: _CloudCurves(
                                  spool: animatedSpool,
                                  delivery: animatedDelivery,
                                  incoming: incoming,
                                ),
                              ),
                        ),
                  ),
                ),
              ),
              Positioned(
                top: 16,
                child: node(
                  peerIcon,
                  56,
                  peerLabel,
                  disconnected: peerDisconnected,
                ),
              ),
              Positioned(
                top: 214,
                child: node(Icons.cloud_outlined, 56, cloudLabel),
              ),
              Positioned(
                top: 398,
                child: node(
                  localIcon,
                  80,
                  localLabel,
                  receipt: completed && incoming,
                ),
              ),
              Positioned(
                top: 95,
                height: 280,
                width: math.max(48, width * .35),
                child: Semantics(
                  button: true,
                  label: detailsLabel,
                  onTap: onDetails,
                  child: TextButton(
                    onPressed: onDetails,
                    style: TextButton.styleFrom(
                      overlayColor: Colors.transparent,
                    ),
                    child: const SizedBox.expand(),
                  ),
                ),
              ),
              Positioned(
                left: 0,
                top: 0,
                child: Semantics(
                  container: true,
                  label: spoolLabel,
                  value: '${(spool * 100).round()}%',
                  child: const SizedBox(width: 1, height: 1),
                ),
              ),
              Positioned(
                left: 0,
                top: 0,
                child: Semantics(
                  container: true,
                  label: deliveryLabel,
                  value: '${(delivery * 100).round()}%',
                  child: const SizedBox(width: 1, height: 1),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class _CloudCurves extends CustomPainter {
  const _CloudCurves({
    required this.spool,
    required this.delivery,
    required this.incoming,
  });
  final double spool, delivery;
  final bool incoming;
  @override
  void paint(Canvas canvas, Size size) {
    final x = size.width / 2;
    Path upper(bool reverse) => Path()
      ..moveTo(x, reverse ? 242 : 44)
      ..cubicTo(
        size.width * .08,
        reverse ? 190 : 96,
        size.width * .08,
        reverse ? 96 : 190,
        x,
        reverse ? 44 : 242,
      );
    Path lower(bool reverse) => Path()
      ..moveTo(x, reverse ? 438 : 242)
      ..cubicTo(
        size.width * .92,
        reverse ? 386 : 294,
        size.width * .92,
        reverse ? 294 : 386,
        x,
        reverse ? 242 : 438,
      );
    void draw(Path path, double value) {
      final paint = Paint()
        ..color = const Color(0xffe5e5e5)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 3
        ..strokeCap = StrokeCap.round;
      canvas.drawPath(path, paint);
      final metric = path.computeMetrics().first;
      canvas.drawPath(
        metric.extractPath(0, metric.length * value),
        paint
          ..color = const Color(0xff0866ff)
          ..strokeWidth = 4,
      );
    }

    if (incoming) {
      draw(upper(false), spool);
      draw(lower(false), delivery);
    } else {
      draw(lower(true), spool);
      draw(upper(true), delivery);
    }
  }

  @override
  bool shouldRepaint(covariant _CloudCurves old) =>
      old.spool != spool ||
      old.delivery != delivery ||
      old.incoming != incoming;
}
