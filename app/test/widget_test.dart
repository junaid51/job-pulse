import 'package:flutter_test/flutter_test.dart';
import 'package:jobpulse/main.dart';

// A smoke test: it fails if the app cannot be built at all, which is the only
// thing worth asserting until there are screens.
void main() {
  testWidgets('app boots', (tester) async {
    await tester.pumpWidget(const JobPulseApp());
    expect(find.text('JobPulse'), findsOneWidget);
  });
}
