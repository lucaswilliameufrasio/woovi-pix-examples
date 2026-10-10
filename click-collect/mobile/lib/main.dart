import 'dart:async';

import 'package:flutter/material.dart';

import 'click_collect_api.dart';

const _ink = Color(0xFF20363C);
const _paper = Color(0xFFF3F4EE);
const _tomato = Color(0xFFC94F3D);
const _leaf = Color(0xFF5E795B);
const _label = Color(0xFFE3BD4F);

void main() {
  runApp(BalcaoApp(service: ClickCollectApi.fromEnvironment()));
}

class BalcaoApp extends StatelessWidget {
  const BalcaoApp({super.key, required this.service});

  final ClickCollectService service;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Balcão',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        scaffoldBackgroundColor: _paper,
        colorScheme: ColorScheme.fromSeed(
          seedColor: _ink,
          primary: _ink,
          surface: _paper,
        ),
        appBarTheme: const AppBarTheme(
          backgroundColor: _paper,
          foregroundColor: _ink,
          surfaceTintColor: Colors.transparent,
        ),
        fontFamily: 'Roboto',
      ),
      home: ClickCollectHome(service: service),
    );
  }
}

sealed class ReservationState {
  const ReservationState();
}

final class ReservationIdle extends ReservationState {
  const ReservationIdle();
}

final class ReservationSubmitting extends ReservationState {
  const ReservationSubmitting();
}

final class ReservationSaved extends ReservationState {
  const ReservationSaved(this.order);
  final ReservedOrder order;
}

final class ReservationFailed extends ReservationState {
  const ReservationFailed(this.message);
  final String message;
}

final class ReservationUnknown extends ReservationState {
  const ReservationUnknown();
}

enum OrderActionMode { idle, working, failed, complete }

class ClickCollectHome extends StatefulWidget {
  const ClickCollectHome({super.key, required this.service});

  final ClickCollectService service;

  @override
  State<ClickCollectHome> createState() => _ClickCollectHomeState();
}

