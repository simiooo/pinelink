import 'package:flutter/material.dart';

import 'motion.dart';

Future<T?> showMotionSurface<T>(
  BuildContext context, {
  required String title,
  required WidgetBuilder builder,
  WidgetBuilder? titleBuilder,
  double horizontalPadding = 24,
}) {
  final content = _SurfaceContent(
    title: title,
    builder: builder,
    titleBuilder: titleBuilder,
    horizontalPadding: horizontalPadding,
  );
  if (MediaQuery.sizeOf(context).width < 640) {
    return showModalBottomSheet<T>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      backgroundColor: Colors.white,
      elevation: 2,
      barrierColor: Colors.black.withValues(alpha: .12),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      sheetAnimationStyle: PinelinkMotion.reduced(context)
          ? AnimationStyle.noAnimation
          : const AnimationStyle(
              duration: PinelinkMotion.surface,
              reverseDuration: PinelinkMotion.control,
            ),
      builder: (_) => content,
    );
  }
  return Navigator.of(context).push<T>(
    DialogRoute<T>(
      context: context,
      barrierColor: Colors.black.withValues(alpha: .12),
      animationStyle: PinelinkMotion.reduced(context)
          ? AnimationStyle.noAnimation
          : const AnimationStyle(
              duration: PinelinkMotion.control,
              reverseDuration: PinelinkMotion.quick,
            ),
      builder: (_) => Dialog(
        backgroundColor: Colors.white,
        elevation: 2,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(24),
          side: const BorderSide(color: Color(0xFFE5E5E5)),
        ),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 520),
          child: content,
        ),
      ),
    ),
  );
}

class _SurfaceContent extends StatelessWidget {
  const _SurfaceContent({
    required this.title,
    required this.builder,
    this.titleBuilder,
    required this.horizontalPadding,
  });
  final String title;
  final WidgetBuilder builder;
  final WidgetBuilder? titleBuilder;
  final double horizontalPadding;
  @override
  Widget build(BuildContext context) => SafeArea(
    top: false,
    child: ConstrainedBox(
      constraints: BoxConstraints(
        maxHeight: MediaQuery.sizeOf(context).height * .8,
      ),
      child: SingleChildScrollView(
        padding: EdgeInsets.fromLTRB(
          horizontalPadding,
          16,
          horizontalPadding,
          24 + MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                Expanded(
                  child: titleBuilder == null
                      ? Text(
                          title,
                          style: const TextStyle(
                            fontFamily: 'Nunito',
                            fontSize: 22,
                            fontWeight: FontWeight.w700,
                          ),
                        )
                      : Builder(builder: titleBuilder!),
                ),
                IconButton(
                  tooltip: MaterialLocalizations.of(context).closeButtonTooltip,
                  onPressed: () => Navigator.pop(context),
                  icon: const Icon(Icons.close),
                ),
              ],
            ),
            const SizedBox(height: 24),
            Builder(builder: builder),
          ],
        ),
      ),
    ),
  );
}
