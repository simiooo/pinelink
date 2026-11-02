import 'package:flutter/material.dart';

enum CourierAction {
  run,
  pause,
  store,
  notify,
  pull,
  seal,
  persist,
  permission,
  deny,
  complete,
}

class Courier extends StatefulWidget {
  const Courier({
    super.key,
    required this.running,
    required this.label,
    this.action,
  });
  final bool running;
  final String label;
  final CourierAction? action;
  @override
  State<Courier> createState() => _CourierState();
}

class _CourierState extends State<Courier> with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1800),
  );

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _updateAnimation();
  }

  @override
  void didUpdateWidget(covariant Courier oldWidget) {
    super.didUpdateWidget(oldWidget);
    _updateAnimation();
  }

  void _updateAnimation() {
    final reduced = MediaQuery.maybeOf(context)?.disableAnimations ?? false;
    _controller.stop();
    if (reduced) {
      _controller.value = 1;
    } else if (widget.running) {
      _controller.duration = const Duration(milliseconds: 1800);
      _controller.repeat(reverse: true);
    } else if (widget.action != null) {
      _controller.duration = Duration(
        milliseconds: switch (widget.action!) {
          CourierAction.notify => 300,
          CourierAction.seal || CourierAction.deny => 200,
          CourierAction.pause || CourierAction.permission => 400,
          _ => 500,
        },
      );
      _controller.forward(from: 0);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Semantics(
    label: widget.label,
    image: true,
    child: AnimatedBuilder(
      animation: _controller,
      builder: (context, child) => Transform.translate(
        offset: Offset(0, widget.running ? -3 * _controller.value : 0),
        child: CustomPaint(
          size: const Size(124, 128),
          painter: CourierPainter(
            action:
                widget.action ??
                (widget.running ? CourierAction.run : CourierAction.pause),
            progress: _controller.value,
          ),
        ),
      ),
    ),
  );
}

class CourierPainter extends CustomPainter {
  const CourierPainter({required this.action, required this.progress});
  final CourierAction action;
  final double progress;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.scale(size.width / 240, size.height / 250);
    if (action == CourierAction.deny) {
      canvas.translate(-6 * progress * (1 - progress), 0);
    }
    if (action == CourierAction.permission) {
      canvas.translate(0, 4 * progress * (1 - progress));
    }
    if (action == CourierAction.persist) canvas.translate(0, -8 * progress);
    final paint = Paint()..isAntiAlias = true;
    void oval(Rect rect, Color color) =>
        canvas.drawOval(rect, paint..color = color);
    void rect(Rect rect, Color color, double radius) => canvas.drawRRect(
      RRect.fromRectAndRadius(rect, Radius.circular(radius)),
      paint..color = color,
    );
    if (action == CourierAction.store || action == CourierAction.persist) {
      rect(const Rect.fromLTWH(180, 30, 50, 42), const Color(0xFFE9E2D9), 12);
      oval(const Rect.fromLTWH(178, 17, 34, 34), const Color(0xFFE9E2D9));
      oval(const Rect.fromLTWH(203, 26, 26, 26), const Color(0xFFE9E2D9));
    }
    if (action == CourierAction.pull) {
      canvas.save();
      canvas.translate(0, 20 * (1 - progress));
    }
    if (action == CourierAction.notify) {
      canvas.drawCircle(
        const Offset(185, 100),
        40 + progress * 12,
        Paint()
          ..color = const Color(
            0xFFB86F3C,
          ).withAlpha(((1 - progress) * 100).round())
          ..style = PaintingStyle.stroke
          ..strokeWidth = 3,
      );
    }
    oval(const Rect.fromLTWH(0, 20, 140, 188), const Color(0xFFB86F3C));
    oval(const Rect.fromLTWH(24, 44, 88, 132), const Color(0xFFF5F6F7));
    rect(const Rect.fromLTWH(84, 132, 82, 90), const Color(0xFFF5F6F7), 0);
    oval(const Rect.fromLTWH(108, 130, 26, 26), const Color(0xFFB86F3C));
    oval(const Rect.fromLTWH(95, 106, 110, 124), const Color(0xFFC9844A));
    oval(const Rect.fromLTWH(116, 134, 68, 86), const Color(0xFFF2D4AC));
    oval(const Rect.fromLTWH(96, 34, 108, 108), const Color(0xFFC9844A));
    final ears = Path()
      ..moveTo(98, 60)
      ..lineTo(117, 14)
      ..lineTo(136, 60)
      ..close()
      ..moveTo(166, 60)
      ..lineTo(185, 14)
      ..lineTo(204, 60)
      ..close();
    canvas.drawPath(ears, paint..color = const Color(0xFFB86F3C));
    canvas.save();
    canvas.translate(149, 26);
    canvas.rotate(20 * 3.141592653589793 / 180);
    rect(const Rect.fromLTWH(-3, -15, 7, 30), const Color(0xFF6B4A34), 4);
    canvas.restore();
    oval(const Rect.fromLTWH(125, 79, 10, 14), const Color(0xFF30363D));
    oval(const Rect.fromLTWH(166, 79, 10, 14), const Color(0xFF30363D));
    for (final point in [
      const Offset(114, 105),
      const Offset(134, 115),
      const Offset(181, 105),
    ]) {
      canvas.save();
      canvas.translate(point.dx, point.dy);
      canvas.rotate(3.141592653589793 / 4);
      rect(const Rect.fromLTWH(-7, -7, 14, 14), const Color(0xFF6B4A34), 2);
      canvas.restore();
    }
    if (action == CourierAction.permission || action == CourierAction.deny) {
      final cap = Path()
        ..moveTo(100, 56)
        ..lineTo(200, 56)
        ..lineTo(185, 34)
        ..lineTo(120, 34)
        ..close();
      canvas.drawPath(cap, paint..color = const Color(0xFF6B4A34));
    }
    if (action == CourierAction.permission) {
      rect(const Rect.fromLTWH(184, 12, 44, 36), const Color(0xFFE7C79B), 6);
      for (var index = 0; index < 3; index++) {
        oval(
          Rect.fromLTWH(192.0 + index * 10, 25, 6, 6),
          const Color(0xFF6B4A34),
        );
      }
    }
    canvas.save();
    if (action == CourierAction.store || action == CourierAction.persist) {
      canvas.translate(12 * progress, -22 * progress);
    }
    rect(const Rect.fromLTWH(166, 143, 70, 48), const Color(0xFFE7C79B), 8);
    final seal = Path()
      ..moveTo(195, 158)
      ..lineTo(204, 167)
      ..lineTo(195, 176)
      ..lineTo(186, 167)
      ..close();
    canvas.drawPath(
      seal,
      paint
        ..color = action == CourierAction.seal
            ? const Color(0xFF6B4A34).withAlpha((255 * progress).round())
            : const Color(0xFF6B4A34),
    );
    canvas.restore();
    oval(const Rect.fromLTWH(103, 215, 32, 23), const Color(0xFFB86F3C));
    oval(const Rect.fromLTWH(158, 215, 32, 23), const Color(0xFFB86F3C));
    if (action == CourierAction.pull) canvas.restore();
  }

  @override
  bool shouldRepaint(covariant CourierPainter oldDelegate) =>
      oldDelegate.action != action || oldDelegate.progress != progress;
}
