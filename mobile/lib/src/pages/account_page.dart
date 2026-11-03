import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../auth/controller.dart';
import '../i18n/controller.dart';
import '../motion/motion.dart';
import '../theme/tokens.dart';

class AccountPage extends StatefulWidget {
  const AccountPage({super.key, required this.controller});
  final AccountController controller;
  @override
  State<AccountPage> createState() => _AccountPageState();
}

class _AccountPageState extends State<AccountPage> {
  final email = TextEditingController(),
      password = TextEditingController(),
      code = TextEditingController();
  final form = GlobalKey<FormState>();
  String page = 'login', error = '', done = '';
  bool busy = false, visible = false, googleWaiting = false, polling = false;
  late Timer timer;
  AccountController get auth => widget.controller;
  String c(String key) => tr(context, 'auth_$key');
  @override
  void initState() {
    super.initState();
    page = auth.signedIn
        ? 'account'
        : auth.pending?['challenge'] != null
        ? 'verify'
        : 'login';
    timer = Timer.periodic(const Duration(seconds: 2), (_) {
      if (!mounted) return;
      setState(() {});
      final state = (auth.pending?['challenge'] as Map?)?['delivery_state'];
      if (page == 'verify' &&
          !polling &&
          (state == 'pending' || state == 'retry')) {
        polling = true;
        unawaited(
          auth
              .refreshEmail()
              .catchError((_) {})
              .whenComplete(() => polling = false),
        );
      }
    });
  }

  @override
  void dispose() {
    timer.cancel();
    email.dispose();
    password.dispose();
    code.dispose();
    if (googleWaiting) unawaited(auth.cancelGoogle());
    super.dispose();
  }

