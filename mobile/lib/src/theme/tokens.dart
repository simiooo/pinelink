import 'package:flutter/material.dart';

class PinelinkColors {
  static const background = Color(0xFFF5F6F7);
  static const paper = Color(0xFFFFFFFF);
  static const text = Color(0xFF30363D);
  static const muted = Color(0xFF626C76);
  static const line = Color(0xFFE2E6EA);
  static const accent = Color(0xFF6B4A34);
  static const soft = Color(0xFFE9E2D9);
}

ThemeData pinelinkTheme() => ThemeData(
  useMaterial3: true,
  scaffoldBackgroundColor: PinelinkColors.background,
  colorScheme: ColorScheme.fromSeed(
    seedColor: PinelinkColors.accent,
    surface: PinelinkColors.paper,
  ),
  textTheme: const TextTheme(
    headlineSmall: TextStyle(
      fontSize: 23,
      fontWeight: FontWeight.w600,
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
      minimumSize: const Size(168, 46),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      textStyle: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
    ),
  ),
  outlinedButtonTheme: OutlinedButtonThemeData(
    style: OutlinedButton.styleFrom(
      foregroundColor: PinelinkColors.text,
      side: const BorderSide(color: PinelinkColors.line),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
    ),
  ),
);
