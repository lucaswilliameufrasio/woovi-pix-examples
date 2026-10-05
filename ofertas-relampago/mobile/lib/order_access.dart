import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class OrderAccess {
  const OrderAccess(this.token, this.expiresAt);

  final String token;
  final DateTime expiresAt;

  factory OrderAccess.parse(Object? value) {
    if (value is! Map<String, dynamic> ||
        value['token'] is! String ||
        !RegExp(r'^[a-f0-9]{64}$').hasMatch(value['token']) ||
        value['expires_at'] is! String) {
      throw const FormatException('Credencial de pedido inválida.');
    }
    final expiry = DateTime.tryParse(value['expires_at']);
    if (expiry == null || !expiry.isUtc) {
      throw const FormatException('Prazo de credencial inválido.');
    }
    return OrderAccess(value['token'], expiry);
  }

  String encode() =>
      jsonEncode({'token': token, 'expires_at': expiresAt.toIso8601String()});
}

abstract interface class OrderAccessStore {
  Future<OrderAccess?> read(String origin, String orderId);
  Future<void> write(String origin, String orderId, OrderAccess access);
}

class SecureOrderAccessStore implements OrderAccessStore {
  const SecureOrderAccessStore();

  static const _storage = FlutterSecureStorage(
    iOptions: IOSOptions(
      accessibility: KeychainAccessibility.unlocked_this_device,
    ),
  );

  static String key(String origin, String orderId) =>
      'offers.access.${Uri.encodeComponent(origin)}.${Uri.encodeComponent(orderId)}';

  @override
  Future<OrderAccess?> read(String origin, String orderId) async {
    final value = await _storage.read(key: key(origin, orderId));
    return value == null ? null : OrderAccess.parse(jsonDecode(value));
  }

  @override
  Future<void> write(String origin, String orderId, OrderAccess access) =>
      _storage.write(key: key(origin, orderId), value: access.encode());
}
