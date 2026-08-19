import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api.dart';
import '../providers.dart';
import '../widgets/job_tile.dart';
import '../widgets/states.dart';

/// Notifications: every match across every profile, newest first.
///
/// Opening the screen marks the feed as read. The dots stay visible for this
/// viewing because the list is not refetched — only the tab badge clears.
class NotificationsScreen extends ConsumerStatefulWidget {
  const NotificationsScreen({super.key});

  @override
  ConsumerState<NotificationsScreen> createState() => _NotificationsScreenState();
}

class _NotificationsScreenState extends ConsumerState<NotificationsScreen> {
  bool _marked = false;

  Future<void> _markSeenOnce(int unread) async {
    if (_marked || unread == 0) return;
    _marked = true;
    try {
      await ref.read(apiProvider).markSeen();
    } on Object {
      _marked = false; // let the next visit try again
    }
  }

  @override
  Widget build(BuildContext context) {
    final feed = ref.watch(notificationsProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Notifications'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            icon: const Icon(Icons.refresh, size: 22),
            onPressed: () {
              _marked = false;
              refreshFeeds(ref);
            },
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          _marked = false;
          ref.invalidate(notificationsProvider);
          await ref.read(notificationsProvider.future);
        },
        child: switch (feed) {
          AsyncError(:final error) => RefreshableState(
            child: ErrorView(
              message: describeError(error),
              onRetry: () => ref.invalidate(notificationsProvider),
            ),
          ),
          AsyncData(:final value) when value.events.isEmpty => const RefreshableState(
            child: EmptyView(
              title: 'No matches yet',
              detail:
                  'When the poller finds a job that fits one of your profiles, it lands '
                  'here — and pings your phone once push is set up.',
            ),
          ),
          AsyncData(:final value) => Builder(
            builder: (context) {
              // Fire and forget once the feed is on screen.
              WidgetsBinding.instance.addPostFrameCallback((_) => _markSeenOnce(value.unread));
              return ListView.separated(
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.only(bottom: 24),
                itemCount: value.events.length,
                separatorBuilder: (_, _) => const Divider(),
                itemBuilder: (_, i) {
                  final event = value.events[i];
                  return JobTile(job: event.job, label: event.profileName, showUnread: true);
                },
              );
            },
          ),
          _ => const LoadingView(),
        },
      ),
    );
  }
}
