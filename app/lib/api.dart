import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

import 'models.dart';

/// Where the backend lives.
///
/// Override it at launch — this is the one setting that changes per machine:
///
///   flutter run --dart-define=JOBPULSE_API=http://192.168.1.20:8080
///
/// A physical phone cannot reach "localhost", so running on a real device needs
/// your computer's address on the network.
String defaultApiBaseUrl() {
  const configured = String.fromEnvironment('JOBPULSE_API');
  if (configured.isNotEmpty) return configured;
  if (!kIsWeb && defaultTargetPlatform == TargetPlatform.android) {
    // 10.0.2.2 is how the Android emulator refers to the host machine.
    return 'http://10.0.2.2:8080';
  }
  return 'http://localhost:8080';
}

/// JobPulseApi is one method per endpoint, returning parsed models. Errors are
/// left to propagate: the screens render them, and hiding them behind a Result
/// type would only move the handling.
class JobPulseApi {
  JobPulseApi({Dio? dio, String? baseUrl})
    : dio =
          dio ??
          Dio(
            BaseOptions(
              baseUrl: baseUrl ?? defaultApiBaseUrl(),
              // Generous on purpose: a free host that has gone to sleep takes
              // up to a minute to wake, and the first request is the alarm.
              connectTimeout: const Duration(seconds: 60),
              receiveTimeout: const Duration(seconds: 60),
              contentType: Headers.jsonContentType,
            ),
          );

  final Dio dio;

  String get baseUrl => dio.options.baseUrl;

  Future<List<Profile>> profiles() async {
    final response = await dio.get<Map<String, dynamic>>('/api/profiles');
    return _list(response.data?['profiles']).map(Profile.fromJson).toList();
  }

  Future<Profile> createProfile({
    required String name,
    required List<String> keywords,
    required List<String> locations,
    required bool remoteOnly,
  }) async {
    final response = await dio.post<Map<String, dynamic>>(
      '/api/profiles',
      data: {
        'name': name,
        'keywords': keywords,
        'locations': locations,
        'remote_only': remoteOnly,
      },
    );
    return Profile.fromJson(response.data!['profile'] as Map<String, dynamic>);
  }

  Future<Profile> updateProfile({
    required int id,
    required String name,
    required List<String> keywords,
    required List<String> locations,
    required bool remoteOnly,
  }) async {
    final response = await dio.put<Map<String, dynamic>>(
      '/api/profiles/$id',
      data: {
        'name': name,
        'keywords': keywords,
        'locations': locations,
        'remote_only': remoteOnly,
      },
    );
    return Profile.fromJson(response.data!['profile'] as Map<String, dynamic>);
  }

  Future<void> deleteProfile(int id) => dio.delete('/api/profiles/$id');

  Future<List<Job>> jobs({required int profileId, int limit = 50}) async {
    final response = await dio.get<Map<String, dynamic>>(
      '/api/jobs',
      queryParameters: {'profile_id': profileId, 'limit': limit},
    );
    return _list(response.data?['jobs']).map(Job.fromJson).toList();
  }

  Future<Notifications> notifications({int limit = 50}) async {
    final response = await dio.get<Map<String, dynamic>>(
      '/api/notifications',
      queryParameters: {'limit': limit},
    );
    return Notifications(
      events: _list(response.data?['notifications']).map(MatchEvent.fromJson).toList(),
      unread: response.data?['unread'] as int? ?? 0,
    );
  }

  Future<void> markSeen() => dio.post('/api/notifications/seen');

  /// Registers this device's FCM token so the poller knows where to send.
  /// Called on every app start — tokens rotate, and re-registering is a no-op.
  Future<void> registerDevice({required String token, required String platform}) =>
      dio.post('/api/devices', data: {'token': token, 'platform': platform});

  /// Starts a poll cycle. The server answers immediately and keeps working, so
  /// the results of this call show up on the next fetch, not in its response.
  Future<void> poll() => dio.post('/api/poll');

  List<Map<String, dynamic>> _list(Object? value) => value is List
      ? value.cast<Map<String, dynamic>>()
      : const <Map<String, dynamic>>[];
}

/// A message worth putting on screen. Dio's own toString is a paragraph of
/// stack-trace noise.
String describeError(Object error) {
  if (error is DioException) {
    return switch (error.type) {
      DioExceptionType.connectionError ||
      DioExceptionType.connectionTimeout =>
        'Cannot reach the backend. On free hosting it may just be waking up — '
            'try again in a moment.',
      DioExceptionType.receiveTimeout ||
      DioExceptionType.sendTimeout => 'The backend took too long to answer.',
      DioExceptionType.badResponse =>
        error.response?.data is Map && (error.response!.data as Map)['error'] != null
            ? (error.response!.data as Map)['error'].toString()
            : 'The backend returned ${error.response?.statusCode}.',
      _ => 'Request failed.',
    };
  }
  return error.toString();
}
