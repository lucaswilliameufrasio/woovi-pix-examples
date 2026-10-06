import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:integration_test/integration_test.dart';
import 'package:ofertas_relampago_app/main.dart';
import 'package:ofertas_relampago_app/offers_api.dart';
import 'package:ofertas_relampago_app/order_access.dart';
import 'package:shared_preferences/shared_preferences.dart';

Uri local(String value) {
  final uri = Uri.parse(value);
  if (uri.scheme != 'http' ||
      uri.host != '127.0.0.1' ||
      !uri.hasPort ||
      uri.userInfo.isNotEmpty ||
      uri.hasQuery ||
      uri.hasFragment ||
      (uri.path.isNotEmpty && uri.path != '/')) {
    throw ArgumentError('Execute pela fixture Android isolada em loopback.');
  }
  return uri;
}

Future<void> simulatedPayment(Uri simulator, String orderId) async {
  final client = http.Client();
  try {
    for (final path in ['/v1/charges', '/v1/charges/$orderId/pay']) {
      final request = http.Request('POST', simulator.resolve(path))
        ..followRedirects = false;
      if (path == '/v1/charges') {
        request.headers['content-type'] = 'application/json';
        request.body = jsonEncode({'order_id': orderId});
      }
      final response = await client
          .send(request)
          .timeout(const Duration(seconds: 8));
      expect(response.statusCode, 200);
      await response.stream.drain<void>().timeout(const Duration(seconds: 8));
    }
  } finally {
    client.close();
  }
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  const phase = String.fromEnvironment('NATIVE_SMOKE_PHASE');
  testWidgets('native reservation and secure persistence: $phase', (
    tester,
  ) async {
    final base = local(const String.fromEnvironment('NATIVE_SMOKE_API_BASE'));
    final simulator = local(
      const String.fromEnvironment('NATIVE_SMOKE_SIM_BASE'),
    );
    if (!['create', 'restore'].contains(phase)) {
      throw ArgumentError(
        'Fase nativa inválida. Use tooling/run_offers_smoke.py --android-device.',
      );
    }
    final api = OffersApi(baseUrl: base.origin);
    addTearDown(api.close);
    final preferences = await SharedPreferences.getInstance();
    final fixtureKey = 'offers.native_smoke.${base.origin}';

    await tester.pumpWidget(FlashOffersApp(api: api));
    await tester.pumpAndSettle(
      const Duration(milliseconds: 100),
      EnginePhase.sendSemanticsUpdate,
      const Duration(seconds: 30),
    );
    String orderId;
    if (phase == 'create') {
      final reserve = find.text('Reservar');
      await tester.ensureVisible(reserve);
      await tester.tap(reserve);
      await tester.pumpAndSettle(
        const Duration(milliseconds: 100),
        EnginePhase.sendSemanticsUpdate,
        const Duration(seconds: 30),
      );
      expect(find.text('Sacola reservada'), findsOneWidget);
      await preferences.reload();
      final ids = preferences.getStringList('offers.order_history');
      if (ids == null ||
          ids.isEmpty ||
          !RegExp(r'^[a-f0-9]{32}$').hasMatch(ids.first)) {
        throw StateError(
          'Reserva não forneceu referência válida no histórico nativo.',
        );
      }
      orderId = ids.first;
      expect(api.accessWasPersisted(orderId), isTrue);
      final grant = await const SecureOrderAccessStore().read(
        base.origin,
        orderId,
      );
      expect(grant != null, isTrue); // Never serialize/print the capability.
      if (grant == null) {
        throw StateError('Credencial nativa indisponível.');
      }
      for (final key in preferences.getKeys()) {
        expect('${preferences.get(key)}'.contains(grant.token), isFalse);
      }
      expect(await preferences.setString(fixtureKey, orderId), isTrue);
      await simulatedPayment(simulator, orderId);
      final deadline = DateTime.now().add(const Duration(seconds: 15));
      while ((await api.getOrder(orderId)).state != 'paid') {
        if (DateTime.now().isAfter(deadline)) {
          throw StateError('Worker não confirmou pagamento local no prazo.');
        }
        await tester.pump(const Duration(milliseconds: 250));
      }
      await tester.tap(find.text('Atualizar estado'));
      await tester.pumpAndSettle();
      expect(find.text('Estado: Pago'), findsOneWidget);
    } else {
      final saved = preferences.getString(fixtureKey);
      if (saved == null || !RegExp(r'^[a-f0-9]{32}$').hasMatch(saved)) {
        throw StateError(
          'Execute create antes de restore, sem apagar dados do app.',
        );
      }
      orderId = saved;
      // This process has no memory grant and no injected mock storage.
      expect((await api.getOrder(orderId)).state, 'paid');
      await tester.tap(find.byTooltip('Meus pedidos'));
      await tester.pumpAndSettle();
      expect(find.text('Pedido $orderId'), findsOneWidget);
      expect(find.text('Pago'), findsOneWidget);
    }
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
  });
}
