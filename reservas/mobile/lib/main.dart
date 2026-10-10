import 'dart:async';

import 'package:flutter/material.dart';

import 'reservations_api.dart';

const _paper = Color(0xFFF1EFE8);
const _ink = Color(0xFF24372E);
const _clay = Color(0xFF9B5E43);

void main() {
  runApp(ReservationsApp(api: ReservationsApi()));
}

class ReservationsApp extends StatelessWidget {
  const ReservationsApp({super.key, required this.api});

  final ReservationsApi api;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Reserva',
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
        ),
      ),
      home: ReservationsHome(api: api),
    );
  }
}

sealed class BookingState {
  const BookingState();
}

final class BookingLoading extends BookingState {
  const BookingLoading();
}

final class BookingReady extends BookingState {
  const BookingReady(this.availability);
  final Availability availability;
}

final class BookingError extends BookingState {
  const BookingError(this.message);
  final String message;
}

final class BookingSubmitting extends BookingState {
  const BookingSubmitting();
}

final class BookingSaved extends BookingState {
  const BookingSaved(this.reservation, this.capability);
  final Reservation reservation;
  final String capability;
}

final class BookingUnknown extends BookingState {
  const BookingUnknown(this.message);
  final String message;
}

class ReservationsHome extends StatefulWidget {
  const ReservationsHome({super.key, required this.api});

  final ReservationsApi api;

  @override
  State<ReservationsHome> createState() => _ReservationsHomeState();
}

class _ReservationsHomeState extends State<ReservationsHome> {
  late String _date;
  BookingState _state = const BookingLoading();
  Reservation? _savedReservation;
  String? _capability;
  bool _working = false;

  @override
  void initState() {
    super.initState();
    _date = _nextBusinessDate();
    unawaited(_restore());
  }

  @override
  void dispose() {
    widget.api.close();
    super.dispose();
  }

  Future<void> _restore() async {
    try {
      final saved = await widget.api.restore();
      if (!mounted) return;
      if (saved != null) {
        final id = await _readSavedId();
        final capability = await _readSavedCapability();
        if (id != null && capability != null) {
          setState(() {
            _savedReservation = saved;
            _capability = capability;
            _state = BookingSaved(saved, capability);
          });
          return;
        }
      }
      await _loadAvailability();
    } catch (error) {
      if (!mounted) return;
      setState(() => _state = BookingError(error.toString()));
    }
  }

  Future<String?> _readSavedId() async => widget.api.savedId;
  Future<String?> _readSavedCapability() async => widget.api.savedCapability;

  Future<void> _loadAvailability() async {
    setState(() => _state = const BookingLoading());
    try {
      final availability = await widget.api.availability(_date);
      if (!mounted) return;
      setState(() => _state = BookingReady(availability));
    } catch (error) {
      if (!mounted) return;
      setState(() => _state = BookingError(error.toString()));
    }
  }

