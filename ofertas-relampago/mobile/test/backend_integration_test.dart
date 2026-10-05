import 'dart:io';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ofertas_relampago_app/offers_api.dart';

void main() {
  final base = Platform.environment['MOBILE_SMOKE_API_BASE'];
  test(
    'real backend reserves once and private capability survives client restart',
    () async {
      if (base == null) {
        fail('Execute usando tooling/run_offers_smoke.py --mobile.');
      }
      // Only the platform keystore is replaced. HTTP/PostgreSQL are real.
      FlutterSecureStorage.setMockInitialValues({});
      final api = OffersApi(baseUrl: base);
      addTearDown(api.close);
      final offers = await api.listOffers();
      expect(offers.single.availableUnits, 1);
      final order = await api.createOrder(offers.single.id);
      expect(order.state, 'pending_payment');
      expect(api.accessWasPersisted(order.id), isTrue);
      final restarted = OffersApi(baseUrl: base);
      addTearDown(restarted.close);
      expect(
        (await restarted.getOrder(order.id)).amountCents,
        offers.single.priceCents,
      );
      expect((await restarted.listOffers()).single.availableUnits, 0);
      await expectLater(
        restarted.getOrder('unknown-id'),
        throwsA(isA<OffersApiException>()),
      );
    },
    skip: base == null
        ? 'Requer backend/PostgreSQL isolados; não substitui teste de dispositivo.'
        : false,
  );
}
