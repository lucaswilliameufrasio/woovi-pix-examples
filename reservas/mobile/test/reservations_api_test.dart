import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:reservas_app/reservations_api.dart';

void main() {
  group('Availability', () {
    test(
      'Should parse UTC slots while retaining local labels and timezone',
      () {
        final availability = Availability.fromJson({
          'resource_name': 'Sala de atendimento',
          'time_zone': 'America/Sao_Paulo',
          'price_cents': 12000,
          'slots': [
            {
              'starts_at': '2026-10-12T12:00:00Z',
              'ends_at': '2026-10-12T12:30:00Z',
              'local_label': 'Mon 12 Oct 09:00 -03:00',
              'time_zone': 'America/Sao_Paulo',
            },
          ],
        });

        expect(availability.priceCents, 12000);
        expect(availability.slots.single.startsAt, '2026-10-12T12:00:00Z');
        expect(availability.slots.single.localLabel, contains('-03:00'));
      },
    );

    test('Should reject malformed availability values', () {
      expect(() => Availability.fromJson({'slots': []}), throwsFormatException);
    });
  });

  test('Should reject non-loopback API origins', () {
    expect(
      () => ReservationsApi(baseUrl: 'http://example.com:8084'),
      throwsArgumentError,
    );
  });

  test('Should report uncertain hold timeout without retrying', () async {
    var requests = 0;
    final client = MockClient((request) async {
      requests += 1;
      await Future<void>.delayed(const Duration(milliseconds: 50));
      return http.Response('{}', 201);
    });
    final api = ReservationsApi(
      client: client,
      requestTimeout: const Duration(milliseconds: 5),
    );
    addTearDown(api.close);

    await expectLater(
      api.createHold('2026-10-12T12:00:00Z'),
      throwsA(isA<UnknownReservationOutcome>()),
    );
    expect(requests, 1);
  });
}
