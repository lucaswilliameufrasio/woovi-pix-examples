import 'dart:convert';

import 'package:http/http.dart' as http;

import 'order_access.dart';

class Offer {
  const Offer({
    required this.id,
    required this.title,
    required this.priceCents,
    required this.availableUnits,
    required this.reservationTtlSeconds,
  });

  final String id;
  final String title;
  final int priceCents;
  final int availableUnits;
  final int reservationTtlSeconds;

  factory Offer.fromJson(Map<String, dynamic> json) {
    if (json['id'] is! String ||
        json['id'].isEmpty ||
        json['title'] is! String ||
        json['title'].trim().isEmpty ||
        json['price_cents'] is! int ||
        json['price_cents'] <= 0 ||
        json['available_units'] is! int ||
        json['available_units'] < 0 ||
        json['reservation_ttl_seconds'] is! int ||
        json['reservation_ttl_seconds'] <= 0) {
      throw const FormatException('Oferta inválida.');
    }
    return Offer(
      id: json['id'] as String,
      title: json['title'] as String,
      priceCents: json['price_cents'] as int,
      availableUnits: json['available_units'] as int,
      reservationTtlSeconds: json['reservation_ttl_seconds'] as int,
    );
  }
}

class DemoOrder {
  const DemoOrder({
    required this.id,
    required this.amountCents,
    required this.state,
  });

  final String id;
  final int amountCents;
  final String state;

  factory DemoOrder.fromJson(Map<String, dynamic> json) {
    if (json['id'] is! String ||
        !RegExp(r'^[A-Za-z0-9_-]{1,200}$').hasMatch(json['id']) ||
        json['amount_cents'] is! int ||
        json['amount_cents'] <= 0 ||
        ![
          'pending_payment',
          'paid',
          'expired',
          'payment_exception',
        ].contains(json['state'])) {
      throw const FormatException('Pedido inválido.');
    }
    return DemoOrder(
      id: json['id'] as String,
      amountCents: json['amount_cents'] as int,
      state: json['state'] as String,
    );
  }
}

class OffersApiException implements Exception {
  const OffersApiException(this.message, this.errorCode);

  final String message;
  final String errorCode;

  @override
  String toString() => '$errorCode: $message';
}

class OffersApi {
  OffersApi({
    required String baseUrl,
    http.Client? client,
    OrderAccessStore? accessStore,
    DateTime Function()? now,
  }) : _baseUri = _localBase(baseUrl),
       _client = client ?? http.Client(),
       _accessStore = accessStore ?? const SecureOrderAccessStore(),
       _now = now ?? DateTime.now;

  static Uri _localBase(String baseUrl) {
    final uri = Uri.parse(baseUrl);
    if (uri.scheme != 'http' ||
        !['127.0.0.1', '::1', '10.0.2.2'].contains(uri.host) ||
        !uri.hasPort ||
        uri.userInfo.isNotEmpty ||
        uri.hasQuery ||
        uri.hasFragment ||
        (uri.path.isNotEmpty && uri.path != '/')) {
      throw ArgumentError(
        'A demo exige origem HTTP local com porta explícita.',
      );
    }
    return uri;
  }

  final Uri _baseUri;
  final http.Client _client;
  final OrderAccessStore _accessStore;
  final DateTime Function() _now;
  final Map<String, OrderAccess> _access = {};
  final Set<String> _unpersisted = {};
  final Set<String> _revoked = {};

  bool accessWasPersisted(String id) => !_unpersisted.contains(id);

  Future<http.Response> _request(
    String method,
    String path, {
    Map<String, String>? headers,
    String? body,
  }) async {
    final request = http.Request(method, _baseUri.resolve(path))
      ..followRedirects = false;
    if (headers != null) request.headers.addAll(headers);
    if (body != null) request.body = body;
    return await (() async {
      final response = await http.Response.fromStream(
        await _client.send(request),
      );
      if (response.isRedirect ||
          (response.statusCode >= 300 && response.statusCode < 400)) {
        throw const OffersApiException(
          'Redirecionamento recusado.',
          'DEPENDENCY_INVALID_RESPONSE',
        );
      }
      return response;
    })().timeout(const Duration(seconds: 8));
  }

  void close() => _client.close();