  Future<void> _pickDate() async {
    final today = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: DateTime.parse(_date),
      firstDate: DateTime(today.year, today.month, today.day),
      lastDate: DateTime(today.year, today.month, today.day + 14),
      helpText: 'Escolha o dia da reserva',
    );
    if (picked == null || !mounted) return;
    setState(() => _date = _formatDate(picked));
    await _loadAvailability();
  }

  Future<void> _reserve(Slot slot) async {
    setState(() => _state = const BookingSubmitting());
    try {
      final saved = await widget.api.createHold(slot.startsAt);
      if (!mounted) return;
      setState(() {
        _savedReservation = saved.reservation;
        _capability = saved.capability;
        _state = BookingSaved(saved.reservation, saved.capability);
      });
    } on UnknownReservationOutcome catch (error) {
      if (!mounted) return;
      setState(() => _state = BookingUnknown(error.toString()));
    } catch (error) {
      if (!mounted) return;
      setState(() => _state = BookingError(error.toString()));
    }
  }

  Future<void> _refresh() async {
    final reservation = _savedReservation;
    final capability = _capability;
    if (reservation == null || capability == null || _working) return;
    setState(() => _working = true);
    try {
      final updated = await widget.api.reservation(reservation.id, capability);
      if (!mounted) return;
      setState(() {
        _savedReservation = updated;
        _state = BookingSaved(updated, capability);
      });
    } catch (error) {
      if (mounted) _showMessage(error.toString());
    } finally {
      if (mounted) setState(() => _working = false);
    }
  }

  Future<void> _cancel() async {
    final reservation = _savedReservation;
    final capability = _capability;
    if (reservation == null || capability == null || _working) return;
    setState(() => _working = true);
    try {
      final updated = await widget.api.cancel(reservation.id, capability);
      if (!mounted) return;
      setState(() {
        _savedReservation = updated;
        _state = BookingSaved(updated, capability);
      });
    } catch (error) {
      if (mounted) _showMessage(error.toString());
    } finally {
      if (mounted) setState(() => _working = false);
    }
  }

  void _showMessage(String message) {
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: CustomScrollView(
          slivers: [
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(22, 18, 22, 28),
              sliver: SliverList.list(
                children: [
                  Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      const Text(
                        'RESERVA  •  LOCAL',
                        style: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w800,
                          letterSpacing: 1.8,
                          color: _ink,
                        ),
                      ),
                      DecoratedBox(
                        decoration: BoxDecoration(
                          border: Border.all(color: Colors.black26),
                          borderRadius: BorderRadius.circular(24),
                        ),
                        child: const Padding(
                          padding: EdgeInsets.symmetric(
                            horizontal: 11,
                            vertical: 7,
                          ),
                          child: Text(
                            'DEMONSTRAÇÃO',
                            style: TextStyle(fontSize: 9, letterSpacing: 1.4),
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 44),
                  const Text(
                    'AGENDA ABERTA · SÃO PAULO',
                    style: TextStyle(
                      fontSize: 10,
                      letterSpacing: 2,
                      fontWeight: FontWeight.bold,
                      color: _clay,
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    'Um tempo\nsó seu.',
                    style: TextStyle(
                      fontFamily: 'serif',
                      fontSize: 58,
                      height: .98,
                      letterSpacing: -2,
                      color: _ink,
                    ),
                  ),
                  const SizedBox(height: 14),
                  const Text(
                    'Escolha seu horário. A reserva fica retida por cinco minutos enquanto você confirma a simulação local.',
                    style: TextStyle(
                      fontSize: 15,
                      height: 1.5,
                      color: Color(0xFF626A62),
                    ),
                  ),
                  const SizedBox(height: 28),
                  Card(
                    color: const Color(0xFFFFFEFA),
                    elevation: 0,
                    shape: RoundedRectangleBorder(
                      side: const BorderSide(color: Color(0xFFD9D7CC)),
                      borderRadius: BorderRadius.circular(2),
                    ),
                    child: Padding(
                      padding: const EdgeInsets.all(20),
                      child: _buildContent(),
                    ),
                  ),
                  const SizedBox(height: 28),
                  const Text(
                    'DEMO LOCAL · NENHUM PAGAMENTO REAL É PROCESSADO',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 9,
                      letterSpacing: 1.1,
                      color: Color(0xFF74796F),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildContent() {
    return switch (_state) {
      BookingLoading() => const Center(
        child: Padding(
          padding: EdgeInsets.all(26),
          child: CircularProgressIndicator(),
        ),
      ),
      BookingReady(:final availability) => _availabilityContent(availability),
      BookingError(:final message) => Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(message),
          const SizedBox(height: 12),
          FilledButton(
            onPressed: _loadAvailability,
            child: const Text('Tentar novamente'),
          ),
        ],
      ),
      BookingSubmitting() => const Center(
        child: Padding(
          padding: EdgeInsets.all(26),
          child: CircularProgressIndicator(),
        ),
      ),
      BookingSaved(:final reservation) => _reservationContent(reservation),
      BookingUnknown(:final message) => Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Não foi possível confirmar se a solicitação chegou ao servidor.',
          ),
          const SizedBox(height: 8),
          Text(message),
          const SizedBox(height: 14),
          OutlinedButton(
            onPressed: _loadAvailability,
            child: const Text('Atualizar horários antes de tentar novamente'),
          ),
        ],
      ),
    };
  }

  Widget _availabilityContent(Availability availability) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          availability.resourceName.toUpperCase(),
          style: const TextStyle(
            fontSize: 10,
            letterSpacing: 1.6,
            color: _clay,
            fontWeight: FontWeight.bold,
          ),
        ),
        const SizedBox(height: 7),
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            const Flexible(
              child: Text(
                'Escolha um horário',
                style: TextStyle(
                  fontFamily: 'serif',
                  fontSize: 24,
                  color: _ink,
                ),
              ),
            ),
            Text(
              'R\$ ${(availability.priceCents / 100).toStringAsFixed(2).replaceAll('.', ',')}',
              style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          '30 minutos · fuso ${availability.timeZone}',
          style: const TextStyle(fontSize: 12, color: Color(0xFF73776F)),
        ),
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: _pickDate,
          icon: const Icon(Icons.calendar_month_outlined),
          label: Text(_date),
        ),
        if (availability.slots.isEmpty)
          const Padding(
            padding: EdgeInsets.only(top: 16),
            child: Text('Sem horários disponíveis para este dia.'),
          ),
        for (final slot in availability.slots) ...[
          const SizedBox(height: 8),
          SizedBox(
            width: double.infinity,
            child: OutlinedButton(
              onPressed: () => _reserve(slot),
              style: OutlinedButton.styleFrom(
                alignment: Alignment.centerLeft,
                padding: const EdgeInsets.all(14),
                foregroundColor: _ink,
                side: const BorderSide(color: Color(0xFFD5D8CE)),
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(slot.localLabel),
                  const Icon(Icons.arrow_forward, size: 16),
                ],
              ),
            ),
          ),
        ],
      ],
    );
  }

  Widget _reservationContent(Reservation reservation) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'SUA RESERVA',
          style: TextStyle(
            fontSize: 10,
            letterSpacing: 1.6,
            color: _clay,
            fontWeight: FontWeight.bold,
          ),
        ),
        const SizedBox(height: 10),
        Text(
          reservation.localLabel,
          style: const TextStyle(
            fontFamily: 'serif',
            fontSize: 24,
            color: _ink,
          ),
        ),
        Text(
          reservation.timeZone,
          style: const TextStyle(color: Color(0xFF73776F)),
        ),
        const Divider(height: 28),
        Text('Reserva: ${reservation.reservationState}'),
        Text('Pagamento: ${reservation.paymentState}'),
        Text(
          'R\$ ${(reservation.amountCents / 100).toStringAsFixed(2).replaceAll('.', ',')}',
        ),
        if (reservation.reservationState == 'held') ...[
          const SizedBox(height: 12),
          const Text(
            'Retenção por cinco minutos. O pagamento é simulado separadamente no operador local.',
          ),
          const SizedBox(height: 8),
          OutlinedButton(
            onPressed: _working ? null : _cancel,
            child: const Text('Cancelar retenção'),
          ),
        ],
        const SizedBox(height: 6),
        TextButton.icon(
          onPressed: _working ? null : _refresh,
          icon: const Icon(Icons.refresh),
          label: const Text('Atualizar estado'),
        ),
        const Text(
          'Nenhum Pix ou pagamento real foi criado.',
          style: TextStyle(fontSize: 12, color: Color(0xFF73776F)),
        ),
      ],
    );
  }

  String _formatDate(DateTime value) =>
      '${value.year.toString().padLeft(4, '0')}-${value.month.toString().padLeft(2, '0')}-${value.day.toString().padLeft(2, '0')}';

  String _nextBusinessDate() {
    final now = DateTime.now().toUtc().subtract(const Duration(hours: 3));
    var date = DateTime.utc(
      now.year,
      now.month,
      now.day,
    ).add(const Duration(days: 1));
    while (date.weekday == DateTime.saturday ||
        date.weekday == DateTime.sunday) {
      date = date.add(const Duration(days: 1));
    }
    return _formatDate(date);
  }
}
