import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/physics.dart';

abstract final class PinelinkMotion {
  static const press = Duration(milliseconds: 80);
  static const quick = Duration(milliseconds: 120);
  static const control = Duration(milliseconds: 160);
  static const surface = Duration(milliseconds: 220);
  static const page = Duration(milliseconds: 280);
  static const sheen = Duration(milliseconds: 1800);
  static const spring = SpringDescription(mass: 1, stiffness: 420, damping: 34);
  static bool reduced(BuildContext context) =>
      MediaQuery.disableAnimationsOf(context);
  static Duration duration(BuildContext context, Duration normal) =>
      reduced(context) ? Duration.zero : normal;
  static AnimationStyle menu(BuildContext context) => reduced(context)
      ? AnimationStyle.noAnimation
      : const AnimationStyle(
          duration: control,
          reverseDuration: quick,
          curve: Curves.easeOutCubic,
        );
}

class MotionScope extends StatefulWidget {
  const MotionScope({super.key, required this.child});
  final Widget child;
  @override
  State<MotionScope> createState() => _MotionScopeState();
}

class _MotionScopeState extends State<MotionScope> with WidgetsBindingObserver {
  bool _active = true;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    final state = WidgetsBinding.instance.lifecycleState;
    _active = state == null || state == AppLifecycleState.resumed;
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (mounted && _active != (state == AppLifecycleState.resumed)) {
      setState(() => _active = state == AppLifecycleState.resumed);
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) =>
      TickerMode(enabled: _active, child: widget.child);
}

class MotionButton extends StatefulWidget {
  const MotionButton({
    super.key,
    required this.onPressed,
    required this.child,
    this.primary = false,
    this.busy = false,
    this.style,
  });
  const MotionButton.primary({
    super.key,
    required this.onPressed,
    required this.child,
    this.busy = false,
    this.style,
  }) : primary = true;
  final FutureOr<void> Function()? onPressed;
  final Widget child;
  final bool primary;
  final bool busy;
  final ButtonStyle? style;
  @override
  State<MotionButton> createState() => _MotionButtonState();
}

class _MotionButtonState extends State<MotionButton>
    with SingleTickerProviderStateMixin {
  late final AnimationController _scale = AnimationController.unbounded(
    vsync: this,
    value: 1,
  );
  final WidgetStatesController _states = WidgetStatesController();
  bool _pending = false, _reduced = true, _active = true, _pressed = false;
  @override
  void initState() {
    super.initState();
    _states.addListener(_changed);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduced = PinelinkMotion.reduced(context);
    _active = TickerMode.valuesOf(context).enabled;
    if (_reduced || !_active) {
      _scale.stop();
      _scale.value = 1;
    }
  }

  void _changed() {
    final down = _states.value.contains(WidgetState.pressed);
    if (_pressed == down) return;
    _pressed = down;
    if (_reduced || !_active) {
      _scale.stop();
      _scale.value = 1;
      return;
    }
    if (down) {
      _scale.animateTo(
        .97,
        duration: PinelinkMotion.press,
        curve: Curves.easeOutCubic,
      );
    } else {
      _scale.animateWith(
        SpringSimulation(
          PinelinkMotion.spring,
          _scale.value,
          1,
          _scale.velocity,
        ),
      );
    }
  }

  Future<void> _invoke() async {
    if (_pending || widget.busy || widget.onPressed == null) return;
    setState(() => _pending = true);
    try {
      await widget.onPressed!();
    } finally {
      if (mounted) setState(() => _pending = false);
    }
  }

  @override
  void dispose() {
    _states.removeListener(_changed);
    _states.dispose();
    _scale.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final busy = _pending || widget.busy;
    final style = (widget.style ?? const ButtonStyle()).copyWith(
      animationDuration: PinelinkMotion.duration(context, PinelinkMotion.quick),
    );
    final content = Stack(
      alignment: Alignment.center,
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20),
          child: widget.child,
        ),
        if (busy) const Positioned(left: 0, child: MotionSpinner(size: 14)),
      ],
    );
    final button = widget.primary
        ? ElevatedButton(
            statesController: _states,
            style: style,
            onPressed: widget.onPressed == null || busy ? null : _invoke,
            child: content,
          )
        : OutlinedButton(
            statesController: _states,
            style: style,
            onPressed: widget.onPressed == null || busy ? null : _invoke,
            child: content,
          );
    return AnimatedBuilder(
      animation: _scale,
      child: button,
      builder: (context, child) => Transform.scale(
        scale: _scale.value.clamp(.94, 1.02),
        transformHitTests: false,
        child: child,
      ),
    );
  }
}

