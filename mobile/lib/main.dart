import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:pinelink_mobile/src/api/pinelink_api.dart';
import 'package:pinelink_mobile/src/components/app_shell.dart';
import 'package:pinelink_mobile/src/i18n/controller.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';
import 'package:pinelink_mobile/src/motion/motion.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final i18n = I18nController();
  await i18n.load();
  runApp(PinelinkApp(i18n: i18n));
}

class PinelinkApp extends StatefulWidget {
  const PinelinkApp({super.key, this.i18n, this.api});
  final I18nController? i18n;
  final PinelinkApi? api;

  @override
  State<PinelinkApp> createState() => _PinelinkAppState();
}

class _PinelinkAppState extends State<PinelinkApp> {
  late final I18nController _i18n = widget.i18n ?? I18nController();
  late final PinelinkApi _api = widget.api ?? PinelinkApi();

  @override
  void dispose() {
    if (widget.i18n == null) _i18n.dispose();
    if (widget.api == null) _api.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _i18n,
    builder: (context, _) => PinelinkI18n(
      controller: _i18n,
      child: MotionScope(
        child: MaterialApp(
          title: 'Pinelink',
          debugShowCheckedModeBanner: false,
          theme: pinelinkTheme(),
          locale: _i18n.locale,
          supportedLocales: const [Locale('zh', 'CN'), Locale('en', 'US')],
          localizationsDelegates: const [
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          home: PinelinkShell(api: _api),
        ),
      ),
    ),
  );
}