  Future<void> run(Future<void> Function() action) async {
    if (busy) return;
    setState(() {
      busy = true;
      error = '';
    });
    try {
      await action();
    } catch (e) {
      if (mounted) {
        setState(
          () => error = e is AccountException ? e.code : 'AUTH_UNAVAILABLE',
        );
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  void go(String value) {
    setState(() {
      page = value;
      error = '';
      done = '';
      password.clear();
      code.clear();
    });
  }

  String errorText() {
    const codes = {
      'INVALID_EMAIL': 'emailInvalid',
      'INVALID_CREDENTIALS': 'credentials',
      'PASSWORD_LENGTH': 'passwordError',
      'INVALID_PASSWORD': 'passwordError',
      'PASSWORD_COMMON': 'weak',
      'PASSWORD_CHECK_UNAVAILABLE': 'checkUnavailable',
      'CODE_INVALID': 'codeInvalid',
      'CODE_REPLACED': 'replaced',
      'AUTH_EXPIRED': 'expired',
      'LINK_EXPIRED': 'invalidLink',
      'AUTH_RATE_LIMITED': 'rate',
      'AUTH_BUSY': 'rate',
      'RESEND_WAIT': 'rate',
      'AUTH_INVALID': 'expired',
      'DEVICE_REVOKED': 'revoked',
      'GOOGLE_CANCELLED': 'googleError',
      'SIGNOUT_PENDING': 'pendingLogout',
    };
    return c(codes[error] ?? 'unavailable');
  }

  Widget field(
    String label,
    TextEditingController controller, {
    bool fresh = false,
    bool secret = false,
    bool otp = false,
  }) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(
        label,
        style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
      ),
      const SizedBox(height: 8),
      Semantics(
        label: label,
        child: TextFormField(
          controller: controller,
          obscureText: secret && !visible,
          keyboardType: secret
              ? TextInputType.visiblePassword
              : otp
              ? TextInputType.number
              : TextInputType.emailAddress,
          autofillHints: secret
              ? [fresh ? AutofillHints.newPassword : AutofillHints.password]
              : otp
              ? const [AutofillHints.oneTimeCode]
              : const [AutofillHints.email],
          autocorrect: false,
          enableSuggestions: false,
          textCapitalization: TextCapitalization.none,
          inputFormatters: otp
              ? [
                  FilteringTextInputFormatter.digitsOnly,
                  LengthLimitingTextInputFormatter(8),
                ]
              : null,
          textInputAction: secret || otp
              ? TextInputAction.done
              : TextInputAction.next,
          onFieldSubmitted: (_) {
            if (!busy) unawaited(submit());
          },
          validator: (value) {
            if (value == null || value.isEmpty) return label;
            if (otp && value.length != 8) return c('codeInvalid');
            if (secret &&
                fresh &&
                (value.runes.length < 15 || value.runes.length > 128)) {
              return c('passwordError');
            }
            return null;
          },
          decoration: InputDecoration(
            filled: true,
            fillColor: const Color(0xFFF7F7F7),
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 16,
            ),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(14),
              borderSide: BorderSide.none,
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(14),
              borderSide: const BorderSide(color: PinelinkColors.accent),
            ),
            suffixIcon: secret
                ? IconButton(
                    tooltip: c(visible ? 'hide' : 'show'),
                    onPressed: () => setState(() => visible = !visible),
                    icon: Icon(
                      visible
                          ? Icons.visibility_off_outlined
                          : Icons.visibility_outlined,
                      size: 20,
                    ),
                  )
                : null,
          ),
        ),
      ),
      if (fresh) ...[
        const SizedBox(height: 8),
        Text(
          c('minimum'),
          style: const TextStyle(color: PinelinkColors.muted, fontSize: 12),
        ),
      ],
    ],
  );
  Widget primary(String label, Future<void> Function() onTap) => SizedBox(
    width: double.infinity,
    child: MotionButton.primary(
      busy: busy,
      onPressed: () => run(onTap),
      child: Text(label),
    ),
  );
  Widget link(String label, VoidCallback onTap) =>
      TextButton(onPressed: busy ? null : onTap, child: Text(label));
  Future<void> submit() async {
    if (!(form.currentState?.validate() ?? false)) return;
    await run(() async {
      if (page == 'login') {
        await auth.login(email.text, password.text);
        password.clear();
        if (mounted) Navigator.of(context).pop();
      } else if (page == 'register' || page == 'forgot') {
        await auth.startEmail(
          page == 'register' ? 'signup' : 'reset',
          email.text,
          password.text,
        );
        if (mounted) go('verify');
      } else if (page == 'verify') {
        final state = await auth.verifyCode(code.text);
        if (!mounted) return;
        if (state == 'ready') {
          Navigator.of(context).pop();
        } else if (state == 'reset') {
          go('reset');
        } else {
          setState(() => done = 'existing');
        }
      } else if (page == 'reset') {
        await auth.resetPassword(password.text);
        password.clear();
        if (mounted) setState(() => done = 'reset');
      }
    });
  }

  Future<void> google() async {
    setState(() => googleWaiting = true);
    await run(() async {
      await auth.googleLogin();
      if (mounted) Navigator.of(context).pop();
    });
    if (mounted) setState(() => googleWaiting = false);
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: auth,
    builder: (context, _) {
      final challenge = auth.pending?['challenge'] as Map<String, dynamic>?;
      final title = done.isNotEmpty
          ? c(done == 'reset' ? 'resetDone' : 'existing')
          : googleWaiting
          ? c('nativeWait')
          : c(switch (page) {
              'register' => 'create',
              'forgot' || 'reset' => 'reset',
              'verify' => 'verify',
              'account' => 'account',
              _ => 'signIn',
            });
      final fields = <Widget>[];
      if (done.isNotEmpty) {
        fields.addAll([
          const Icon(Icons.check, size: 28, color: PinelinkColors.accent),
          Text(c(done == 'reset' ? 'signInAgain' : 'existingHint')),
          link(c('back'), () => go('login')),
        ]);
      } else if (googleWaiting) {
        fields.addAll([
          if (auth.deviceCode.isNotEmpty)
            Text(
              '${auth.deviceCode.substring(0, 4)} ${auth.deviceCode.substring(4)}',
              style: const TextStyle(
                fontSize: 32,
                fontWeight: FontWeight.w800,
                letterSpacing: 4,
              ),
            ),
          const Center(child: MotionSpinner(size: 24)),
          TextButton(
            onPressed: () async {
              await auth.cancelGoogle();
              if (mounted) setState(() => googleWaiting = false);
            },
            child: Text(c('deny')),
          ),
        ]);
      } else if (page == 'account') {
        fields.addAll([
          Text(
            (auth.account['name'] as String? ?? '').isNotEmpty
                ? auth.account['name'] as String
                : auth.account['email'] as String? ?? c('noAccount'),
            style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
          ),
          if ((auth.account['name'] as String? ?? '').isNotEmpty)
            Text(
              auth.account['email'] as String? ?? c('local'),
              style: const TextStyle(color: PinelinkColors.muted),
            ),
          if (auth.signedIn)
            primary(c('signOut'), () async {
              final revoked = await auth.signOut();
              if (mounted) {
                go('login');
                if (!revoked) setState(() => error = 'SIGNOUT_PENDING');
              }
            })
          else
            primary(c('signIn'), () async => go('login')),
        ]);
      } else if (page == 'verify' && challenge != null) {
        final resendAt = DateTime.parse(challenge['resend_at']),
            seconds = resendAt.difference(DateTime.now()).inSeconds + 1;
        final delivery = challenge['delivery_state'];
        fields.addAll([
          const Icon(Icons.mail_outline, size: 28, color: PinelinkColors.muted),
          Text(
            auth.pending?['email'] as String? ?? '',
            style: const TextStyle(fontWeight: FontWeight.w600),
          ),
          Text(
            c(
              delivery == 'pending' || delivery == 'retry'
                  ? 'sending'
                  : delivery == 'failed'
                  ? 'mailFailed'
                  : delivery == 'conditional'
                  ? 'conditional'
                  : 'sent',
            ),
            style: const TextStyle(color: PinelinkColors.muted),
          ),
          Form(
            key: form,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                field(c('code'), code, otp: true),
                const SizedBox(height: 24),
                MotionButton.primary(
                  busy: busy,
                  onPressed: submit,
                  child: Text(c('confirm')),
                ),
              ],
            ),
          ),
          TextButton(
            onPressed: seconds > 0 || busy
                ? null
                : () => run(() => auth.refreshEmail(resend: true)),
            child: Text(
              seconds > 0
                  ? '${c('wait')} $seconds${c('seconds')}'
                  : c('resend'),
            ),
          ),
          link(
            c('emailChange'),
            () => go(challenge['purpose'] == 'reset' ? 'forgot' : 'register'),
          ),
        ]);
      } else {
        final register = page == 'register',
            forgot = page == 'forgot',
            reset = page == 'reset';
        if (!forgot && !reset && auth.capabilities['google'] == true) {
          fields.addAll([
            SizedBox(
              height: 48,
              child: MotionButton(
                busy: busy,
                onPressed: busy ? null : google,
                style: OutlinedButton.styleFrom(
                  backgroundColor: const Color(0xFFF2F2F2),
                  side: const BorderSide(color: PinelinkColors.line),
                ),
                child: Semantics(
                  label: c('googleLabel'),
                  child: Image.asset(
                    'assets/google-continue-neutral.png',
                    width: 180,
                    height: 40,
                    excludeFromSemantics: true,
                  ),
                ),
              ),
            ),
            Row(
              children: [
                const Expanded(child: Divider(color: Color(0xFFEDEDED))),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: Text(
                    c('or'),
                    style: const TextStyle(
                      fontSize: 12,
                      color: PinelinkColors.muted,
                    ),
                  ),
                ),
                const Expanded(child: Divider(color: Color(0xFFEDEDED))),
              ],
            ),
          ]);
        }
        fields.add(
          Form(
            key: form,
            child: AutofillGroup(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (!reset) field(c('email'), email),
                  if (!forgot) ...[
                    if (!reset) const SizedBox(height: 16),
                    field(
                      c(reset ? 'newPassword' : 'password'),
                      password,
                      secret: true,
                      fresh: register || reset,
                    ),
                  ],
                  const SizedBox(height: 24),
                  MotionButton.primary(
                    busy: busy,
                    onPressed:
                        (register || forgot) &&
                            auth.capabilities['email'] != true
                        ? null
                        : submit,
                    child: Text(
                      c(
                        forgot
                            ? 'send'
                            : register
                            ? 'create'
                            : reset
                            ? 'reset'
                            : 'signIn',
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        );
        fields.add(
          Wrap(
            alignment: WrapAlignment.spaceBetween,
            spacing: 24,
            children: [
              link(
                c(register || forgot || reset ? 'back' : 'create'),
                () => go(register || forgot || reset ? 'login' : 'register'),
              ),
              if (!forgot && !reset) link(c('forgot'), () => go('forgot')),
            ],
          ),
        );
      }
      return Scaffold(
        backgroundColor: Colors.white,
        appBar: AppBar(
          backgroundColor: Colors.white,
          title: const Text(
            'Pinelink',
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
          ),
          leading: IconButton(
            tooltip: c('deny'),
            icon: const Icon(Icons.close),
            onPressed: () => Navigator.of(context).pop(),
          ),
        ),
        body: SafeArea(
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 32, 20, 24),
            child: Center(
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 360),
                child: MotionPage(
                  identity: '$page/$done/$googleWaiting',
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text(
                        title,
                        style: const TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                      const SizedBox(height: 32),
                      if (error.isNotEmpty) ...[
                        Semantics(
                          liveRegion: true,
                          child: Text(
                            errorText(),
                            style: const TextStyle(color: PinelinkColors.text),
                          ),
                        ),
                        const SizedBox(height: 16),
                      ],
                      for (var i = 0; i < fields.length; i++) ...[
                        fields[i],
                        if (i < fields.length - 1) const SizedBox(height: 24),
                      ],
                      if (!auth.signedIn && !googleWaiting)
                        link(c('offline'), () => Navigator.of(context).pop()),
                      const SizedBox(height: 32),
                      Wrap(
                        alignment: WrapAlignment.center,
                        spacing: 24,
                        children: [
                          TextButton(
                            onPressed: () => showLegal(false),
                            child: Text(c('terms')),
                          ),
                          TextButton(
                            onPressed: () => showLegal(true),
                            child: Text(c('privacy')),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      );
    },
  );
  void showLegal(bool privacy) {
    showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(c(privacy ? 'privacy' : 'terms')),
        content: SingleChildScrollView(
          child: Text(c(privacy ? 'privacyText' : 'termsText')),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(c('confirm')),
          ),
        ],
      ),
    );
  }
}
