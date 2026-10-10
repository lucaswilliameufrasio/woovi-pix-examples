import 'package:click_collect_app/click_collect_api.dart';
import 'package:click_collect_app/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('Should create and privately track a local customer order', (
    tester,
  ) async {
    final service = ClickCollectApi.fromEnvironment();
    await tester.pumpWidget(BalcaoApp(service: service));
    await _scrollUntilVisible(tester, find.text('Reservar uma fatia'));

    final reserve = find.text('Reservar uma fatia');
    await tester.ensureVisible(reserve);
    await tester.tap(reserve);
    await _pumpUntil(tester, find.byKey(const Key('reservation-saved')));
    await tester.ensureVisible(find.text('Seu pedido'));
    expect(find.text('Aguardando confirmação'), findsNWidgets(2));

    final simulate = find.text('Simular confirmação local');
    await tester.ensureVisible(simulate);
    await tester.pumpAndSettle();
    await tester.tap(simulate);
    await _waitForPreparation(tester);
  });
}

Future<void> _scrollUntilVisible(WidgetTester tester, Finder finder) async {
  final list = find.byType(ListView).first;
  for (var attempt = 0; attempt < 8; attempt += 1) {
    await tester.pump(const Duration(milliseconds: 200));
    if (finder.evaluate().isNotEmpty) {
      await tester.ensureVisible(finder);
      return;
    }
    await tester.drag(list, const Offset(0, -500));
  }
  await _pumpUntil(tester, finder);
}

Future<void> _waitForPreparation(WidgetTester tester) async {
  final confirmed = find.text('Confirmado');
  final preparing = find.text('A cozinha está preparando');
  final refresh = find.text('Atualizar status');
  for (var attempt = 0; attempt < 60; attempt += 1) {
    if (confirmed.evaluate().isNotEmpty && preparing.evaluate().isNotEmpty) {
      return;
    }
    if (refresh.evaluate().isNotEmpty) {
      await tester.ensureVisible(refresh);
      await tester.pumpAndSettle();
      await tester.tap(refresh);
    }
    await tester.pump(const Duration(milliseconds: 200));
  }
  throw TestFailure('A confirmação local não chegou ao estado de preparo.');
}

Future<void> _pumpUntil(WidgetTester tester, Finder finder) async {
  for (var attempt = 0; attempt < 60; attempt += 1) {
    await tester.pump(const Duration(milliseconds: 200));
    if (finder.evaluate().isNotEmpty) {
      return;
    }
  }
  final visibleText = tester
      .widgetList<Text>(find.byType(Text))
      .map((widget) => widget.data ?? widget.textSpan?.toPlainText() ?? '')
      .where((text) => text.isNotEmpty)
      .join(' | ');
  throw TestFailure(
    'A interface não atingiu o estado esperado: $finder. Tela: $visibleText',
  );
}
