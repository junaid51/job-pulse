// JobPulse — Flutter client.
//
// Three screens: Jobs, Notifications, Settings.
//
// TODO(M4): Firebase Messaging — register the token with POST /api/devices and
// refresh the feed when a push arrives.
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'router.dart';
import 'theme.dart';

void main() {
  runApp(const ProviderScope(child: JobPulseApp()));
}

class JobPulseApp extends ConsumerWidget {
  const JobPulseApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp.router(
      title: 'JobPulse',
      debugShowCheckedModeBanner: false,
      routerConfig: ref.watch(routerProvider),
      // Dark by default, light if that is what the system asks for.
      theme: buildTheme(Brightness.light),
      darkTheme: buildTheme(Brightness.dark),
      themeMode: ThemeMode.system,
    );
  }
}
