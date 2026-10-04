import 'dart:async';

import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'offers_api.dart';

const _forest = Color(0xFF153D2C);
const _leaf = Color(0xFF2E6B48);
const _lime = Color(0xFFD7F36A);
const _paper = Color(0xFFF5F4EA);
const _ink = Color(0xFF17231C);
const _muted = Color(0xFF66736A);

void main() {
  runApp(const FlashOffersApp());
}

class FlashOffersApp extends StatelessWidget {
  const FlashOffersApp({super.key, this.api});

  final OffersApi? api;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Última Chamada',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        scaffoldBackgroundColor: _paper,
        colorScheme: ColorScheme.fromSeed(
          seedColor: _forest,
          primary: _forest,
          surface: _paper,
        ),
        fontFamily: 'Roboto',
      ),
      home: OffersHome(api: api ?? OffersApi.fromEnvironment()),
    );
  }
}

class OffersHome extends StatefulWidget {
  const OffersHome({super.key, required this.api});

  final OffersApi api;

  @override
  State<OffersHome> createState() => _OffersHomeState();
}

class _OffersHomeState extends State<OffersHome> with WidgetsBindingObserver {
  late Future<List<Offer>> _offers;
  Timer? _orderPollTimer;
  Duration _pollInterval = const Duration(seconds: 5);
  bool _creatingOrder = false;
  bool _refreshingOrder = false;
  bool _loadingHistory = false;
  List<DemoOrder> _orderHistory = const [];
  bool _historyRefreshFailed = false;
  DemoOrder? _order;
  String? _error;
  String? _orderError;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _offers = widget.api.listOffers();
    unawaited(_loadOrderHistory());
  }

  @override
  void dispose() {
    _orderPollTimer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      if (_order?.state == 'pending_payment') {
        unawaited(_refreshOrder());
      }
    } else {
      _orderPollTimer?.cancel();
    }
  }

  void _scheduleOrderPoll() {
    _orderPollTimer?.cancel();
    if (_order?.state != 'pending_payment' ||
        WidgetsBinding.instance.lifecycleState != AppLifecycleState.resumed) {
      return;
    }
    _orderPollTimer = Timer(_pollInterval, () => unawaited(_refreshOrder()));
  }

  Future<void> _refresh({bool clearError = true}) async {
    setState(() {
      if (clearError) _error = null;
      _offers = widget.api.listOffers();
    });
    try {
      await _offers;
    } catch (_) {
      // FutureBuilder renders the offline state; keep refresh errors in the UI.
    }
  }

  Future<void> _reserve(Offer offer) async {
    setState(() {
      _creatingOrder = true;
      _error = null;
    });
    try {
      final order = await widget.api.createOrder(offer.id);
      if (!mounted) return;
      setState(() {
        _order = order;
        _orderHistory = [
          order,
          ..._orderHistory.where((item) => item.id != order.id),
        ];
      });
      try {
        await _saveOrderId(order.id);
      } catch (_) {
        // A storage error must not turn a successfully created order into UI failure.
      }
      _pollInterval = const Duration(seconds: 5);
      _scheduleOrderPoll();
      await _refresh();
    } on OffersApiException catch (error) {
      if (!mounted) return;
      setState(() => _error = error.message);
      await _refresh(clearError: false);
    } catch (_) {
      if (!mounted) return;
      setState(() => _error = 'Não foi possível conectar à loja local.');
    } finally {
      if (mounted) setState(() => _creatingOrder = false);
    }
  }

  Future<void> _refreshOrder() async {
    final currentOrder = _order;
    if (currentOrder == null || _refreshingOrder) return;
    setState(() {
      _refreshingOrder = true;
      _orderError = null;
    });
    try {
      final latest = await widget.api.getOrder(currentOrder.id);
      if (mounted) {
        setState(() {
          _order = latest;
          _orderHistory = [
            latest,
            ..._orderHistory.where((item) => item.id != latest.id),
          ];
        });
        if (latest.state == 'pending_payment') {
          _pollInterval = const Duration(seconds: 5);
        } else {
          _orderPollTimer?.cancel();
        }
      }
    } on OffersApiException catch (error) {
      if (mounted) {
        setState(() => _orderError = error.message);
        _increasePollInterval();
      }
    } catch (_) {
      if (mounted) {
        setState(() => _orderError = 'Não foi possível atualizar o pedido.');
        _increasePollInterval();
      }
    } finally {
      if (mounted) {
        setState(() => _refreshingOrder = false);
        _scheduleOrderPoll();
      }
    }
  }

  void _increasePollInterval() {
    final nextSeconds = (_pollInterval.inSeconds * 2).clamp(5, 60);
    _pollInterval = Duration(seconds: nextSeconds);
  }

  Future<void> _loadOrderHistory() async {
    setState(() => _loadingHistory = true);
    try {
      final preferences = await SharedPreferences.getInstance();
      final ids = preferences.getStringList('offers.order_history') ?? const [];
      final orders = <DemoOrder>[];
      var failedRequests = 0;
      for (final id in ids.take(20)) {
        try {
          orders.add(await widget.api.getOrder(id));
        } catch (_) {
          failedRequests++;
          // Keep unreachable orders out of the displayed history; IDs remain
          // locally stored so a transient network failure does not erase them.
        }
      }
      if (mounted) {
        setState(() {
          _orderHistory = orders;
          _historyRefreshFailed = failedRequests > 0;
        });
      }
    } catch (_) {
      // History is a convenience; catalog and checkout remain usable.
      if (mounted) setState(() => _historyRefreshFailed = true);
    } finally {
      if (mounted) setState(() => _loadingHistory = false);
    }
  }

  Future<void> _saveOrderId(String orderId) async {
    final preferences = await SharedPreferences.getInstance();
    final ids = preferences.getStringList('offers.order_history') ?? const [];
    await preferences.setStringList(
      'offers.order_history',
      [orderId, ...ids.where((id) => id != orderId)].take(20).toList(),
    );
  }

  Future<void> _showOrderHistory() async {
    await _loadOrderHistory();
    if (!mounted) return;
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: _paper,
      builder: (context) => StatefulBuilder(
        builder: (sheetContext, setSheetState) => SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(20, 18, 20, 24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Expanded(
                      child: Text(
                        'Meus pedidos',
                        style: TextStyle(
                          color: _forest,
                          fontSize: 22,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                    ),
                    IconButton(
                      tooltip: 'Atualizar pedidos',
                      onPressed: _loadingHistory
                          ? null
                          : () async {
                              final refresh = _loadOrderHistory();
                              setSheetState(() {});
                              await refresh;
                              if (sheetContext.mounted) setSheetState(() {});
                            },
                      icon: const Icon(Icons.refresh_rounded),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                if (_loadingHistory)
                  const Center(child: CircularProgressIndicator())
                else if (_orderHistory.isEmpty && _historyRefreshFailed)
                  const Padding(
                    padding: EdgeInsets.symmetric(vertical: 24),
                    child: Text(
                      'Não foi possível consultar seus pedidos agora. Tente atualizar.',
                      style: TextStyle(color: _muted),
                    ),
                  )
                else if (_orderHistory.isEmpty)
                  const Padding(
                    padding: EdgeInsets.symmetric(vertical: 24),
                    child: Text(
                      'Seus pedidos feitos neste aparelho aparecem aqui.',
                      style: TextStyle(color: _muted),
                    ),
                  )
                else
                  Flexible(
                    child: ListView.separated(
                      shrinkWrap: true,
                      itemCount: _orderHistory.length,
                      separatorBuilder: (_, _) => const Divider(height: 1),
                      itemBuilder: (context, index) {
                        final order = _orderHistory[index];
                        return ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: const CircleAvatar(
                            backgroundColor: Color(0xFFEAF2D8),
                            child: Icon(
                              Icons.shopping_bag_outlined,
                              color: _leaf,
                            ),
                          ),
                          title: Text(
                            _orderStateLabel(order.state),
                            style: const TextStyle(
                              color: _forest,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          subtitle: Text('Pedido ${order.id}'),
                          trailing: Text(
                            _formatPrice(order.amountCents),
                            style: const TextStyle(
                              color: _forest,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                        );
                      },
                    ),
                  ),
                const Padding(
                  padding: EdgeInsets.only(top: 12),
                  child: Text(
                    'Histórico local deste aparelho. O backend continua sendo a fonte do estado.',
                    style: TextStyle(color: _muted, fontSize: 11),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: FutureBuilder<List<Offer>>(
          future: _offers,
          builder: (context, snapshot) {
            return RefreshIndicator(
              color: _forest,
              onRefresh: _refresh,
              child: ListView(
                padding: const EdgeInsets.fromLTRB(22, 14, 22, 36),
                children: [
                  _topBar(),
                  const SizedBox(height: 30),
                  _hero(),
                  const SizedBox(height: 28),
                  if (_error != null) _errorBanner(_error!),
                  if (_order case final order?) ...[
                    _orderConfirmation(order),
                    const SizedBox(height: 22),
                  ],
                  _sectionHeading(),
                  const SizedBox(height: 14),
                  if (snapshot.connectionState == ConnectionState.waiting &&
                      !snapshot.hasData)
                    const Padding(
                      padding: EdgeInsets.symmetric(vertical: 38),
                      child: Center(child: CircularProgressIndicator()),
                    )
                  else if (snapshot.hasError)
                    _offlineState()
                  else if (snapshot.data case final offers? when offers.isEmpty)
                    _emptyState()
                  else if (snapshot.data case final offers?)
                    ...offers.map(
                      (offer) => Padding(
                        padding: const EdgeInsets.only(bottom: 14),
                        child: _offerCard(offer),
                      ),
                    ),
                  const SizedBox(height: 16),
                  const Center(
                    child: Text(
                      'DEMO LOCAL · SEM PAGAMENTO REAL',
                      style: TextStyle(
                        color: _muted,
                        fontSize: 10,
                        letterSpacing: 1.1,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                ],
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _topBar() => Row(
    children: [
      Container(
        width: 42,
        height: 42,
        decoration: const BoxDecoration(color: _forest, shape: BoxShape.circle),
        child: const Icon(Icons.eco_outlined, color: _lime, size: 23),
      ),
      const SizedBox(width: 11),
      const Expanded(
        child: Text(
          'última chamada',
          style: TextStyle(
            color: _forest,
            fontSize: 18,
            fontWeight: FontWeight.w800,
            letterSpacing: -0.5,
          ),
        ),
      ),
      IconButton(
        tooltip: 'Meus pedidos',
        onPressed: _showOrderHistory,
        icon: const Icon(Icons.receipt_long_outlined, color: _forest),
      ),
      IconButton(
        tooltip: 'Atualizar ofertas',
        onPressed: _refresh,
        icon: const Icon(Icons.refresh_rounded, color: _forest),
      ),
    ],
  );

  Widget _hero() => Container(
    padding: const EdgeInsets.fromLTRB(22, 22, 20, 20),
    decoration: BoxDecoration(
      color: _forest,
      borderRadius: BorderRadius.circular(26),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const Icon(Icons.wb_twilight_rounded, color: _lime, size: 17),
            const SizedBox(width: 7),
            Text(
              'BOM DEMAIS PRA DESCARTAR',
              style: TextStyle(
                color: _lime.withValues(alpha: 0.94),
                fontSize: 10,
                fontWeight: FontWeight.w800,
                letterSpacing: 1.05,
              ),
            ),
          ],
        ),
        const SizedBox(height: 18),
        const Text(
          'O fim do dia\nchega com sabor.',
          style: TextStyle(
            color: Colors.white,
            fontSize: 31,
            height: 1.08,
            fontWeight: FontWeight.w800,
            letterSpacing: -1.1,
          ),
        ),
        const SizedBox(height: 12),
        Text(
          'Uma sacola surpresa da vizinhança,\nreservada por você e retirada na loja.',
          style: TextStyle(
            color: Colors.white.withValues(alpha: 0.75),
            height: 1.45,
            fontSize: 13,
          ),
        ),
        const SizedBox(height: 21),
        Row(
          children: [
            _heroTag(Icons.storefront_outlined, 'retirada local'),
            const SizedBox(width: 8),
            _heroTag(Icons.schedule_rounded, 'hoje'),
          ],
        ),
      ],
    ),
  );

  Widget _heroTag(IconData icon, String label) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
    decoration: BoxDecoration(
      color: Colors.white.withValues(alpha: 0.1),
      borderRadius: BorderRadius.circular(20),
    ),
    child: Row(
      children: [
        Icon(icon, color: _lime, size: 14),
        const SizedBox(width: 5),
        Text(label, style: const TextStyle(color: Colors.white, fontSize: 11)),
      ],
    ),
  );

  Widget _sectionHeading() => Row(
    crossAxisAlignment: CrossAxisAlignment.end,
    children: [
      const Expanded(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Oferta de hoje',
              style: TextStyle(
                color: _ink,
                fontSize: 22,
                fontWeight: FontWeight.w800,
                letterSpacing: -0.6,
              ),
            ),
            SizedBox(height: 3),
            Text(
              'Poucas sacolas. Boas descobertas.',
              style: TextStyle(color: _muted, fontSize: 12),
            ),
          ],
        ),
      ),
      const Icon(Icons.storefront_outlined, size: 15, color: _leaf),
      const SizedBox(width: 4),
      const Text('na loja', style: TextStyle(color: _leaf, fontSize: 12)),
    ],
  );

  Widget _offerCard(Offer offer) {
    final available = offer.availableUnits > 0;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(21),
        border: Border.all(color: const Color(0xFFE7E9DD)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 63,
                height: 63,
                decoration: BoxDecoration(
                  color: const Color(0xFFEAF2D8),
                  borderRadius: BorderRadius.circular(17),
                ),
                child: const Icon(
                  Icons.shopping_basket_outlined,
                  color: _leaf,
                  size: 30,
                ),
              ),
              const SizedBox(width: 13),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      available
                          ? '${offer.availableUnits} ${offer.availableUnits == 1 ? 'sacola' : 'sacolas'} restantes'
                          : 'esgotado por hoje',
                      style: TextStyle(
                        color: available ? _leaf : _muted,
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 5),
                    Text(
                      offer.title,
                      style: const TextStyle(
                        color: _ink,
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        letterSpacing: -0.35,
                      ),
                    ),
                    const SizedBox(height: 4),
                    const Text(
                      'Seleção surpresa do dia',
                      style: TextStyle(color: _muted, fontSize: 11),
                    ),
                  ],
                ),
              ),
              Text(
                _formatPrice(offer.priceCents),
                style: const TextStyle(
                  color: _forest,
                  fontSize: 16,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ],
          ),
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 14),
            child: Divider(height: 1, color: Color(0xFFEDEFE6)),
          ),
          Row(
            children: [
              const Icon(Icons.location_on_outlined, size: 15, color: _muted),
              const SizedBox(width: 4),
              const Expanded(
                child: Text(
                  'Retirada presencial na loja',
                  style: TextStyle(color: _muted, fontSize: 11),
                ),
              ),
              FilledButton(
                onPressed: available && !_creatingOrder
                    ? () => _reserve(offer)
                    : null,
                style: FilledButton.styleFrom(
                  backgroundColor: _forest,
                  foregroundColor: Colors.white,
                  disabledBackgroundColor: const Color(0xFFBBC3B9),
                  padding: const EdgeInsets.symmetric(
                    horizontal: 15,
                    vertical: 10,
                  ),
                  minimumSize: const Size(0, 39),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(13),
                  ),
                ),
                child: _creatingOrder
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : Text(available ? 'Reservar' : 'Indisponível'),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _orderConfirmation(DemoOrder order) => Container(
    padding: const EdgeInsets.all(16),
    decoration: BoxDecoration(
      color: const Color(0xFFEAF2D8),
      borderRadius: BorderRadius.circular(18),
      border: Border.all(color: const Color(0xFFC8DDA7)),
    ),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Icon(Icons.check_circle_outline, color: _leaf),
        const SizedBox(width: 11),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'Sacola reservada',
                style: TextStyle(
                  color: _forest,
                  fontWeight: FontWeight.w800,
                  fontSize: 14,
                ),
              ),
              const SizedBox(height: 3),
              Text(
                'Pedido ${order.id} · ${_formatPrice(order.amountCents)}',
                style: const TextStyle(color: _leaf, fontSize: 11),
              ),
              const SizedBox(height: 6),
              Text(
                'Estado: ${_orderStateLabel(order.state)}',
                style: const TextStyle(
                  color: _forest,
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 5),
              const Text(
                'Pagamento ainda não integrado. Atualizamos o estado enquanto aguarda.',
                style: TextStyle(color: _muted, fontSize: 11, height: 1.35),
              ),
              if (_orderError case final error?) ...[
                const SizedBox(height: 5),
                Text(
                  error,
                  style: const TextStyle(
                    color: Color(0xFF713D22),
                    fontSize: 11,
                  ),
                ),
              ],
              const SizedBox(height: 4),
              TextButton.icon(
                onPressed: _refreshingOrder ? null : _refreshOrder,
                icon: _refreshingOrder
                    ? const SizedBox(
                        width: 14,
                        height: 14,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.refresh_rounded, size: 16),
                label: const Text('Atualizar estado'),
                style: TextButton.styleFrom(
                  foregroundColor: _forest,
                  padding: EdgeInsets.zero,
                  visualDensity: VisualDensity.compact,
                ),
              ),
            ],
          ),
        ),
      ],
    ),
  );

  Widget _errorBanner(String message) => Container(
    margin: const EdgeInsets.only(bottom: 16),
    padding: const EdgeInsets.all(13),
    decoration: BoxDecoration(
      color: const Color(0xFFFFE8D8),
      borderRadius: BorderRadius.circular(14),
    ),
    child: Text(message, style: const TextStyle(color: Color(0xFF713D22))),
  );

  Widget _offlineState() => Padding(
    padding: const EdgeInsets.symmetric(vertical: 28),
    child: Column(
      children: [
        const Icon(Icons.cloud_off_outlined, size: 32, color: _muted),
        const SizedBox(height: 9),
        const Text(
          'Não foi possível carregar as ofertas.',
          style: TextStyle(color: _ink, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 4),
        const Text(
          'Confira se a API local está rodando.',
          style: TextStyle(color: _muted, fontSize: 12),
        ),
        TextButton(onPressed: _refresh, child: const Text('Tentar de novo')),
      ],
    ),
  );

  Widget _emptyState() => const Padding(
    padding: EdgeInsets.symmetric(vertical: 32),
    child: Center(
      child: Text(
        'As sacolas de hoje já foram reservadas.',
        style: TextStyle(color: _muted),
      ),
    ),
  );

  String _formatPrice(int cents) =>
      'R\$ ${(cents / 100).toStringAsFixed(2).replaceAll('.', ',')}';

  String _orderStateLabel(String state) => switch (state) {
    'pending_payment' => 'Aguardando pagamento',
    'paid' => 'Pago',
    'expired' => 'Reserva expirada',
    'payment_exception' => 'Pagamento em análise',
    _ => 'Status indisponível',
  };
}
