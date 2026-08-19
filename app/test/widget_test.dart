import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:jobpulse/api.dart';
import 'package:jobpulse/main.dart';
import 'package:jobpulse/models.dart';
import 'package:jobpulse/providers.dart';

/// A stand-in for the backend. The screens only ever reach the network through
/// JobPulseApi, so overriding it is enough to run the whole app offline.
class FakeApi extends JobPulseApi {
  FakeApi({this.profileList = const [], this.jobList = const []}) : super(baseUrl: 'http://fake');

  final List<Profile> profileList;
  final List<Job> jobList;

  @override
  Future<List<Profile>> profiles() async => profileList;

  @override
  Future<List<Job>> jobs({required int profileId, int limit = 50}) async => jobList;

  @override
  Future<Notifications> notifications({int limit = 50}) async =>
      Notifications(events: const [], unread: jobList.length);
}

Widget app(FakeApi api) => ProviderScope(
  overrides: [apiProvider.overrideWithValue(api)],
  child: const JobPulseApp(),
);

void main() {
  testWidgets('boots to Jobs with all three tabs', (tester) async {
    await tester.pumpWidget(app(FakeApi()));
    await tester.pumpAndSettle();

    expect(find.text('Jobs'), findsWidgets);
    expect(find.text('Notifications'), findsWidgets);
    expect(find.text('Settings'), findsWidgets);
  });

  testWidgets('with no profiles it says where to make one', (tester) async {
    await tester.pumpWidget(app(FakeApi()));
    await tester.pumpAndSettle();

    expect(find.text('No search profiles yet'), findsOneWidget);
  });

  testWidgets('a matched job shows its title, company and an Apply button', (tester) async {
    final api = FakeApi(
      profileList: [
        const Profile(id: 1, name: 'Backend Go', keywords: ['go'], locations: [], remoteOnly: true),
      ],
      jobList: [
        Job(
          id: 10,
          provider: 'greenhouse',
          company: 'Stripe',
          title: 'Backend Engineer, Payments',
          location: 'Remote',
          remote: true,
          url: 'https://example.com/apply',
          matchedAt: DateTime.now(),
        ),
      ],
    );

    await tester.pumpWidget(app(api));
    await tester.pumpAndSettle();

    expect(find.text('Backend Engineer, Payments'), findsOneWidget);
    expect(find.textContaining('Stripe'), findsOneWidget);
    expect(find.text('Apply'), findsOneWidget);
  });

  testWidgets('the unread badge appears on the Notifications tab', (tester) async {
    final api = FakeApi(
      profileList: [
        const Profile(id: 1, name: 'Backend Go', keywords: [], locations: [], remoteOnly: false),
      ],
      jobList: [
        Job(
          id: 10,
          provider: 'lever',
          company: 'Spotify',
          title: 'Data Engineer',
          location: 'Stockholm',
          remote: false,
          url: 'https://example.com/apply',
          matchedAt: DateTime.now(),
        ),
      ],
    );

    await tester.pumpWidget(app(api));
    await tester.pumpAndSettle();

    expect(find.text('1'), findsOneWidget); // the badge count
  });
}
