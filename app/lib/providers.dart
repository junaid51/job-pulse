import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';
import 'models.dart';

final apiProvider = Provider<JobPulseApi>((ref) => JobPulseApi());

final profilesProvider = FutureProvider<List<Profile>>(
  (ref) => ref.watch(apiProvider).profiles(),
);

/// Which profile the Jobs screen is showing. Null means "the first one", so the
/// screen works before anything has been tapped.
class SelectedProfile extends Notifier<int?> {
  @override
  int? build() => null;

  void select(int? id) => state = id;
}

final selectedProfileProvider = NotifierProvider<SelectedProfile, int?>(SelectedProfile.new);

final jobsProvider = FutureProvider.family<List<Job>, int>(
  (ref, profileId) => ref.watch(apiProvider).jobs(profileId: profileId),
);

final notificationsProvider = FutureProvider<Notifications>(
  (ref) => ref.watch(apiProvider).notifications(),
);

/// refreshFeeds is what both pull-to-refresh and the refresh buttons do: ask
/// the backend to poll the boards, then refetch everything. The poll is best
/// effort — a deployed backend reserves that endpoint for its cron and answers
/// 401, in which case the refetch still picks up whatever the cron stored.
Future<void> refreshFeeds(WidgetRef ref) async {
  try {
    await ref.read(apiProvider).poll();
  } on Object {
    // Not being allowed to poll is not a reason to skip the refetch.
  }
  ref.invalidate(jobsProvider);
  ref.invalidate(notificationsProvider);
}

/// The badge on the Notifications tab. It reads the feed that screen already
/// loads rather than adding a second endpoint for a number.
final unreadProvider = Provider<int>((ref) => switch (ref.watch(notificationsProvider)) {
  AsyncData(:final value) => value.unread,
  _ => 0,
});
