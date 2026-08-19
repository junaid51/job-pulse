// Push notifications via Firebase Cloud Messaging.
//
// Everything here degrades to "no push" rather than failing: a platform without
// APNs, a denied permission prompt, or a fork not yet pointed at its own
// Firebase project all leave the rest of the app working — pull-to-refresh
// never depended on push.
import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'firebase_options.dart';
import 'providers.dart';

// The VAPID key identifies this app to browser push services. It is public by
// design — it ships to every browser; the private half never leaves Firebase.
const _vapidKey = String.fromEnvironment('JOBPULSE_VAPID');

/// initFirebase runs before runApp. It reports rather than throws, because a
/// platform missing from firebase_options.dart is a configuration state, not a
/// crash.
Future<bool> initFirebase() async {
  try {
    await Firebase.initializeApp(options: DefaultFirebaseOptions.currentPlatform);
    return true;
  } on Object catch (error) {
    debugPrint('push: firebase unavailable: $error');
    return false;
  }
}

/// PushController owns the pipeline: permission, token, registration with the
/// backend, and refreshing the feeds when a message arrives while the app is
/// open. Its value — is push live? — is what Settings shows.
///
/// Building it never prompts: browsers (and iOS Home-Screen apps especially)
/// only honor a permission request made from a tap, so the prompt lives behind
/// enable(), wired to a button in Settings. If permission was granted on an
/// earlier visit, build() quietly finishes the job.
final pushProvider = AsyncNotifierProvider<PushController, bool>(PushController.new);

class PushController extends AsyncNotifier<bool> {
  @override
  Future<bool> build() async {
    final messaging = FirebaseMessaging.instance;
    final settings = await messaging.getNotificationSettings();
    if (settings.authorizationStatus != AuthorizationStatus.authorized &&
        settings.authorizationStatus != AuthorizationStatus.provisional) {
      return false;
    }
    return _connect(messaging);
  }

  /// enable is the tap: ask for permission, then connect.
  Future<void> enable() async {
    state = const AsyncLoading();
    state = await AsyncValue.guard(() async {
      final messaging = FirebaseMessaging.instance;
      final settings = await messaging.requestPermission();
      if (settings.authorizationStatus == AuthorizationStatus.denied) {
        return false;
      }
      return _connect(messaging);
    });
  }

  Future<bool> _connect(FirebaseMessaging messaging) async {
    Future<void> register(String token) => ref.read(apiProvider).registerDevice(
      token: token,
      platform: kIsWeb ? 'web' : defaultTargetPlatform.name,
    );

    try {
      final token = await messaging.getToken(
        vapidKey: kIsWeb && _vapidKey.isNotEmpty ? _vapidKey : null,
      );
      if (token == null || token.isEmpty) return false;
      await register(token);
    } on Object catch (error) {
      // On native iOS without an APNs key this is where it stops — expected
      // until an Apple Developer enrollment. The app carries on without push.
      debugPrint('push: token unavailable: $error');
      return false;
    }

    messaging.onTokenRefresh.listen((token) {
      register(token).ignore();
    });

    // A push while the app is open refreshes the feeds instead of showing a
    // system banner: the new match appearing is the notification.
    FirebaseMessaging.onMessage.listen((_) {
      ref.invalidate(notificationsProvider);
    });

    return true;
  }
}
