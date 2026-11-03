import 'package:flutter/material.dart';
import 'package:pinelink_mobile/src/auth/controller.dart';
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
  final auth = AccountController(
    server: Uri.base
        .resolve(
          const String.fromEnvironment(
            'PINELINK_API_URL',
            defaultValue: 'https://pinelink.sqmem.top/api',
          ),
        )
        .toString()
        .replaceAll(RegExp(r'/$'), ''),
  );
  runApp(_AccountBootstrap(i18n: i18n, auth: auth, loading: auth.load()));
}

class PinelinkApp extends StatefulWidget {
  const PinelinkApp({super.key, this.i18n, this.api, this.auth});
  final I18nController? i18n;
  final PinelinkApi? api;
  final AccountController? auth;

  @override
  State<PinelinkApp> createState() => _PinelinkAppState();
}

class _AccountBootstrap extends StatelessWidget {
  const _AccountBootstrap({
    required this.i18n,
    required this.auth,
    required this.loading,
  });
  final I18nController i18n;
  final AccountController auth;
  final Future<void> loading;
  @override
  Widget build(BuildContext context) => FutureBuilder<void>(
    future: loading,
    builder: (context, state) => state.connectionState == ConnectionState.done
        ? PinelinkApp(i18n: i18n, auth: auth)
        : MaterialApp(
            debugShowCheckedModeBanner: false,
            theme: pinelinkTheme(),
            home: const Scaffold(
              body: Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      'Pinelink',
                      style: TextStyle(
                        fontSize: 28,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    SizedBox(height: 24),
                    MotionSpinner(size: 24),
                  ],
                ),
              ),
            ),
          ),
  );
}

class _PinelinkAppState extends State<PinelinkApp> {
  late final I18nController _i18n = widget.i18n ?? I18nController();
  late PinelinkApi _api = widget.api ?? _accountApi();
  late int _accountRevision = widget.auth?.profileRevision ?? 0;
  PinelinkApi _accountApi() => PinelinkApi(
    baseUrl: widget.auth?.server,
    account: widget.auth,
    openRuntime: widget.auth?.openRuntime,
  );
  @override
  void initState() {
    super.initState();
    widget.auth?.beforeProfileChange = () => _api.close();
    widget.auth?.addListener(_accountChanged);
  }

  void _accountChanged() {
    final next = widget.auth!.profileRevision;
    if (next == _accountRevision || !mounted) return;
    final old = _api;
    setState(() {
      _accountRevision = next;
      _api = _accountApi();
    });
    WidgetsBinding.instance.addPostFrameCallback((_) {
      old.dispose();
    });
  }

  @override
  void dispose() {
    widget.auth?.removeListener(_accountChanged);
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
          home: widget.auth == null
              ? PinelinkShell(api: _api)
              : AccountScope(
                  controller: widget.auth!,
                  child: PinelinkShell(
                    key: ValueKey(_accountRevision),
                    api: _api,
                  ),
                ),
        ),
      ),
    ),
  );
}
