import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;

class Slot {
  const Slot({
    required this.startsAt,
    required this.localLabel,
    required this.timeZone,
  });

  final String startsAt;
  final String localLabel;
  final String timeZone;

  factory Slot.fromJson(Map<String, Object?> json) {
    final startsAt = json['starts_at'];
    final localLabel = json['local_label'];
    final timeZone = json['time_zone'];
    if (startsAt is! String ||
        DateTime.tryParse(startsAt) == null ||
        localLabel is! String ||
        timeZone is! String) {
      throw const FormatException('Horário inválido retornado pela API.');
    }
    return Slot(startsAt: startsAt, localLabel: localLabel, timeZone: timeZone);
  }
}

class Availability {
  const Availability({
    required this.resourceName,
    required this.timeZone,
    required this.priceCents,
    required this.slots,
  });

  final String resourceName;
  final String timeZone;
  final int priceCents;
  final List<Slot> slots;

  factory Availability.fromJson(Object? value) {
    if (value is! Map<String, Object?> ||
        value['resource_name'] is! String ||
        value['time_zone'] is! String ||
        value['price_cents'] is! int ||
        value['slots'] is! List<Object?>) {
      throw const FormatException(
        'Disponibilidade inválida retornada pela API.',
      );
    }
    final slots = <Slot>[];
    for (final rawSlot in value['slots'] as List<Object?>) {
      if (rawSlot is! Map<String, Object?>) {
        throw const FormatException('Horário inválido retornado pela API.');
      }
      slots.add(Slot.fromJson(rawSlot));
    }
    return Availability(
      resourceName: value['resource_name'] as String,
      timeZone: value['time_zone'] as String,
      priceCents: value['price_cents'] as int,
      slots: slots,
    );
  }
}

class Reservation {
  const Reservation({
    required this.id,
    required this.localLabel,
    required this.timeZone,
    required this.amountCents,
    required this.reservationState,
    required this.paymentState,
    required this.expiresAt,
  });

  final String id;
  final String localLabel;
  final String timeZone;
  final int amountCents;
  final String reservationState;
  final String paymentState;
  final String expiresAt;

  factory Reservation.fromJson(Object? value) {
    if (value is! Map<String, Object?> ||
        value['id'] is! String ||
        value['local_label'] is! String ||
        value['time_zone'] is! String ||
        value['amount_cents'] is! int ||
        value['reservation_state'] is! String ||
        value['payment_state'] is! String ||
        value['expires_at'] is! String ||
        DateTime.tryParse(value['expires_at'] as String) == null) {
      throw const FormatException('Reserva inválida retornada pela API.');
    }
    return Reservation(
      id: value['id'] as String,
      localLabel: value['local_label'] as String,
      timeZone: value['time_zone'] as String,
      amountCents: value['amount_cents'] as int,
      reservationState: value['reservation_state'] as String,
      paymentState: value['payment_state'] as String,
      expiresAt: value['expires_at'] as String,
    );
  }
}

class PrivateReservation {
  const PrivateReservation({
    required this.reservation,
    required this.capability,
  });

  final Reservation reservation;
  final String capability;
}

class ReservationsApi {
  ReservationsApi({
    http.Client? client,
    FlutterSecureStorage? storage,
    String? baseUrl,
    this.requestTimeout = const Duration(seconds: 5),
  }) : _client = client ?? http.Client(),
       _storage = storage ?? const FlutterSecureStorage(),
       _baseUrl =
           baseUrl ??
           const String.fromEnvironment(
             'API_BASE_URL',
             defaultValue: 'http://127.0.0.1:8084',
           ) {
    final uri = Uri.tryParse(_baseUrl);
    if (uri == null ||
        uri.scheme != 'http' ||
        !const ['127.0.0.1', 'localhost', '::1'].contains(uri.host) ||
        uri.port == 0 ||
        uri.userInfo.isNotEmpty ||
        uri.path.isNotEmpty ||
        uri.query.isNotEmpty ||
        uri.fragment.isNotEmpty) {
      throw ArgumentError.value(
        _baseUrl,
        'baseUrl',
        'A API deve usar uma URL explícita de loopback.',
      );
    }
  }

