import 'dart:math' as math;

import 'package:flutter/material.dart';

import 'motion.dart';

class TransferGraph extends StatefulWidget {
  const TransferGraph({
    super.key,
    required this.value,
    required this.running,
    required this.completed,
    required this.cloud,
    required this.incoming,
    required this.localLabel,
    required this.peerLabel,
    required this.statusLabel,
    required this.detailsLabel,
    required this.onDetails,
  });
  final double value;
  final bool running, completed, cloud, incoming;
  final String localLabel, peerLabel, statusLabel, detailsLabel;
  final VoidCallback onDetails;
  @override
  State<TransferGraph> createState() => _TransferGraphState();
}

class _TransferGraphState extends State<TransferGraph>
    with TickerProviderStateMixin {
  late final AnimationController _flow = AnimationController(
    vsync: this,
    duration: PinelinkMotion.sheen,
  );
  late final AnimationController _boundary = AnimationController(
    vsync: this,
    duration: PinelinkMotion.quick,
  );
  late double _from = _value, _to = _value;
  bool _reduced = true, _active = true;
  double get _value => widget.value.isFinite ? widget.value.clamp(0.0, 1.0) : 0;
  double get _shown =>
      _from + (_to - _from) * Curves.easeOutCubic.transform(_boundary.value);
  bool get _moving =>
      widget.running && _to > 0 && _to < 1 && !_reduced && _active;
  @override
  void initState() {
    super.initState();
    _boundary.value = 1;
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduced = PinelinkMotion.reduced(context);
    _active = TickerMode.valuesOf(context).enabled;
    if (_reduced || !_active) {
      _boundary.stop();
      _from = _to;
      _boundary.value = 1;
    }
    _sync();
  }

  @override
  void didUpdateWidget(covariant TransferGraph oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (_value != _to) {
      _from = _shown;
      _to = _value;
      if (_reduced || !widget.running || !_active) {
        _boundary.stop();
        _from = _to;
        _boundary.value = 1;
      } else {
        _boundary.forward(from: 0);
      }
    }
    if (!widget.running) {
      _boundary.stop();
      _from = _to;
      _boundary.value = 1;
    }
    _sync();
  }

  void _sync() {
    if (_moving) {
      if (!_flow.isAnimating) _flow.repeat();
    } else {
      _flow.stop();
    }
  }

  @override
  void dispose() {
    _flow.dispose();
    _boundary.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, box) {
      final small = box.maxWidth < 640,
          peerSize = small ? 56.0 : 64.0,
          localSize = small ? 80.0 : 96.0;
      Widget node(
        IconData icon,
        double size,
        String label, {
        bool receipt = false,
      }) => SizedBox(
        width: math.min(240, box.maxWidth),
        child: Column(
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
                    color: Color(0xFFF5F5F5),
                  ),
                  child: Icon(
                    icon,
                    size: size / 2,
                    color: const Color(0xFF101010),
                  ),
                ),
                if (receipt)
                  const Positioned(
                    right: 0,
                    bottom: 0,
                    child: DecoratedBox(
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: Colors.white,
                      ),
                      child: Padding(
                        padding: EdgeInsets.all(3),
                        child: Icon(
                          Icons.check,
                          size: 14,
                          color: Color(0xFF0866FF),
                        ),
                      ),
                    ),
                  ),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(fontSize: small ? 12 : 13),
            ),
          ],
        ),
      );
      return Semantics(
        label: widget.statusLabel,
        value: '${(_value * 100).round()}%',
        child: SizedBox(
          height: 400,
          width: double.infinity,
          child: Stack(
            alignment: Alignment.topCenter,
            children: [
              Positioned.fill(
                child: ExcludeSemantics(
                  child: CustomPaint(
                    painter: _CurvePainter(
                      progress: () => _shown,
                      phase: () => _flow.value,
                      running: _moving,
                      incoming: widget.incoming,
                      repaint: Listenable.merge([_flow, _boundary]),
                    ),
                  ),
                ),
              ),
              Positioned(
                top: small ? 36 : 32,
                child: node(
                  widget.cloud
                      ? Icons.cloud_outlined
                      : Icons.smartphone_outlined,
                  peerSize,
                  widget.peerLabel,
                ),
              ),
              Positioned(
                top: 248,
                child: node(
                  Icons.computer_outlined,
                  localSize,
                  widget.localLabel,
                  receipt: widget.completed && widget.incoming,
                ),
              ),
              Positioned(
                top: 136,
                width: math.max(48, box.maxWidth * .4),
                height: 116,
                child: Semantics(
                  button: true,
                  label: widget.detailsLabel,
                  child: TextButton(
                    onPressed: widget.onDetails,
                    style: TextButton.styleFrom(
                      overlayColor: Colors.transparent,
                    ),
                    child: const SizedBox.expand(),
                  ),
                ),
              ),
            ],
          ),
        ),
      );
    },
  );
}

class _CurvePainter extends CustomPainter {
  _CurvePainter({
    required this.progress,
    required this.phase,
    required this.running,
    required this.incoming,
    required super.repaint,
  });
  final double Function() progress, phase;
  final bool running, incoming;
  @override
  void paint(Canvas canvas, Size size) {
    final x = size.width / 2;
    final path = Path();
    if (incoming) {
      path.moveTo(x, 140);
      path.cubicTo(size.width * .8, 176, size.width * .2, 212, x, 248);
    } else {
      path.moveTo(x, 248);
      path.cubicTo(size.width * .2, 212, size.width * .8, 176, x, 140);
    }
    final paint = Paint()
      ..color = const Color(0xFFE5E5E5)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round;
    canvas.drawPath(path, paint);
    final metric = path.computeMetrics().first,
        end = metric.length * progress().clamp(0, 1);
    canvas.drawPath(
      metric.extractPath(0, end),
      paint..color = const Color(0xFF0866FF),
    );
    if (running && end > 0) {
      final start = metric.length * phase(),
          finish = math.min(end, start + metric.length * .06);
      if (start < finish) {
        canvas.drawPath(
          metric.extractPath(start, finish),
          paint..color = const Color(0xFF80B1FF),
        );
      }
    }
  }

  @override
  bool shouldRepaint(covariant _CurvePainter oldDelegate) => true;
}
