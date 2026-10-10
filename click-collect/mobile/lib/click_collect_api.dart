import 'dart:async';
import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;

class Product {
  const Product({
    required this.id,
    required this.title,
    required this.description,
    required this.priceCents,
    required this.availableUnits,
  });

  final String id;
  final String title;
  final String description;
  final int priceCents;
  final int availableUnits;

  factory Product.fromJson(Map<String, dynamic> json) {
    if (json['id'] is! String ||
        json['id'].isEmpty ||
        json['title'] is! String ||
        json['title'].trim().isEmpty ||
        json['description'] is! String ||
        json['price_cents'] is! int ||
        json['price_cents'] <= 0 ||
        json['available_units'] is! int ||
        json['available_units'] < 0) {
      throw const FormatException('Produto inválido.');
    }
    return Product(
      id: json['id'] as String,
      title: json['title'] as String,
      description: json['description'] as String,
      priceCents: json['price_cents'] as int,
      availableUnits: json['available_units'] as int,
    );
  }
}

enum PaymentState { pending, paid, expired, cancelled, paymentException }

enum FulfillmentState { awaitingPayment, preparing, readyForPickup, pickedUp }

class DemoOrder {
  const DemoOrder({
    required this.id,
    required this.productId,
    required this.productTitle,
    required this.amountCents,
    required this.paymentState,
    required this.fulfillmentState,
    required this.expiresAt,
    required this.createdAt,
    this.pickedUpAt,
  });

  final String id;
  final String productId;
  final String productTitle;
  final int amountCents;
  final PaymentState paymentState;
  final FulfillmentState fulfillmentState;
  final DateTime expiresAt;
  final DateTime createdAt;
  final DateTime? pickedUpAt;

  factory DemoOrder.fromJson(Map<String, dynamic> json) {
    if (json['id'] is! String ||
        !RegExp(r'^[a-f0-9]{32}$').hasMatch(json['id']) ||
        json['product_id'] is! String ||
        json['product_title'] is! String ||
        json['amount_cents'] is! int ||
        json['amount_cents'] <= 0 ||
        json['payment_state'] is! String ||
        json['fulfillment_state'] is! String ||
        json['expires_at'] is! String ||
        json['created_at'] is! String) {
      throw const FormatException('Pedido inválido.');
    }
    final expiresAt = DateTime.tryParse(json['expires_at'] as String);
    final createdAt = DateTime.tryParse(json['created_at'] as String);
    final paymentState = switch (json['payment_state']) {
      'pending' => PaymentState.pending,
      'paid' => PaymentState.paid,
      'expired' => PaymentState.expired,
      'cancelled' => PaymentState.cancelled,
      'payment_exception' => PaymentState.paymentException,
      _ => null,
    };
    final fulfillmentState = switch (json['fulfillment_state']) {
      'awaiting_payment' => FulfillmentState.awaitingPayment,
      'preparing' => FulfillmentState.preparing,
      'ready_for_pickup' => FulfillmentState.readyForPickup,
      'picked_up' => FulfillmentState.pickedUp,
      _ => null,
    };
    if (expiresAt == null ||
        createdAt == null ||
        paymentState == null ||
        fulfillmentState == null) {
      throw const FormatException('Estado ou data do pedido inválido.');
    }
    DateTime? pickedUpAt;
    if (json.containsKey('picked_up_at')) {
      if (json['picked_up_at'] is! String) {
        throw const FormatException('Data de retirada inválida.');
      }
      pickedUpAt = DateTime.tryParse(json['picked_up_at'] as String);
      if (pickedUpAt == null) {
        throw const FormatException('Data de retirada inválida.');
      }
    }
    return DemoOrder(
      id: json['id'] as String,
      productId: json['product_id'] as String,
      productTitle: json['product_title'] as String,
      amountCents: json['amount_cents'] as int,
      paymentState: paymentState,
      fulfillmentState: fulfillmentState,
      expiresAt: expiresAt,
      createdAt: createdAt,
      pickedUpAt: pickedUpAt,
    );
  }
}

class ReservedOrder {
  const ReservedOrder({
    required this.order,
    required this.pickupCode,
    required this.accessToken,
  });

  final DemoOrder order;
  final String pickupCode;
  final String accessToken;

  factory ReservedOrder.fromJson(Map<String, dynamic> json) {
    if (json['order_access_token'] is! String ||
        !RegExp(r'^[a-f0-9]{64}$').hasMatch(json['order_access_token']) ||
        json['pickup_code'] is! String ||
        !RegExp(r'^[A-F0-9]{8}$').hasMatch(json['pickup_code'])) {
      throw const FormatException('Credenciais do pedido inválidas.');
    }
    return ReservedOrder(
      order: DemoOrder.fromJson(json),
      pickupCode: json['pickup_code'] as String,
      accessToken: json['order_access_token'] as String,
    );
  }
}