  final http.Client _client;
  final FlutterSecureStorage _storage;
  final String _baseUrl;
  final Duration requestTimeout;
  static const _reservationKey = 'reservation_id';
  static const _capabilityKey = 'reservation_capability';

  Future<String?> get savedId => _storage.read(key: _reservationKey);
  Future<String?> get savedCapability => _storage.read(key: _capabilityKey);

  Future<Availability> availability(String date) async {
    final response = await _send(
      'GET',
      '/v1/availability?date=${Uri.encodeQueryComponent(date)}',
    );
    return Availability.fromJson(response);
  }

  Future<PrivateReservation> createHold(String startsAt) async {
    final response = await _send(
      'POST',
      '/v1/reservations',
      body: {'starts_at': startsAt},
      unknownOnFailure: true,
    );
    if (response is! Map<String, Object?> ||
        response['capability'] is! String ||
        (response['capability'] as String).length != 64 ||
        response['reservation'] is! Map<String, Object?>) {
      throw const FormatException('Reserva não reconhecida.');
    }
    final reservation = Reservation.fromJson(response['reservation']);
    final capability = response['capability'] as String;
    await _storage.write(key: _reservationKey, value: reservation.id);
    await _storage.write(key: _capabilityKey, value: capability);
    return PrivateReservation(reservation: reservation, capability: capability);
  }

  Future<Reservation?> restore() async {
    final id = await _storage.read(key: _reservationKey);
    final capability = await _storage.read(key: _capabilityKey);
    if (id == null || capability == null) {
      return null;
    }
    return reservation(id, capability);
  }

  Future<Reservation> reservation(String id, String capability) async {
    final response = await _send(
      'GET',
      '/v1/reservations/${Uri.encodeComponent(id)}',
      token: capability,
    );
    return Reservation.fromJson(response);
  }

  Future<Reservation> cancel(String id, String capability) async {
    final response = await _send(
      'POST',
      '/v1/reservations/${Uri.encodeComponent(id)}/cancel',
      token: capability,
    );
    return Reservation.fromJson(response);
  }

  Future<Object?> _send(
    String method,
    String path, {
    String? token,
    Map<String, Object?>? body,
    bool unknownOnFailure = false,
  }) async {
    final uri = Uri.parse(_baseUrl).resolve(path);
    final headers = <String, String>{'Accept': 'application/json'};
    if (body != null) {
      headers['Content-Type'] = 'application/json';
    }
    if (token != null) {
      headers['Authorization'] = 'Bearer $token';
    }
    late http.Response response;
    try {
      final request = http.Request(method, uri)..headers.addAll(headers);
      if (body != null) {
        request.body = jsonEncode(body);
      }
      response = await http.Response.fromStream(
        await _client.send(request).timeout(requestTimeout),
      );
    } catch (error) {
      if (method == 'POST' && path == '/v1/reservations' && unknownOnFailure) {
        throw UnknownReservationOutcome(error.toString());
      }
      throw Exception('Não foi possível conectar ao backend local. $error');
    }
    Object? decoded;
    if (response.body.isNotEmpty) {
      try {
        decoded = jsonDecode(response.body);
      } on FormatException {
        throw const FormatException('O backend retornou JSON inválido.');
      }
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      if (decoded is Map<String, Object?> && decoded['message'] is String) {
        throw ReservationsApiException(
          response.statusCode,
          decoded['message'] as String,
        );
      }
      throw ReservationsApiException(
        response.statusCode,
        'Não foi possível concluir a solicitação.',
      );
    }
    return decoded;
  }

  void close() => _client.close();
}

class ReservationsApiException implements Exception {
  const ReservationsApiException(this.statusCode, this.message);
  final int statusCode;
  final String message;
  @override
  String toString() => message;
}

class UnknownReservationOutcome implements Exception {
  const UnknownReservationOutcome(this.details);
  final String details;
  @override
  String toString() =>
      'Resultado incerto; atualize antes de tentar reservar novamente. $details';
}
