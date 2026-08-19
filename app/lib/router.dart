import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import 'providers.dart';
import 'push.dart';
import 'screens/jobs.dart';
import 'screens/notifications.dart';
import 'screens/settings.dart';

/// The router lives behind a provider rather than in a top-level final, so each
/// ProviderScope (the app, and each test) gets its own navigation state.
final routerProvider = Provider<GoRouter>((ref) => buildRouter());

/// Three routes behind one shell, so each tab keeps its scroll position.
GoRouter buildRouter() => GoRouter(
  initialLocation: '/jobs',
  routes: [
    StatefulShellRoute.indexedStack(
      builder: (context, state, shell) => _Shell(shell: shell),
      branches: [
        StatefulShellBranch(
          routes: [GoRoute(path: '/jobs', builder: (_, _) => const JobsScreen())],
        ),
        StatefulShellBranch(
          routes: [GoRoute(path: '/notifications', builder: (_, _) => const NotificationsScreen())],
        ),
        StatefulShellBranch(
          routes: [GoRoute(path: '/settings', builder: (_, _) => const SettingsScreen())],
        ),
      ],
    ),
  ],
);

class _Shell extends ConsumerWidget {
  const _Shell({required this.shell});

  final StatefulNavigationShell shell;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Watching starts the push pipeline (permission, token, registration) as a
    // side effect of the shell existing; the value itself is shown in Settings.
    ref.watch(pushProvider);
    final unread = ref.watch(unreadProvider);

    return Scaffold(
      // On a desktop browser the app would otherwise stretch into a
      // phone-layout the width of the monitor. A centered column with hairline
      // edges reads as intentional; on phones the constraint never engages.
      body: LayoutBuilder(
        builder: (context, constraints) {
          if (constraints.maxWidth <= 760) return shell;
          final colors = Theme.of(context).colorScheme;
          return Center(
            child: Container(
              constraints: const BoxConstraints(maxWidth: 760),
              decoration: BoxDecoration(
                border: Border.symmetric(
                  vertical: BorderSide(color: colors.outlineVariant),
                ),
              ),
              child: shell,
            ),
          );
        },
      ),
      bottomNavigationBar: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Divider(),
          NavigationBar(
            selectedIndex: shell.currentIndex,
            onDestinationSelected: (index) => shell.goBranch(
              index,
              // Tapping the current tab returns it to its first screen.
              initialLocation: index == shell.currentIndex,
            ),
            destinations: [
              const NavigationDestination(
                icon: Icon(Icons.work_outline, size: 22),
                selectedIcon: Icon(Icons.work, size: 22),
                label: 'Jobs',
              ),
              NavigationDestination(
                icon: Badge.count(
                  count: unread,
                  isLabelVisible: unread > 0,
                  child: const Icon(Icons.notifications_none, size: 22),
                ),
                selectedIcon: const Icon(Icons.notifications, size: 22),
                label: 'Notifications',
              ),
              const NavigationDestination(
                icon: Icon(Icons.settings_outlined, size: 22),
                selectedIcon: Icon(Icons.settings, size: 22),
                label: 'Settings',
              ),
            ],
          ),
        ],
      ),
    );
  }
}
