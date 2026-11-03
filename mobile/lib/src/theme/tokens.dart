import 'package:flutter/material.dart';

class PinelinkColors {
  static const background = Color(0xFFFFFFFF);
  static const paper = Color(0xFFFFFFFF);
  static const text = Color(0xFF101010);
  static const muted = Color(0xFF6E6E6E);
  static const line = Color(0xFFDBDBDB);
  static const accent = Color(0xFF0866FF);
  static const soft = Color(0xFFEEF5FF);
}

ThemeData pinelinkTheme() => ThemeData(
  useMaterial3: true,
  fontFamily: 'Nunito',
  fontFamilyFallback: const ['Microsoft YaHei', 'Noto Sans SC'],
  popupMenuTheme: PopupMenuThemeData(
    color: Colors.white,
    surfaceTintColor: Colors.transparent,
    elevation: 2,
    shadowColor: const Color(0x10000000),
    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
  ),
  scaffoldBackgroundColor: PinelinkColors.background,
  colorScheme: const ColorScheme.light(
    primary: PinelinkColors.accent,
    onPrimary: Colors.white,
    secondary: PinelinkColors.accent,
    onSecondary: Colors.white,
    surface: PinelinkColors.paper,
    onSurface: PinelinkColors.text,
    outline: PinelinkColors.line,
    error: PinelinkColors.text,
    onError: Colors.white,
  ),
  textTheme: const TextTheme(
    headlineSmall: TextStyle(
      fontSize: 23,
      fontWeight: FontWeight.w700,
      color: PinelinkColors.text,
    ),
    titleMedium: TextStyle(
      fontSize: 17,
      fontWeight: FontWeight.w600,
      color: PinelinkColors.text,
    ),
    bodyMedium: TextStyle(fontSize: 14, color: PinelinkColors.text),
    bodySmall: TextStyle(fontSize: 12, color: PinelinkColors.muted),
  ),
  elevatedButtonTheme: ElevatedButtonThemeData(
    style: ElevatedButton.styleFrom(
      backgroundColor: PinelinkColors.accent,
      foregroundColor: Colors.white,
      elevation: 0,
      shadowColor: Colors.transparent,
      splashFactory: NoSplash.splashFactory,
      minimumSize: const Size(168, 48),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 12),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(24)),
      textStyle: const TextStyle(
        fontFamily: 'Nunito',
        fontSize: 15,
        fontWeight: FontWeight.w600,
      ),
    ),
  ),
  outlinedButtonTheme: OutlinedButtonThemeData(
    style: OutlinedButton.styleFrom(
      textStyle: const TextStyle(
        fontFamily: 'Nunito',
        fontSize: 15,
        fontWeight: FontWeight.w600,
      ),
      foregroundColor: PinelinkColors.text,
      backgroundColor: const Color(0xFFF5F5F5),
      minimumSize: const Size(48, 48),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 12),
      splashFactory: NoSplash.splashFactory,
      side: BorderSide.none,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(24)),
    ),
  ),
);
