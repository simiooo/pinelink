import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pinelink_mobile/src/auth/controller.dart';
import 'package:pinelink_mobile/src/pages/account_page.dart';
import 'package:pinelink_mobile/src/i18n/controller.dart';
import 'package:pinelink_mobile/src/theme/tokens.dart';

import 'account_controller_test.dart' show MemoryAuthVault;

void main() {
  testWidgets(
    'account fields have accessible names and revealing a password never submits',
    (tester) async {
      SharedPreferences.setMockInitialValues({});
      var requests = 0;
      final auth = AccountController(
        server: 'https://unit.invalid/api',
        vault: MemoryAuthVault(),
        client: MockClient((_) async {
          requests++;
          return http.Response('{}', 503);
        }),
      );
      final locale = I18nController(initialPreference: 'en-US');
      final semantics = tester.ensureSemantics();
      await tester.pumpWidget(
        PinelinkI18n(
          controller: locale,
          child: MaterialApp(
            theme: pinelinkTheme(),
            home: MediaQuery(
              data: const MediaQueryData(disableAnimations: true),
              child: AccountPage(controller: auth),
            ),
          ),
        ),
      );
      await tester.pump();
      expect(find.bySemanticsLabel(RegExp('Email')), findsWidgets);
      expect(find.bySemanticsLabel(RegExp('Password')), findsWidgets);
      await tester.tap(find.byTooltip('Show password'));
      await tester.pump();
      expect(
        tester.widget<EditableText>(find.byType(EditableText).last).obscureText,
        false,
      );
      expect(requests, 0);
      await tester.pumpWidget(const SizedBox());
      semantics.dispose();
      auth.dispose();
      locale.dispose();
    },
  );
}