  factory OffersApi.fromEnvironment() => OffersApi(
    baseUrl: const String.fromEnvironment(
      'API_BASE_URL',
      defaultValue: 'http://127.0.0.1:8080',
    ),
  );

  Future<List<Offer>> listOffers() async {
    final response = await _request('GET', '/v1/offers');
    final body = _decodeBody(response);
    if (response.statusCode != 200) _throwApiError(body);
    if (body is! List) {
      throw const FormatException('Resposta de ofertas inválida.');
    }
    return body
        .map((item) {
          if (item is! Map<String, dynamic>) {
            throw const FormatException('Oferta inválida.');
          }
          return Offer.fromJson(item);
        })
        .toList(growable: false);
  }

  Future<DemoOrder> createOrder(String offerId) async {
    final response = await _request(
      'POST',
      '/v1/orders',
      headers: const {'content-type': 'application/json'},
      body: jsonEncode({'offer_id': offerId}),
    );
    final body = _decodeBody(response);
    if (response.statusCode != 201) _throwApiError(body);
    if (body is! Map<String, dynamic>) {
      throw const FormatException('Reserva inválida; resultado incerto.');
    }
    final order = DemoOrder.fromJson(body);
    final access = OrderAccess.parse({
      'token': body['order_access_token'],
      'expires_at': body['order_token_expires_at'],
    });
    final now = _now();
    if (!access.expiresAt.isAfter(now) ||
        access.expiresAt.isAfter(now.add(const Duration(minutes: 21)))) {
      throw const FormatException(
        'Prazo de acesso inválido; resultado da reserva incerto.',
      );
    }
    _access[order.id] = access;
    _revoked.remove(order.id);
    try {
      await _accessStore
          .write(_baseUri.origin, order.id, access)
          .timeout(const Duration(seconds: 8));
    } catch (_) {
      _unpersisted.add(order.id);
    }
    return order;
  }

  Future<DemoOrder> getOrder(String orderId) async {
    if (!RegExp(r'^[A-Za-z0-9_-]{1,200}$').hasMatch(orderId) ||
        _revoked.contains(orderId)) {
      throw const OffersApiException(
        'Acesso ao pedido indisponível ou expirado.',
        'ORDER_UNAUTHORIZED',
      );
    }
    OrderAccess? access = _access[orderId];
    if (access == null) {
      try {
        access = await _accessStore
            .read(_baseUri.origin, orderId)
            .timeout(const Duration(seconds: 8));
      } catch (_) {
        throw const OffersApiException(
          'Não foi possível ler o acesso seguro deste pedido.',
          'ORDER_ACCESS_UNAVAILABLE',
        );
      }
    }
    if (access == null || !access.expiresAt.isAfter(_now())) {
      _revoked.add(orderId);
      throw const OffersApiException(
        'Acesso ao pedido ausente ou expirado. O ID não autoriza consulta.',
        'ORDER_UNAUTHORIZED',
      );
    }
    final response = await _request(
      'GET',
      '/v1/customer/orders/${Uri.encodeComponent(orderId)}',
      headers: {'authorization': 'Bearer ${access.token}'},
    );
    if (response.statusCode == 401) {
      _revoked.add(orderId);
      _access.remove(orderId);
      throw const OffersApiException(
        'A credencial do pedido expirou ou foi revogada. Procure o operador local.',
        'ORDER_UNAUTHORIZED',
      );
    }
    final body = _decodeBody(response);
    if (response.statusCode != 200) _throwApiError(body);
    if (body is! Map<String, dynamic>) {
      throw const FormatException('Pedido inválido.');
    }
    final order = DemoOrder.fromJson(body);
    if (order.id != orderId) {
      throw const FormatException('A API retornou outro pedido.');
    }
    return order;
  }

  dynamic _decodeBody(http.Response response) {
    try {
      return jsonDecode(utf8.decode(response.bodyBytes));
    } on FormatException {
      throw const FormatException('Resposta inválida da API local.');
    }
  }

  Never _throwApiError(dynamic body) {
    if (body is Map<String, dynamic>) {
      throw OffersApiException(
        body['message'] is String
            ? body['message']
            : 'Não foi possível concluir a solicitação.',
        body['error_code'] is String ? body['error_code'] : 'UNEXPECTED_ERROR',
      );
    }
    throw const OffersApiException(
      'Não foi possível concluir a solicitação.',
      'UNEXPECTED_ERROR',
    );
  }
}
