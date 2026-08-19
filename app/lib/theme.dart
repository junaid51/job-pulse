import 'package:flutter/material.dart';

/// One accent colour, flat surfaces, hairline dividers, no shadows. The density
/// comes from the list rows rather than from small type.
const _accent = Color(0xFF6E6EF7);

ThemeData buildTheme(Brightness brightness) {
  final dark = brightness == Brightness.dark;

  final scheme = ColorScheme.fromSeed(seedColor: _accent, brightness: brightness).copyWith(
    primary: _accent,
    surface: dark ? const Color(0xFF0C0C0F) : Colors.white,
    onSurface: dark ? const Color(0xFFE8E8EC) : const Color(0xFF17171A),
    surfaceContainerHighest: dark ? const Color(0xFF16161B) : const Color(0xFFF5F5F7),
    onSurfaceVariant: dark ? const Color(0xFF8B8B96) : const Color(0xFF6B6B76),
    outlineVariant: dark ? const Color(0xFF22222A) : const Color(0xFFE6E6EA),
  );

  return ThemeData(
    colorScheme: scheme,
    scaffoldBackgroundColor: scheme.surface,
    dividerTheme: DividerThemeData(color: scheme.outlineVariant, thickness: 1, space: 1),
    appBarTheme: AppBarTheme(
      backgroundColor: scheme.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: TextStyle(
        color: scheme.onSurface,
        fontSize: 20,
        fontWeight: FontWeight.w600,
        letterSpacing: -0.4,
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: scheme.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      height: 62,
      indicatorColor: _accent.withValues(alpha: 0.16),
      labelTextStyle: WidgetStatePropertyAll(
        TextStyle(fontSize: 11.5, fontWeight: FontWeight.w500, color: scheme.onSurfaceVariant),
      ),
    ),
    chipTheme: ChipThemeData(
      backgroundColor: scheme.surface,
      selectedColor: _accent.withValues(alpha: 0.16),
      side: BorderSide(color: scheme.outlineVariant),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      labelStyle: TextStyle(fontSize: 13, fontWeight: FontWeight.w500, color: scheme.onSurface),
      showCheckmark: false,
    ),
    inputDecorationTheme: InputDecorationTheme(
      isDense: true,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(8),
        borderSide: BorderSide(color: scheme.outlineVariant),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(8),
        borderSide: BorderSide(color: scheme.outlineVariant),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    splashFactory: NoSplash.splashFactory,
    visualDensity: VisualDensity.compact,
  );
}
