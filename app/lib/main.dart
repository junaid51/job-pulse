// JobPulse — Flutter client.
//
// M1 is deliberately a boot placeholder: it proves the app compiles and runs on
// Android, iOS and web, and it puts ProviderScope in place so that adding state
// later is a pure addition.
//
// TODO(M3): GoRouter with the three screens (Jobs, Notifications, Settings).
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

void main() {
  runApp(const ProviderScope(child: JobPulseApp()));
}

class JobPulseApp extends StatelessWidget {
  const JobPulseApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'JobPulse',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(colorSchemeSeed: const Color(0xFF5B5BD6)),
      home: const Scaffold(
        body: Center(child: Text('JobPulse')),
      ),
    );
  }
}
