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
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:ofertas_relampago_app/order_access.dart';

void seedAccess(List<String> ids) {
  FlutterSecureStorage.setMockInitialValues({
    for (final id in ids)
      SecureOrderAccessStore.key('http://127.0.0.1:8080', id): OrderAccess(
        'a' * 64,
        DateTime.now().toUtc().add(const Duration(minutes: 20)),
      ).encode(),
  });
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('creation deadline has unknown outcome and never repeats POST', (
    tester,
  ) async {
    var requests = 0;
    final pending = Completer<http.Response>();
    final api = OffersApi(
      baseUrl: 'http://127.0.0.1:8080',
      client: MockClient((r) {
        requests++;
        return pending.future;
      }),
    );
    final assertion = expectLater(
      api.createOrder('demo-offer'),
      throwsA(isA<TimeoutException>()),
    );
    await tester.pump(const Duration(seconds: 8));
    await assertion;
    expect(requests, 1);
    pending.complete(http.Response('{}', 201));
    await tester.pump();
  });

  test('local Woovi SDK package exports the merchant checkout contract', () {
    expect(CheckoutStatus.values, contains(CheckoutStatus.pending));
    expect(HttpCheckoutTransport, isA<Type>());
    expect(PixCheckoutView, isA<Type>());
  });

  testWidgets('legacy history IDs cannot consult without secure capability', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({
      'offers.order_history': ['legacy-order'],
    });
    seedAccess([]);
    var orderRequests = 0;
    final api = OffersApi(
      baseUrl: 'http://127.0.0.1:8080',
      client: MockClient((request) async {
        if (request.url.path != '/v1/offers') orderRequests++;
        return http.Response('[]', 200);
      }),
    );
    await tester.pumpWidget(FlashOffersApp(api: api));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Meus pedidos'));
    await tester.pumpAndSettle();
    expect(orderRequests, 0);
    expect(
      find.textContaining('O histórico e o ID não autorizam consulta'),
      findsOneWidget,
    );
    expect(find.text('Pedido legacy-order'), findsNothing);
  });

  testWidgets(
    '401 removes stale confirmation and stops polling without public fallback',
    (tester) async {
      SharedPreferences.setMockInitialValues({});
      seedAccess([]);
      var queries = 0;
      final api = OffersApi(
        baseUrl: 'http://127.0.0.1:8080',
        client: MockClient((request) async {
          if (request.url.path == '/v1/offers') {
            return http.Response(
              jsonEncode([
                {
                  'id': 'demo-offer',
                  'title': 'Sacola teste',
                  'price_cents': 2500,
                  'available_units': 1,
                  'reservation_ttl_seconds': 120,
                },
              ]),
              200,
            );
          }
          if (request.method == 'POST') {
            return http.Response(
              jsonEncode({
                'id': 'order-revoked',
                'amount_cents': 2500,
                'state': 'pending_payment',
                'order_access_token': 'a' * 64,
                'order_token_expires_at': DateTime.now()
                    .toUtc()
                    .add(const Duration(minutes: 20))
                    .toIso8601String(),
              }),
              201,
            );
          }
          expect(request.url.path, '/v1/customer/orders/order-revoked');
          queries++;
          return http.Response(
            '{"message":"Expirou","error_code":"ORDER_UNAUTHORIZED"}',
            401,
          );
        }),
      );
      await tester.pumpWidget(FlashOffersApp(api: api));
      await tester.pumpAndSettle();
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.tap(find.text('Reservar'));
      await tester.pumpAndSettle();
      expect(find.text('Sacola reservada'), findsOneWidget);
      await tester.pump(const Duration(seconds: 5));
      await tester.pumpAndSettle();
      expect(queries, 1);
      expect(find.text('Sacola reservada'), findsNothing);
      expect(find.textContaining('expirou ou foi revogada'), findsOneWidget);
      await tester.pump(const Duration(minutes: 1));
      expect(queries, 1);
    },
  );

  testWidgets('restores saved order IDs and fetches their current state', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({
      'offers.order_history': ['saved-order-1'],
    });
    var statusRequests = 0;
    seedAccess(['saved-order-1']);
    var orderState = 'paid';
    final client = MockClient((request) async {
      if (request.url.path == '/v1/offers') {
        return http.Response('[]', 200);
      }
      if (request.url.path == '/v1/customer/orders/saved-order-1') {
        expect(request.headers['authorization'], 'Bearer ${'a' * 64}');
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
        api: OffersApi(baseUrl: 'http://127.0.0.1:8080', client: client),
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
      seedAccess(['offline-order']);
      final client = MockClient((request) async {
        if (request.url.path == '/v1/offers') return http.Response('[]', 200);
        if (request.url.path == '/v1/customer/orders/offline-order') {
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
          api: OffersApi(baseUrl: 'http://127.0.0.1:8080', client: client),
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

  testWidgets('opening history waits for the initial history request', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({
      'offers.order_history': ['slow-order'],
    });
    seedAccess(['slow-order']);
    final pendingResponse = Completer<http.Response>();
    final client = MockClient((request) async {
      if (request.url.path == '/v1/offers') return http.Response('[]', 200);
      return pendingResponse.future;
    });
    await tester.pumpWidget(
      FlashOffersApp(
        api: OffersApi(baseUrl: 'http://127.0.0.1:8080', client: client),
      ),
    );
    await tester.pump();
    await tester.tap(find.byTooltip('Meus pedidos'));
    await tester.pump();
    expect(find.text('Meus pedidos'), findsNothing);

    pendingResponse.complete(
      http.Response(
        jsonEncode({'id': 'slow-order', 'amount_cents': 2500, 'state': 'paid'}),
        200,
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Pedido slow-order'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('history refresh is single-flight and safe after closing modal', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({
      'offers.order_history': ['slow-order'],
    });
    var statusRequests = 0;
    seedAccess(['slow-order']);
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
        api: OffersApi(baseUrl: 'http://127.0.0.1:8080', client: client),
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
    seedAccess([]);
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
            'order_access_token': 'a' * 64,
            'order_token_expires_at': DateTime.now()
                .toUtc()
                .add(const Duration(minutes: 20))
                .toIso8601String(),
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
          request.url.path == '/v1/customer/orders/order-demo-123') {
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
    final api = OffersApi(baseUrl: 'http://127.0.0.1:8080', client: client);

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
