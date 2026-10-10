import 'dart:convert';

import 'package:click_collect_app/click_collect_api.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class MemoryCredentialStore implements CredentialStore {
  final values = <String, String>{};

  @override
  Future<Map<String, String>> readAll(String prefix) async => Map.fromEntries(
    values.entries.where((entry) => entry.key.startsWith(prefix)),
  );

  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}

Map<String, Object?> reservedOrderJson() => {
  'id': '0123456789abcdef0123456789abcdef',
  'product_id': 'house-cake',
  'product_title': 'Bolo de fubá da casa',
  'amount_cents': 890,
  'payment_state': 'pending',
  'fulfillment_state': 'awaiting_payment',
  'expires_at': '2026-10-09T15:00:00Z',
  'created_at': '2026-10-09T14:57:00Z',
  'order_access_token': List<String>.filled(64, 'a').join(),
  'pickup_code': 'A1B2C3D4',
};

void main() {
  group('ClickCollectApi', () {
    test('Should reject a non-loopback API origin', () {
      expect(
        () => ClickCollectApi(baseUrl: 'https://api.example.com'),
        throwsArgumentError,
      );
    });

    test(
      'Should persist private order access before returning a reservation',
      () async {
        final credentials = MemoryCredentialStore();
        final api = ClickCollectApi(
          baseUrl: 'http://127.0.0.1:8082',
          credentials: credentials,
          client: MockClient((request) async {
            expect(request.method, 'POST');
            expect(request.url.path, '/v1/orders');
            expect(jsonDecode(request.body), {'product_id': 'house-cake'});
            return http.Response(
              jsonEncode(reservedOrderJson()),
              201,
              headers: {'content-type': 'application/json'},
            );
          }),
        );
        addTearDown(api.close);

        final reserved = await api.reserve('house-cake');

        expect(reserved.order.paymentState, PaymentState.pending);
        expect(
          reserved.order.fulfillmentState,
          FulfillmentState.awaitingPayment,
        );
        expect(reserved.pickupCode, 'A1B2C3D4');
        expect(reserved.accessToken, List<String>.filled(64, 'a').join());
        expect(credentials.values, hasLength(1));
        expect(credentials.values.values.single, contains('access_token'));
      },
    );

    test(
      'Should reject unknown independent payment and fulfillment states',
      () {
        final payload = {
          ...reservedOrderJson(),
          'payment_state': 'ready_for_pickup',
        };
        payload.remove('order_access_token');
        payload.remove('pickup_code');

        expect(() => DemoOrder.fromJson(payload), throwsFormatException);
      },
    );

    test(
      'Should report secure-storage failure without retrying reservation',
      () async {
        final api = ClickCollectApi(
          baseUrl: 'http://127.0.0.1:8082',
          credentials: _FailingCredentialStore(),
          client: MockClient((request) async {
            return http.Response(
              jsonEncode(reservedOrderJson()),
              201,
              headers: {'content-type': 'application/json'},
            );
          }),
        );
        addTearDown(api.close);

        await expectLater(
          api.reserve('house-cake'),
          throwsA(
            isA<ClickCollectApiException>().having(
              (error) => error.errorCode,
              'errorCode',
              'CREDENTIAL_STORAGE_FAILED',
            ),
          ),
        );
      },
    );
  });
}

class _FailingCredentialStore implements CredentialStore {
  @override
  Future<Map<String, String>> readAll(String prefix) async => {};

  @override
  Future<void> write(String key, String value) async {
    throw StateError('secure store unavailable');
  }
}