class _ClickCollectHomeState extends State<ClickCollectHome>
    with WidgetsBindingObserver {
  late Future<List<Product>> _catalog;
  ReservationState _reservation = const ReservationIdle();
  OrderActionMode _actionMode = OrderActionMode.idle;
  String? _actionMessage;
  List<ReservedOrder> _orders = const [];
  ReservedOrder? _selected;
  bool _loadingOrders = true;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _catalog = widget.service.listProducts();
    unawaited(_loadSavedOrders());
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.service.close();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed && _selected != null) {
      unawaited(_refreshSelected());
    }
  }

  Future<void> _loadSavedOrders() async {
    try {
      final orders = await widget.service.loadSavedOrders();
      if (!mounted) return;
      setState(() {
        _orders = orders;
        _selected = orders.isEmpty ? null : orders.first;
        _loadingOrders = false;
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _loadingOrders = false;
        _actionMode = OrderActionMode.failed;
        _actionMessage = error.toString();
      });
    }
  }

  Future<void> _reserve(Product product) async {
    setState(() => _reservation = const ReservationSubmitting());
    try {
      final reserved = await widget.service.reserve(product.id);
      if (!mounted) return;
      setState(() {
        _reservation = ReservationSaved(reserved);
        _orders = [
          reserved,
          ..._orders.where((item) => item.order.id != reserved.order.id),
        ];
        _selected = reserved;
        _actionMode = OrderActionMode.idle;
        _actionMessage = null;
      });
    } on UnknownReservationOutcome {
      if (!mounted) return;
      setState(() => _reservation = const ReservationUnknown());
    } catch (error) {
      if (!mounted) return;
      setState(() => _reservation = ReservationFailed(error.toString()));
    }
  }

  Future<void> _refreshCatalog() async {
    setState(() => _catalog = widget.service.listProducts());
  }

  Future<void> _refreshSelected() async {
    final selected = _selected;
    if (selected == null || _actionMode == OrderActionMode.working) return;
    await _runOrderAction(() => widget.service.refresh(selected));
  }

  Future<void> _simulatePayment() async {
    final selected = _selected;
    if (selected == null) return;
    await _runOrderAction(() => widget.service.simulatePayment(selected));
  }

  Future<void> _cancelReservation() async {
    final selected = _selected;
    if (selected == null) return;
    await _runOrderAction(() => widget.service.cancel(selected));
  }

  Future<void> _runOrderAction(Future<DemoOrder> Function() action) async {
    setState(() {
      _actionMode = OrderActionMode.working;
      _actionMessage = null;
    });
    try {
      final updated = await action();
      if (!mounted) return;
      final selected = _selected;
      if (selected == null) return;
      final saved = ReservedOrder(
        order: updated,
        pickupCode: selected.pickupCode,
        accessToken: selected.accessToken,
      );
      setState(() {
        _selected = saved;
        _orders = [
          saved,
          ..._orders.where((item) => item.order.id != updated.id),
        ];
        _actionMode = OrderActionMode.complete;
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _actionMode = OrderActionMode.failed;
        _actionMessage = error.toString();
      });
    }
  }

  String _paymentLabel(PaymentState state) => switch (state) {
    PaymentState.pending => 'Aguardando confirmação',
    PaymentState.paid => 'Confirmado',
    PaymentState.expired => 'Reserva vencida',
    PaymentState.cancelled => 'Cancelado',
    PaymentState.paymentException => 'Confirmação em análise',
  };

  String _fulfillmentLabel(FulfillmentState state) => switch (state) {
    FulfillmentState.awaitingPayment => 'Aguardando confirmação',
    FulfillmentState.preparing => 'A cozinha está preparando',
    FulfillmentState.readyForPickup => 'Pronto para retirar',
    FulfillmentState.pickedUp => 'Retirado',
  };

  String _money(int cents) =>
      'R\$ ${(cents / 100).toStringAsFixed(2).replaceAll('.', ',')}';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        titleSpacing: 22,
        title: const Row(
          children: [
            Icon(Icons.circle, color: _tomato, size: 13),
            SizedBox(width: 9),
            Text('balcão', style: TextStyle(fontWeight: FontWeight.w900)),
          ],
        ),
        actions: [
          IconButton(
            tooltip: 'Atualizar vitrine',
            onPressed: _refreshCatalog,
            icon: const Icon(Icons.refresh),
          ),
        ],
      ),
      body: FutureBuilder<List<Product>>(
        future: _catalog,
        builder: (context, snapshot) {
          if (snapshot.connectionState == ConnectionState.waiting) {
            return const Center(child: CircularProgressIndicator());
          }
          if (snapshot.hasError) {
            return _offlineState();
          }
          final products = snapshot.data ?? const <Product>[];
          return RefreshIndicator(
            onRefresh: _refreshCatalog,
            child: ListView(
              padding: const EdgeInsets.fromLTRB(22, 10, 22, 36),
              children: [
                _hero(),
                const SizedBox(height: 34),
                const _Eyebrow('DA VITRINE PARA A SUA MESA'),
                const SizedBox(height: 7),
                const Text(
                  'Hoje no balcão',
                  style: TextStyle(fontSize: 30, fontWeight: FontWeight.w900),
                ),
                const SizedBox(height: 17),
                if (products.isEmpty)
                  const _Notice('A vitrine está vazia por enquanto.')
                else
                  for (final product in products) _productCard(product),
                const SizedBox(height: 14),
                const _DemoNote(
                  'Demonstração local. Nenhum pagamento real é processado.',
                ),
                const SizedBox(height: 34),
                _orderSection(),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _hero() => Container(
    padding: const EdgeInsets.fromLTRB(20, 25, 20, 25),
    decoration: const BoxDecoration(color: _ink),
    child: const Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _Eyebrow('COZINHA ABERTA · HOJE', color: _label),
        SizedBox(height: 16),
        Text.rich(
          TextSpan(
            children: [
              TextSpan(text: 'FEITO HOJE.\n'),
              TextSpan(
                text: 'RETIRADO POR VOCÊ.',
                style: TextStyle(color: _label),
              ),
            ],
          ),
          style: TextStyle(
            fontSize: 35,
            height: 1.05,
            fontWeight: FontWeight.w900,
            color: _paper,
          ),
        ),
        SizedBox(height: 13),
        Text(
          'Peça direto do balcão. A gente prepara com calma; você leva quentinho.',
          style: TextStyle(color: Color(0xFFD9E0D9), height: 1.5),
        ),
      ],
    ),
  );

  Widget _productCard(Product product) => Card(
    elevation: 0,
    color: const Color(0xFFFAFAF5),
    shape: RoundedRectangleBorder(
      side: const BorderSide(color: _ink, width: 1),
      borderRadius: BorderRadius.circular(2),
    ),
    child: Padding(
      padding: const EdgeInsets.all(19),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const _Eyebrow('FEITO NA COZINHA · HOJE', color: _leaf),
          const SizedBox(height: 8),
          Text(
            product.title,
            style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 5),
          Text(
            product.description,
            style: const TextStyle(color: Color(0xFF5D6B6E), height: 1.45),
          ),
          const SizedBox(height: 18),
          Row(
            children: [
              Expanded(
                child: Text(
                  _money(product.priceCents),
                  style: const TextStyle(
                    fontSize: 21,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ),
              FilledButton(
                onPressed:
                    product.availableUnits == 0 ||
                        _reservation is ReservationSubmitting
                    ? null
                    : () => _reserve(product),
                style: FilledButton.styleFrom(
                  backgroundColor: _tomato,
                  foregroundColor: Colors.white,
                ),
                child: _reservation is ReservationSubmitting
                    ? const SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : Text(
                        product.availableUnits == 0
                            ? 'Esgotado'
                            : 'Reservar uma fatia',
                      ),
              ),
            ],
          ),
          switch (_reservation) {
            ReservationFailed(:final message) => Padding(
              padding: const EdgeInsets.only(top: 10),
              child: Text(
                message,
                style: const TextStyle(color: _tomato),
                key: const Key('reservation-error'),
              ),
            ),
            ReservationUnknown() => const Padding(
              padding: EdgeInsets.only(top: 10),
              child: Text(
                'Resultado desconhecido. Não reserve novamente às cegas; consulte a loja local.',
                style: TextStyle(color: _tomato),
                key: Key('reservation-unknown'),
              ),
            ),
            ReservationSaved(:final order) => Padding(
              padding: const EdgeInsets.only(top: 10),
              child: Text(
                'Reserva ${order.order.id.substring(0, 8)} salva com acesso seguro.',
                style: const TextStyle(color: _leaf),
                key: const Key('reservation-saved'),
              ),
            ),
            _ => const SizedBox.shrink(),
          },
        ],
      ),
    ),
  );

  Widget _orderSection() {
    if (_loadingOrders) {
      return const Center(child: CircularProgressIndicator());
    }
    final order = _selected;
    if (order == null) {
      return const _Notice(
        'Suas reservas privadas aparecem aqui neste aparelho.',
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const _Eyebrow('ACOMPANHAMENTO PRIVADO'),
        const SizedBox(height: 7),
        const Text(
          'Seu pedido',
          style: TextStyle(fontSize: 28, fontWeight: FontWeight.w900),
        ),
        const SizedBox(height: 15),
        Container(
          padding: const EdgeInsets.all(19),
          decoration: BoxDecoration(
            border: Border.all(color: _ink),
            color: const Color(0xFFFAFAF5),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                order.order.productTitle,
                style: const TextStyle(
                  fontSize: 21,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 12),
              _StateRow(
                label: 'Pagamento',
                value: _paymentLabel(order.order.paymentState),
              ),
              _StateRow(
                label: 'Preparo e retirada',
                value: _fulfillmentLabel(order.order.fulfillmentState),
              ),
              _StateRow(label: 'Total', value: _money(order.order.amountCents)),
              if (order.order.fulfillmentState ==
                  FulfillmentState.readyForPickup) ...[
                const Divider(height: 24),
                const _Eyebrow('MOSTRE ESTE CÓDIGO NO BALCÃO', color: _leaf),
                const SizedBox(height: 5),
                Text(
                  '${order.pickupCode.substring(0, 4)} ${order.pickupCode.substring(4)}',
                  style: const TextStyle(
                    color: _leaf,
                    fontSize: 29,
                    letterSpacing: 4,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ],
              if (order.order.paymentState == PaymentState.pending) ...[
                const SizedBox(height: 16),
                SizedBox(
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: _actionMode == OrderActionMode.working
                        ? null
                        : _simulatePayment,
                    style: FilledButton.styleFrom(backgroundColor: _tomato),
                    child: const Text('Simular confirmação local'),
                  ),
                ),
                TextButton(
                  onPressed: _actionMode == OrderActionMode.working
                      ? null
                      : _cancelReservation,
                  child: const Text('Cancelar reserva'),
                ),
                const _DemoNote(
                  'Sem Pix pagável e sem instituição financeira.',
                ),
              ],
              if (_actionMode == OrderActionMode.failed)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Text(
                    _actionMessage ?? 'Não foi possível atualizar o pedido.',
                    style: const TextStyle(color: _tomato),
                  ),
                ),
              if (_actionMode == OrderActionMode.working)
                const LinearProgressIndicator(color: _tomato),
              Align(
                alignment: Alignment.centerRight,
                child: TextButton.icon(
                  onPressed: _actionMode == OrderActionMode.working
                      ? null
                      : _refreshSelected,
                  icon: const Icon(Icons.refresh, size: 17),
                  label: const Text('Atualizar status'),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _offlineState() => ListView(
    padding: const EdgeInsets.all(28),
    children: [
      const SizedBox(height: 90),
      const Icon(Icons.wifi_off, size: 42, color: _tomato),
      const SizedBox(height: 17),
      const Text(
        'O balcão está offline.',
        textAlign: TextAlign.center,
        style: TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
      ),
      const SizedBox(height: 8),
      const Text(
        'Confira se o backend local está em execução.',
        textAlign: TextAlign.center,
      ),
      const SizedBox(height: 18),
      Center(
        child: OutlinedButton(
          onPressed: _refreshCatalog,
          child: const Text('Tentar novamente'),
        ),
      ),
    ],
  );
}

class _Eyebrow extends StatelessWidget {
  const _Eyebrow(this.text, {this.color = _ink});

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Text(
    text,
    style: TextStyle(
      color: color,
      fontSize: 10,
      fontWeight: FontWeight.w700,
      letterSpacing: 1.3,
    ),
  );
}

class _DemoNote extends StatelessWidget {
  const _DemoNote(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Text(
    text,
    style: const TextStyle(
      color: Color(0xFF647176),
      fontSize: 10,
      height: 1.45,
    ),
  );
}

class _Notice extends StatelessWidget {
  const _Notice(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(16),
    decoration: BoxDecoration(
      border: Border.all(color: _ink.withValues(alpha: .35)),
    ),
    child: Text(text),
  );
}

class _StateRow extends StatelessWidget {
  const _StateRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 6),
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          label,
          style: const TextStyle(color: Color(0xFF647176), fontSize: 12),
        ),
        Flexible(
          child: Text(
            value,
            textAlign: TextAlign.end,
            style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600),
          ),
        ),
      ],
    ),
  );
}
