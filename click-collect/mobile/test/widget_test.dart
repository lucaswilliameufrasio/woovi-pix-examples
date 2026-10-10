import 'package:click_collect_app/click_collect_api.dart';
import 'package:click_collect_app/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class FakeClickCollectService implements ClickCollectService {
  DemoOrder currentOrder = _order(PaymentState.pending);

  @override
  Future<DemoOrder> cancel(ReservedOrder order) async {
    currentOrder = _order(PaymentState.cancelled);
    return currentOrder;
  }

  @override
  void close() {}

  @override
  Future<List<Product>> listProducts() async => const [
    Product(
      id: 'house-cake',
      title: 'Bolo de fubá da casa',
      description: 'Fatia preparada hoje.',
      priceCents: 890,
      availableUnits: 3,
    ),
  ];

  @override
  Future<List<ReservedOrder>> loadSavedOrders() async => [];

  @override
  Future<DemoOrder> refresh(ReservedOrder order) async => currentOrder;

  @override
  Future<ReservedOrder> reserve(String productId) async => ReservedOrder(
    order: currentOrder,
    pickupCode: 'A1B2C3D4',
    accessToken: List<String>.filled(64, 'a').join(),
  );

  @override
  Future<DemoOrder> simulatePayment(ReservedOrder order) async {
    currentOrder = _order(PaymentState.paid);
    return currentOrder;
  }
}

DemoOrder _order(PaymentState paymentState) => DemoOrder(
  id: '0123456789abcdef0123456789abcdef',
  productId: 'house-cake',
  productTitle: 'Bolo de fubá da casa',
  amountCents: 890,
  paymentState: paymentState,
  fulfillmentState: paymentState == PaymentState.paid
      ? FulfillmentState.preparing
      : FulfillmentState.awaitingPayment,
  expiresAt: DateTime.utc(2026, 10, 9, 15),
  createdAt: DateTime.utc(2026, 10, 9, 14, 57),
);

void main() {
  testWidgets('Should reserve and show a separate payment state', (
    tester,
  ) async {
    final service = FakeClickCollectService();
    await tester.pumpWidget(BalcaoApp(service: service));
    await tester.pumpAndSettle();

    expect(find.text('Hoje no balcão'), findsOneWidget);
    expect(find.text('Bolo de fubá da casa'), findsOneWidget);
    final reserve = find.text('Reservar uma fatia');
    await tester.ensureVisible(reserve);
    await tester.tap(reserve);
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('reservation-saved')), findsOneWidget);
    expect(find.text('Aguardando confirmação'), findsNWidgets(2));
    expect(find.text('Simular confirmação local'), findsOneWidget);
    expect(find.text('A cozinha está preparando'), findsNothing);

    final simulate = find.text('Simular confirmação local');
    await tester.ensureVisible(simulate);
    await tester.pumpAndSettle();
    await tester.tap(simulate);
    await tester.pumpAndSettle();

    expect(find.text('Confirmado'), findsOneWidget);
    expect(find.text('A cozinha está preparando'), findsOneWidget);
  });
}
