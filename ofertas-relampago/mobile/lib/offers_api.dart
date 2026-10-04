import 'dart:convert';

import 'package:http/http.dart' as http;

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

  factory Offer.fromJson(Map<String, dynamic> json) => Offer(
    id: json['id'] as String,
    title: json['title'] as String,
    priceCents: json['price_cents'] as int,
    availableUnits: json['available_units'] as int,
    reservationTtlSeconds: json['reservation_ttl_seconds'] as int,
  );
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

  factory DemoOrder.fromJson(Map<String, dynamic> json) => DemoOrder(
    id: json['id'] as String,
    amountCents: json['amount_cents'] as int,
    state: json['state'] as String,
  );
}

class OffersApiException implements Exception {
  const OffersApiException(this.message, this.errorCode);

  final String message;
  final String errorCode;

  @override
  String toString() => '$errorCode: $message';
}

class OffersApi {
  OffersApi({required String baseUrl, http.Client? client})
    : _baseUri = Uri.parse(baseUrl),
      _client = client ?? http.Client();

  final Uri _baseUri;
  final http.Client _client;

  factory OffersApi.fromEnvironment() => OffersApi(
    baseUrl: const String.fromEnvironment(
      'API_BASE_URL',
      defaultValue: 'http://127.0.0.1:8080',
    ),
  );

  Future<List<Offer>> listOffers() async {
    final response = await _client
        .get(_baseUri.resolve('/v1/offers'))
        .timeout(const Duration(seconds: 8));
    final body = _decodeBody(response);
    if (response.statusCode != 200) _throwApiError(body);
    if (body is! List) {
      throw const FormatException('Resposta de ofertas inválida.');
    }
    return body
        .map((item) => Offer.fromJson(item as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<DemoOrder> createOrder(String offerId) async {
    final response = await _client
        .post(
          _baseUri.resolve('/v1/orders'),
          headers: const {'content-type': 'application/json'},
          body: jsonEncode({'offer_id': offerId}),
        )
        .timeout(const Duration(seconds: 8));
    final body = _decodeBody(response);
    if (response.statusCode != 201) _throwApiError(body);
    return DemoOrder.fromJson(body as Map<String, dynamic>);
  }

  Future<DemoOrder> getOrder(String orderId) async {
    final response = await _client
        .get(_baseUri.resolve('/v1/orders/$orderId'))
        .timeout(const Duration(seconds: 8));
    final body = _decodeBody(response);
    if (response.statusCode != 200) _throwApiError(body);
    return DemoOrder.fromJson(body as Map<String, dynamic>);
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
        body['message'] as String? ??
            'Não foi possível concluir a solicitação.',
        body['error_code'] as String? ?? 'UNEXPECTED_ERROR',
      );
    }
    throw const OffersApiException(
      'Não foi possível concluir a solicitação.',
      'UNEXPECTED_ERROR',
    );
  }
}
