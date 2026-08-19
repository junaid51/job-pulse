// JobPulse — Flutter client.
//
// Three screens: Jobs, Notifications, Settings.
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'push.dart';
import 'router.dart';
import 'theme.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await initFirebase();
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