class StoredOrderAccess {
  const StoredOrderAccess({
    required this.orderId,
    required this.accessToken,
    required this.pickupCode,
  });

  final String orderId;
  final String accessToken;
  final String pickupCode;
}

abstract interface class CredentialStore {
  Future<Map<String, String>> readAll(String prefix);
  Future<void> write(String key, String value);
}

class SecureCredentialStore implements CredentialStore {
  const SecureCredentialStore();

  static const _storage = FlutterSecureStorage();

  @override
  Future<Map<String, String>> readAll(String prefix) async {
    final values = await _storage.readAll();
    return Map<String, String>.fromEntries(
      values.entries.where((entry) => entry.key.startsWith(prefix)),
    );
  }

  @override
  Future<void> write(String key, String value) =>
      _storage.write(key: key, value: value);
}

abstract interface class ClickCollectService {
  Future<List<Product>> listProducts();
  Future<List<ReservedOrder>> loadSavedOrders();
  Future<ReservedOrder> reserve(String productId);
  Future<DemoOrder> refresh(ReservedOrder order);
  Future<DemoOrder> simulatePayment(ReservedOrder order);
  Future<DemoOrder> cancel(ReservedOrder order);
  void close();
}

class ClickCollectApiException implements Exception {
  const ClickCollectApiException(this.message, this.errorCode);

  final String message;
  final String errorCode;

  @override
  String toString() => '$errorCode: $message';
}

class UnknownReservationOutcome implements Exception {
  const UnknownReservationOutcome();

  @override
  String toString() =>
      'O resultado da reserva é desconhecido. Não envie outra reserva às cegas.';
}

class ClickCollectApi implements ClickCollectService {
  ClickCollectApi({
    required String baseUrl,
    http.Client? client,
    CredentialStore? credentials,
  }) : _baseUri = _localBase(baseUrl),
       _client = client ?? http.Client(),
       _credentials = credentials ?? const SecureCredentialStore();

  final Uri _baseUri;
  final http.Client _client;
  final CredentialStore _credentials;
  String get _credentialPrefix =>
      'click_collect_${_baseUri.origin.replaceAll(RegExp(r'[^A-Za-z0-9]'), '_')}_';

  static Uri _localBase(String value) {
    final uri = Uri.parse(value);
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

  factory ClickCollectApi.fromEnvironment({
    http.Client? client,
    CredentialStore? credentials,
  }) => ClickCollectApi(
    baseUrl: const String.fromEnvironment(
      'API_BASE_URL',
      defaultValue: 'http://10.0.2.2:8082',
    ),
    client: client,
    credentials: credentials,
  );

  @override
  void close() => _client.close();

  Future<http.Response> _request(
    String method,
    String path, {
    Map<String, String> headers = const {},
    String? body,
  }) async {
    final request = http.Request(method, _baseUri.resolve(path))
      ..followRedirects = false
      ..headers.addAll({'Accept': 'application/json', ...headers});
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = body;
    }
    try {
      final streamed = await _client
          .send(request)
          .timeout(const Duration(seconds: 8));
      final response = await http.Response.fromStream(streamed);
      if (response.isRedirect ||
          (response.statusCode >= 300 && response.statusCode < 400)) {
        throw const ClickCollectApiException(
          'Redirecionamento recusado.',
          'DEPENDENCY_INVALID_RESPONSE',
        );
      }
      return response;
    } on TimeoutException {
      throw const ClickCollectApiException(
        'A solicitação excedeu o prazo. O resultado pode ser desconhecido.',
        'REQUEST_OUTCOME_UNKNOWN',
      );
    } on ClickCollectApiException {
      rethrow;
    } catch (_) {
      throw const ClickCollectApiException(
        'Não foi possível conectar ao backend local.',
        'DEPENDENCY_UNAVAILABLE',
      );
    }
  }

  Object? _decode(http.Response response) {
    if (response.bodyBytes.isEmpty) {
      return const <String, Object?>{};
    }
    try {
      return jsonDecode(utf8.decode(response.bodyBytes));
    } on FormatException {
      throw const ClickCollectApiException(
        'Resposta JSON inválida.',
        'DEPENDENCY_INVALID_RESPONSE',
      );
    }
  }

  void _checkError(http.Response response, Object? value) {
    if (response.statusCode >= 200 && response.statusCode < 300) {
      return;
    }
    if (value is Map<String, dynamic> &&
        value['message'] is String &&
        value['error_code'] is String) {
      throw ClickCollectApiException(
        value['message'] as String,
        value['error_code'] as String,
      );
    }
    throw const ClickCollectApiException(
      'Não foi possível concluir a solicitação.',
      'DEPENDENCY_INVALID_RESPONSE',
    );
  }

