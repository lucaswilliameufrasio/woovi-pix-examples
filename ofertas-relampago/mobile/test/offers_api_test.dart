import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:ofertas_relampago_app/offers_api.dart';
import 'package:ofertas_relampago_app/order_access.dart';

class MemoryAccessStore implements OrderAccessStore {
  final values = <String, OrderAccess>{};
  bool failWrite = false;
  bool failRead = false;

  @override
  Future<OrderAccess?> read(String origin, String id) async {
    if (failRead) throw const FileSystemException('unavailable');
    return values['$origin/$id'];
  }

  @override
  Future<void> write(String origin, String id, OrderAccess access) async {
    if (failWrite) throw const FileSystemException('unavailable');
    values['$origin/$id'] = access;
  }
}

final now = DateTime.utc(2026, 10, 5);
final token = 'a' * 64;
Map<String, Object> orderBody({String id = 'order-a', bool access = false}) => {
  'id': id,
  'amount_cents': 2500,
  'state': 'pending_payment',
  if (access) 'order_access_token': token,
  if (access)
    'order_token_expires_at': now
        .add(const Duration(minutes: 20))
        .toIso8601String(),
};

void main() {
  test(
    'unknown states, wrong order IDs and malformed numeric fields are rejected',
    () async {
      final store = MemoryAccessStore();
      await store.write(
        'http://127.0.0.1:8080',
        'order-a',
        OrderAccess(token, now.add(const Duration(minutes: 20))),
      );
      for (final body in [
        {...orderBody(), 'id': 'another-order'},
        {...orderBody(), 'state': 'fulfilled'},
        {...orderBody(), 'amount_cents': -1},
        {...orderBody(), 'amount_cents': '2500'},
      ]) {
        final api = OffersApi(
          baseUrl: 'http://127.0.0.1:8080',
          client: MockClient((r) async => http.Response(jsonEncode(body), 200)),
          accessStore: store,
          now: () => now,
        );
        await expectLater(api.getOrder('order-a'), throwsFormatException);
      }
    },
  );
  test('creation stores an origin-scoped capability and restart uses only private route', () async {
    final store = MemoryAccessStore();
    final client = MockClient((request) async {
      expect(request.followRedirects, isFalse);
      if (request.method == 'POST') {
        expect(jsonDecode(request.body), {'offer_id': 'demo-offer'});
        return http.Response(jsonEncode(orderBody(access: true)), 201);
      }
      expect(request.url.path, '/v1/customer/orders/order-a');
      expect(request.headers['authorization'], 'Bearer $token');
      return http.Response(jsonEncode(orderBody()), 200);
    });
    final first = OffersApi(
      baseUrl: 'http://127.0.0.1:8080',
      client: client,
      accessStore: store,
      now: () => now,
    );
    expect((await first.createOrder('demo-offer')).amountCents, 2500);
    expect(first.accessWasPersisted('order-a'), isTrue);
    final restarted = OffersApi(
      baseUrl: 'http://127.0.0.1:8080',
      client: client,
      accessStore: store,
      now: () => now,
    );
    expect((await restarted.getOrder('order-a')).id, 'order-a');
    final otherOrigin = OffersApi(
      baseUrl: 'http://127.0.0.1:8081',
      client: client,
      accessStore: store,
      now: () => now,
    );
    await expectLater(
      otherOrigin.getOrder('order-a'),
      throwsA(
        isA<OffersApiException>().having(
          (e) => e.errorCode,
          'code',
          'ORDER_UNAUTHORIZED',
        ),
      ),
    );
  });

  test('failed secure write does not repeat reservation or invalidate in-memory access', () async {
    final store = MemoryAccessStore()..failWrite = true;
    var posts = 0;
    final client = MockClient((request) async {
      if (request.method == 'POST') {
        posts++;
        return http.Response(jsonEncode(orderBody(access: true)), 201);
      }
      return http.Response(jsonEncode(orderBody()), 200);
    });
    final api = OffersApi(
      baseUrl: 'http://127.0.0.1:8080',
      client: client,
      accessStore: store,
      now: () => now,
    );
    await api.createOrder('demo-offer');
    expect(api.accessWasPersisted('order-a'), isFalse);
    await api.getOrder('order-a');
    final restarted = OffersApi(
      baseUrl: 'http://127.0.0.1:8080',
      client: client,
      accessStore: store,
      now: () => now,
    );
    await expectLater(
      restarted.getOrder('order-a'),
      throwsA(isA<OffersApiException>()),
    );
    expect(posts, 1);
  });

  test(
    'missing, expired and unsafe IDs never trigger network or public fallback',
    () async {
      var requests = 0;
      final store = MemoryAccessStore();
      await store.write(
        'http://127.0.0.1:8080',
        'expired',
        OrderAccess(token, now),
      );
      final api = OffersApi(
        baseUrl: 'http://127.0.0.1:8080',
        client: MockClient((r) async {
          requests++;
          return http.Response('{}', 200);
        }),
        accessStore: store,
        now: () => now,
      );
      for (final id in ['unknown', 'expired', '../other', 'order?a=1']) {
        await expectLater(
          api.getOrder(id),
          throwsA(
            isA<OffersApiException>().having(
              (e) => e.errorCode,
              'code',
              'ORDER_UNAUTHORIZED',
            ),
          ),
        );
      }
      expect(requests, 0);
    },
  );

  test('401 revokes local capability and does not keep requesting or resurrect it from storage', () async {
    var requests = 0;
    final store = MemoryAccessStore();
    await store.write(
      'http://127.0.0.1:8080',
      'order-a',
      OrderAccess(token, now.add(const Duration(minutes: 20))),
    );
    final api = OffersApi(
      baseUrl: 'http://127.0.0.1:8080',
      client: MockClient((r) async {
        requests++;
        return http.Response(
          jsonEncode({
            'message': 'Acesso expirado',
            'error_code': 'ORDER_UNAUTHORIZED',
          }),
          401,
        );
      }),
      accessStore: store,
      now: () => now,
    );
    for (var i = 0; i < 2; i++) {
      await expectLater(
        api.getOrder('order-a'),
        throwsA(isA<OffersApiException>()),
      );
    }
    expect(requests, 1);
  });

  test(
    'storage read failure is recoverable but never calls unauthenticated route',
    () async {
      final store = MemoryAccessStore()..failRead = true;
      await store.write(
        'http://127.0.0.1:8080',
        'order-a',
        OrderAccess(token, now.add(const Duration(minutes: 20))),
      );
      var requests = 0;
      final api = OffersApi(
        baseUrl: 'http://127.0.0.1:8080',
        client: MockClient((r) async {
          requests++;
          return http.Response(jsonEncode(orderBody()), 200);
        }),
        accessStore: store,
        now: () => now,
      );
      await expectLater(
        api.getOrder('order-a'),
        throwsA(
          isA<OffersApiException>().having(
            (e) => e.errorCode,
            'code',
            'ORDER_ACCESS_UNAVAILABLE',
          ),
        ),
      );
      expect(requests, 0);
      store.failRead = false;
      await api.getOrder('order-a');
      expect(requests, 1);
    },
  );

  test(
    'invalid creation capability and network errors never retry POST',
    () async {
      for (final body in [
        'not json',
        jsonEncode(orderBody()),
        jsonEncode({
          ...orderBody(access: true),
          'order_access_token': 'invalid',
        }),
        jsonEncode({...orderBody(access: true), 'state': 'unknown'}),
      ]) {
        var requests = 0;
        final api = OffersApi(
          baseUrl: 'http://127.0.0.1:8080',
          client: MockClient((r) async {
            requests++;
            return http.Response(body, 201);
          }),
          accessStore: MemoryAccessStore(),
          now: () => now,
        );
        await expectLater(api.createOrder('demo-offer'), throwsFormatException);
        expect(requests, 1);
      }
    },
  );

  test('refuses remote and credential-bearing configured origins', () {
    for (final url in [
      'https://example.com',
      'http://localhost:8080',
      'http://127.0.0.1',
      'http://u:p@127.0.0.1:8080',
      'http://127.0.0.1:8080/path',
      'http://127.0.0.1:8080?token=secret',
    ]) {
      expect(() => OffersApi(baseUrl: url), throwsArgumentError);
    }
  });

  test(
    'real HTTP transport refuses credential redirects without following them',
    () async {
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      addTearDown(() => server.close(force: true));
      var calls = 0;
      server.listen((request) async {
        calls++;
        request.response.statusCode = 302;
        request.response.headers.set('location', '/other');
        await request.response.close();
      });
      final origin = 'http://127.0.0.1:${server.port}';
      final store = MemoryAccessStore();
      await store.write(
        origin,
        'order-a',
        OrderAccess(token, now.add(const Duration(minutes: 20))),
      );
      final api = OffersApi(
        baseUrl: origin,
        accessStore: store,
        now: () => now,
      );
      addTearDown(api.close);
      await expectLater(
        api.getOrder('order-a'),
        throwsA(
          isA<OffersApiException>().having(
            (e) => e.errorCode,
            'code',
            'DEPENDENCY_INVALID_RESPONSE',
          ),
        ),
      );
      expect(calls, 1);
    },
  );
}