class MotionSpinner extends StatefulWidget {
  const MotionSpinner({super.key, this.size = 16});
  final double size;
  @override
  State<MotionSpinner> createState() => _MotionSpinnerState();
}

class _MotionSpinnerState extends State<MotionSpinner>
    with SingleTickerProviderStateMixin {
  late final AnimationController _turn = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1200),
  );
  bool _moving = false;
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _moving =
        !PinelinkMotion.reduced(context) &&
        TickerMode.valuesOf(context).enabled;
    if (_moving) {
      if (!_turn.isAnimating) _turn.repeat();
    } else {
      _turn.stop();
      _turn.value = 0;
    }
  }

  @override
  void dispose() {
    _turn.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: SizedBox(
      width: widget.size,
      height: widget.size,
      child: RotationTransition(
        turns: _turn,
        child: Icon(Icons.sync_rounded, size: widget.size),
      ),
    ),
  );
}

class MotionReveal extends StatefulWidget {
  const MotionReveal({
    super.key,
    required this.child,
    this.identity,
    this.distance = 8,
    this.duration = PinelinkMotion.control,
  });
  final Widget child;
  final Object? identity;
  final double distance;
  final Duration duration;
  @override
  State<MotionReveal> createState() => _MotionRevealState();
}

class _MotionRevealState extends State<MotionReveal>
    with SingleTickerProviderStateMixin {
  late final AnimationController _entry = AnimationController(
    vsync: this,
    duration: widget.duration,
    value: 0,
  );
  bool _started = false;
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (PinelinkMotion.reduced(context) ||
        !TickerMode.valuesOf(context).enabled) {
      _entry.stop();
      _entry.value = 1;
    } else if (!_started) {
      _entry.forward();
    }
    _started = true;
  }

  @override
  void didUpdateWidget(covariant MotionReveal oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.identity != widget.identity) {
      if (PinelinkMotion.reduced(context) ||
          !TickerMode.valuesOf(context).enabled) {
        _entry.value = 1;
      } else {
        if (!_entry.isAnimating) _entry.value = .25;
        _entry.forward();
      }
    }
  }

  @override
  void dispose() {
    _entry.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _entry,
    child: widget.child,
    builder: (context, child) {
      final value = Curves.easeOutCubic.transform(_entry.value);
      return Transform.translate(
        offset: Offset(0, widget.distance * (1 - value)),
        transformHitTests: false,
        child: Opacity(opacity: .25 + .75 * value, child: child),
      );
    },
  );
}

class MotionStateIcon extends StatelessWidget {
  const MotionStateIcon({
    super.key,
    required this.identity,
    required this.icon,
    required this.label,
    this.color,
  });
  final Object identity;
  final IconData icon;
  final String label;
  final Color? color;
  @override
  Widget build(BuildContext context) => Semantics(
    label: label,
    image: true,
    child: Tooltip(
      message: label,
      excludeFromSemantics: true,
      waitDuration: const Duration(milliseconds: 350),
      child: ExcludeSemantics(
        child: AnimatedSwitcher(
          duration: PinelinkMotion.duration(context, PinelinkMotion.quick),
          child: Icon(icon, key: ValueKey(identity), color: color, size: 20),
        ),
      ),
    ),
  );
}

class MotionMeter extends StatelessWidget {
  const MotionMeter({super.key, required this.value, required this.label});
  final double value;
  final String label;
  @override
  Widget build(BuildContext context) {
    final bounded = value.isFinite ? value.clamp(0.0, 1.0) : 0.0;
    return Semantics(
      label: label,
      value: '${(bounded * 100).round()}%',
      child: ExcludeSemantics(
        child: TweenAnimationBuilder<double>(
          tween: Tween(end: bounded),
          duration: PinelinkMotion.duration(context, PinelinkMotion.quick),
          curve: Curves.easeOutCubic,
          builder: (context, value, _) => LinearProgressIndicator(
            value: value,
            minHeight: 3,
            backgroundColor: const Color(0xFFE5E5E5),
            color: const Color(0xFF0866FF),
          ),
        ),
      ),
    );
  }
}

class MotionPage extends StatelessWidget {
  const MotionPage({super.key, required this.identity, required this.child});
  final Object identity;
  final Widget child;
  @override
  Widget build(BuildContext context) => MotionReveal(
    identity: identity,
    distance: 16,
    duration: PinelinkMotion.page,
    child: child,
  );
}