  @override
  Future<List<Product>> listProducts() async {
    final response = await _request('GET', '/v1/products');
    final value = _decode(response);
    _checkError(response, value);
    if (value is! List) {
      throw const FormatException('Catálogo inválido.');
    }
    return value
        .map((item) {
          if (item is! Map<String, dynamic>) {
            throw const FormatException('Produto inválido.');
          }
          return Product.fromJson(item);
        })
        .toList(growable: false);
  }

  @override
  Future<ReservedOrder> reserve(String productId) async {
    http.Response response;
    try {
      response = await _request(
        'POST',
        '/v1/orders',
        body: jsonEncode({'product_id': productId}),
      );
    } on ClickCollectApiException catch (error) {
      if (error.errorCode == 'REQUEST_OUTCOME_UNKNOWN' ||
          error.errorCode == 'DEPENDENCY_UNAVAILABLE') {
        throw const UnknownReservationOutcome();
      }
      rethrow;
    }
    final value = _decode(response);
    _checkError(response, value);
    if (value is! Map<String, dynamic>) {
      throw const FormatException('Reserva inválida.');
    }
    final reserved = ReservedOrder.fromJson(value);
    final access = value['order_access_token'] as String;
    final key = '$_credentialPrefix${reserved.order.id}';
    try {
      await _credentials.write(
        key,
        jsonEncode({
          'order_id': reserved.order.id,
          'access_token': access,
          'pickup_code': reserved.pickupCode,
        }),
      );
    } catch (_) {
      throw ClickCollectApiException(
        'A reserva foi criada, mas o acesso seguro não pôde ser salvo. Não reserve novamente; pedido ${reserved.order.id}.',
        'CREDENTIAL_STORAGE_FAILED',
      );
    }
    return ReservedOrder(
      order: reserved.order,
      pickupCode: reserved.pickupCode,
      accessToken: access,
    );
  }

  @override
  Future<List<ReservedOrder>> loadSavedOrders() async {
    final stored = await _credentials.readAll(_credentialPrefix);
    final result = <ReservedOrder>[];
    for (final value in stored.values) {
      final decoded = jsonDecode(value);
      if (decoded is! Map<String, dynamic> ||
          decoded['order_id'] is! String ||
          decoded['access_token'] is! String ||
          !RegExp(r'^[a-f0-9]{64}$').hasMatch(decoded['access_token']) ||
          decoded['pickup_code'] is! String ||
          !RegExp(r'^[A-F0-9]{8}$').hasMatch(decoded['pickup_code'])) {
        throw const FormatException('Acesso local salvo está inválido.');
      }
      final id = decoded['order_id'] as String;
      final access = decoded['access_token'] as String;
      final order = await _getOrder(id, access);
      result.add(
        ReservedOrder(
          order: order,
          pickupCode: decoded['pickup_code'] as String,
          accessToken: access,
        ),
      );
    }
    result.sort(
      (left, right) => right.order.createdAt.compareTo(left.order.createdAt),
    );
    return result;
  }

  Future<DemoOrder> _getOrder(String orderId, String accessToken) async {
    final response = await _request(
      'GET',
      '/v1/orders/$orderId',
      headers: {'Authorization': 'Bearer $accessToken'},
    );
    final value = _decode(response);
    _checkError(response, value);
    if (value is! Map<String, dynamic>) {
      throw const FormatException('Pedido inválido.');
    }
    return DemoOrder.fromJson(value);
  }

  @override
  Future<DemoOrder> refresh(ReservedOrder order) =>
      _getOrder(order.order.id, order.accessToken);

  Future<DemoOrder> _changeOrder(
    String method,
    String path,
    ReservedOrder order,
  ) async {
    final response = await _request(
      method,
      path,
      headers: {'Authorization': 'Bearer ${order.accessToken}'},
    );
    final value = _decode(response);
    _checkError(response, value);
    if (value is! Map<String, dynamic>) {
      throw const FormatException('Pedido inválido.');
    }
    return DemoOrder.fromJson(value);
  }

  @override
  Future<DemoOrder> simulatePayment(ReservedOrder order) async {
    final response = await _request(
      'POST',
      '/v1/orders/${order.order.id}/demo-payment',
      headers: {'Authorization': 'Bearer ${order.accessToken}'},
    );
    final value = _decode(response);
    _checkError(response, value);
    return refresh(order);
  }

  @override
  Future<DemoOrder> cancel(ReservedOrder order) =>
      _changeOrder('DELETE', '/v1/orders/${order.order.id}', order);
}
