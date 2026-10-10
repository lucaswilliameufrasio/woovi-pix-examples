import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:reservas_app/main.dart';
import 'package:reservas_app/reservations_api.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('Should reserve and privately track a local appointment', (
    tester,
  ) async {
    await tester.pumpWidget(ReservationsApp(api: ReservationsApi()));
    await _pumpUntil(tester, find.textContaining('09:'));

    final slot = find.textContaining('09:').first;
    await tester.drag(find.byType(Scrollable).first, const Offset(0, -350));
    await tester.pumpAndSettle();
    await tester.ensureVisible(slot);
    await tester.pumpAndSettle();
    await tester.tap(slot);
    await _pumpUntil(tester, find.text('SUA RESERVA'));
    expect(find.text('Reserva: held'), findsOneWidget);
    expect(find.text('Pagamento: pending'), findsOneWidget);

    final cancel = find.text('Cancelar retenção');
    await tester.ensureVisible(cancel);
    await tester.pumpAndSettle();
    await tester.tap(cancel);
    await _pumpUntil(tester, find.text('Reserva: cancelled'));
    expect(find.text('Pagamento: cancelled'), findsOneWidget);
  });
}

Future<void> _pumpUntil(WidgetTester tester, Finder finder) async {
  for (var attempt = 0; attempt < 60; attempt += 1) {
    await tester.pump(const Duration(milliseconds: 200));
    if (finder.evaluate().isNotEmpty) {
      return;
    }
  }
  throw TestFailure('A interface não atingiu o estado esperado: $finder');
}
