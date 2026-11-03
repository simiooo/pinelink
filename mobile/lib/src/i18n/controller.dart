import 'package:flutter/widgets.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'messages.g.dart';

typedef TranslateParams = Map<String, Object>;

String translateLocale(
  Locale locale,
  String key, [
  TranslateParams params = const {},
]) {
  final messages = locale.languageCode == 'zh' ? zhMessages : enMessages;
  var result = messages[key] ?? zhMessages[key] ?? key;
  for (final entry in params.entries) {
    result = result.replaceAll('{${entry.key}}', '${entry.value}');
  }
  return result;
}

class I18nController extends ChangeNotifier with WidgetsBindingObserver {
  I18nController({String initialPreference = 'system'})
    : _preference = initialPreference {
    _updateLocale();
    WidgetsBinding.instance.addObserver(this);
  }

  String _preference;
  late Locale _locale;
  bool _hasSavedPreference = false;

  Locale get locale => _locale;
  String get preference => _preference;
  bool get hasSavedPreference => _hasSavedPreference;

  Future<void> load() async {
    final preferences = await SharedPreferences.getInstance();
    final stored = preferences.getString('pinelink.locale');
    if (stored == 'system' || stored == 'zh-CN' || stored == 'en-US') {
      _preference = stored!;
      _hasSavedPreference = true;
      _updateLocale();
      notifyListeners();
    }
  }

  Future<void> setPreference(String value) async {
    if (value != 'system' && value != 'zh-CN' && value != 'en-US') return;
    _preference = value;
    _hasSavedPreference = true;
    _updateLocale();
    notifyListeners();
    final preferences = await SharedPreferences.getInstance();
    await preferences.setString('pinelink.locale', value);
  }

  void _updateLocale() {
    if (_preference != 'system') {
      _locale = _preference == 'zh-CN'
          ? const Locale('zh', 'CN')
          : const Locale('en', 'US');
      return;
    }
    final system = WidgetsBinding.instance.platformDispatcher.locale;
    _locale = system.languageCode == 'zh'
        ? const Locale('zh', 'CN')
        : const Locale('en', 'US');
  }

  @override
  void didChangeLocales(List<Locale>? locales) {
    if (_preference == 'system') {
      _updateLocale();
      notifyListeners();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  String t(String key, [TranslateParams params = const {}]) =>
      translateLocale(_locale, key, params);
}

class PinelinkI18n extends InheritedWidget {
  const PinelinkI18n({
    super.key,
    required this.controller,
    required super.child,
  });
  final I18nController controller;

  static I18nController? of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<PinelinkI18n>()?.controller;

  @override
  bool updateShouldNotify(PinelinkI18n oldWidget) =>
      oldWidget.controller != controller ||
      oldWidget.controller.locale != controller.locale;
}

String tr(
  BuildContext context,
  String key, [
  TranslateParams params = const {},
]) {
  final controller = PinelinkI18n.of(context);
  return controller?.t(key, params) ??
      translateLocale(const Locale('zh', 'CN'), key, params);
}

Locale i18nLocale(BuildContext context) =>
    PinelinkI18n.of(context)?.locale ?? const Locale('zh', 'CN');
