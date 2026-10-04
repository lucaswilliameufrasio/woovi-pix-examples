import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:ofertas_relampago_app/main.dart';
import 'package:ofertas_relampago_app/offers_api.dart';
import 'package:woovi_pix_flutter/woovi_pix_flutter.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('local Woovi SDK package exports the merchant checkout contract', () {
    expect(CheckoutStatus.values, contains(CheckoutStatus.pending));
    expect(HttpCheckoutTransport, isA<Type>());
    expect(PixCheckoutView, isA<Type>());
  });

  testWidgets('restores saved order IDs and fetches their current state', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({
      'offers.order_history': ['saved-order-1'],
    });
    var statusRequests = 0;
    var orderState = 'paid';
    final client = MockClient((request) async {
      if (request.url.path == '/v1/offers') {
        return http.Response('[]', 200);
      }
      if (request.url.path == '/v1/orders/saved-order-1') {
        statusRequests++;
        return http.Response(
          jsonEncode({
            'id': 'saved-order-1',
            'offer_id': 'demo-offer',
            'amount_cents': 3100,
            'state': orderState,
            'expires_at': '2026-10-04T20:00:00Z',
          }),
          200,
        );
      }
      return http.Response('{}', 404);
    });

    await tester.pumpWidget(
      FlashOffersApp(
        api: OffersApi(baseUrl: 'http://local.test', client: client),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Meus pedidos'));
    await tester.pumpAndSettle();

    expect(statusRequests, 2);
    expect(find.text('Pedido saved-order-1'), findsOneWidget);
    expect(find.text('Pago'), findsOneWidget);

    orderState = 'expired';
    await tester.tap(find.byTooltip('Atualizar pedidos'));
    await tester.pumpAndSettle();
    expect(statusRequests, 3);
    expect(find.text('Reserva expirada'), findsOneWidget);
    expect(find.text('Pago'), findsNothing);
  });

  testWidgets(
    'shows a retry direction instead of stale history on API failure',
    (tester) async {
      SharedPreferences.setMockInitialValues({
        'offers.order_history': ['offline-order'],
      });
      var online = false;
      final client = MockClient((request) async {
        if (request.url.path == '/v1/offers') return http.Response('[]', 200);
        if (request.url.path == '/v1/orders/offline-order') {
          if (!online) throw http.ClientException('offline');
          return http.Response(
            jsonEncode({
              'id': 'offline-order',
              'amount_cents': 2500,
              'state': 'pending_payment',
            }),
            200,
          );
        }
        return http.Response('{}', 404);
      });

      await tester.pumpWidget(
        FlashOffersApp(
          api: OffersApi(baseUrl: 'http://local.test', client: client),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('Meus pedidos'));
      await tester.pumpAndSettle();

      expect(
        find.text(
          'Não foi possível consultar seus pedidos agora. Tente atualizar.',
        ),
        findsOneWidget,
      );
      expect(find.text('Pedido offline-order'), findsNothing);
      final preferences = await SharedPreferences.getInstance();
      expect(preferences.getStringList('offers.order_history'), [
        'offline-order',
      ]);

      online = true;
      await tester.tap(find.byTooltip('Atualizar pedidos'));
      await tester.pumpAndSettle();
      expect(find.text('Pedido offline-order'), findsOneWidget);
      expect(find.text('Aguardando pagamento'), findsOneWidget);
      expect(find.textContaining('Não foi possível consultar'), findsNothing);
    },
  );

  testWidgets('history refresh is single-flight and safe after closing modal', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({
      'offers.order_history': ['slow-order'],
    });
    var statusRequests = 0;
    final pendingResponse = Completer<http.Response>();
    http.Response orderResponse() => http.Response(
      jsonEncode({
        'id': 'slow-order',
        'amount_cents': 2500,
        'state': 'expired',
      }),
      200,
    );
    final client = MockClient((request) async {
      if (request.url.path == '/v1/offers') return http.Response('[]', 200);
      statusRequests++;
      if (statusRequests > 2) return pendingResponse.future;
      return orderResponse();
    });
    await tester.pumpWidget(
      FlashOffersApp(
        api: OffersApi(baseUrl: 'http://local.test', client: client),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Meus pedidos'));
    await tester.pumpAndSettle();

    final refresh = find.byTooltip('Atualizar pedidos');
    await tester.tap(refresh);
    await tester.pump();
    expect(statusRequests, 3);
    final refreshButton = find.byWidgetPredicate(
      (widget) => widget is IconButton && widget.tooltip == 'Atualizar pedidos',
    );
    expect(tester.widget<IconButton>(refreshButton).onPressed, isNull);
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.tap(refresh);
    await tester.pump();
    expect(statusRequests, 3);

    Navigator.of(tester.element(refresh)).pop();
    await tester.pumpAndSettle();
    pendingResponse.complete(orderResponse());
    await tester.pumpAndSettle();
    expect(find.text('Meus pedidos'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('loads the local offer and creates an order through the API', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    var offerRequests = 0;
    var orderStatusRequests = 0;
    var orderCreated = false;
    var backendOrderState = 'pending_payment';
    final client = MockClient((request) async {
      if (request.method == 'GET' && request.url.path == '/v1/offers') {
        offerRequests++;
        return http.Response(
          jsonEncode([
            {
              'id': 'demo-offer',
              'title': 'Sacola surpresa da feira',
              'price_cents': 2500,
              'available_units': orderCreated ? 0 : 1,
              'reservation_ttl_seconds': 120,
            },
          ]),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      if (request.method == 'POST' && request.url.path == '/v1/orders') {
        expect(jsonDecode(request.body), {'offer_id': 'demo-offer'});
        orderCreated = true;
        return http.Response(
          jsonEncode({
            'id': 'order-demo-123',
            'offer_id': 'demo-offer',
            'amount_cents': 2500,
            'state': 'pending_payment',
            'expires_at': '2026-10-04T20:00:00Z',
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }
      if (request.method == 'GET' &&
          request.url.path == '/v1/orders/order-demo-123') {
        orderStatusRequests++;
        return http.Response(
          jsonEncode({
            'id': 'order-demo-123',
            'offer_id': 'demo-offer',
            'amount_cents': 2500,
            'state': backendOrderState,
            'expires_at': '2026-10-04T20:00:00Z',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response('{}', 404);
    });
    final api = OffersApi(baseUrl: 'http://local.test', client: client);

    await tester.pumpWidget(FlashOffersApp(api: api));
    await tester.pumpAndSettle();

    expect(find.text('Sacola surpresa da feira'), findsOneWidget);
    expect(find.text('R\$ 25,00'), findsOneWidget);
    expect(offerRequests, 1);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);

    await tester.tap(find.text('Reservar'));
    await tester.pumpAndSettle();

    expect(orderCreated, isTrue);
    expect(find.text('Sacola reservada'), findsOneWidget);
    expect(find.textContaining('order-demo-123'), findsOneWidget);
    expect(
      find.textContaining('Pagamento ainda não integrado'),
      findsOneWidget,
    );
    expect(offerRequests, 2);
    final preferences = await SharedPreferences.getInstance();
    expect(preferences.getStringList('offers.order_history'), [
      'order-demo-123',
    ]);

    backendOrderState = 'expired';
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump(const Duration(seconds: 10));
    expect(orderStatusRequests, 0);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpAndSettle();

    expect(orderStatusRequests, 1);
    expect(find.text('Estado: Reserva expirada'), findsOneWidget);
    expect(find.text('Pagar'), findsNothing);

    await tester.tap(find.byTooltip('Meus pedidos'));
    await tester.pumpAndSettle();
    expect(find.text('Meus pedidos'), findsOneWidget);
    expect(find.textContaining('order-demo-123'), findsNWidgets(2));
    expect(find.text('Reserva expirada'), findsOneWidget);
  });
}
