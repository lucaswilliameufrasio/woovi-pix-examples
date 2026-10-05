#!/usr/bin/env python3
"""Opt-in, loopback-only smoke checks; never call a PSP or reset a database."""

import argparse
import ipaddress
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


class SmokeFailure(Exception):
    """A sanitized failure safe to display without response bodies or tokens."""


def require(condition, message):
    if not condition:
        raise SmokeFailure(message)


def local_url(value):
    parsed = urllib.parse.urlsplit(value)
    try:
        loopback = ipaddress.ip_address(parsed.hostname or "").is_loopback
        valid_port = parsed.port is not None
    except ValueError:
        loopback, valid_port = False, False
    require(
        parsed.scheme == "http"
        and loopback
        and valid_port
        and not parsed.username
        and not parsed.password
        and parsed.path in ("", "/")
        and not parsed.query
        and not parsed.fragment,
        "Use uma base HTTP com IP loopback literal e porta, sem credencial/path/query.",
    )
    return value.rstrip("/")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise SmokeFailure("Redirect recusado: o smoke deve permanecer em loopback.")


class Client:
    def __init__(self, base):
        self.base = local_url(base)
        self.opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}), NoRedirect()
        )

    def request(
        self, method, path, expected=200, body=None, token=None, idempotency_key=None
    ):
        require(path.startswith("/") and not path.startswith("//"), "Path inválido.")
        headers = {"Accept": "application/json"}
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key
        data = None
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        if token:
            headers["Authorization"] = token
        request = urllib.request.Request(
            self.base + path, data=data, headers=headers, method=method
        )
        try:
            response = self.opener.open(request, timeout=8)
        except urllib.error.HTTPError as failure:
            response = failure
        except (urllib.error.URLError, TimeoutError, OSError) as failure:
            raise SmokeFailure(
                "HTTP local indisponível ou timeout; resultado de POST pode ser incerto."
            ) from failure
        with response:
            status = response.code
            payload = response.read(65537)
        require(
            status == expected,
            f"HTTP {status}; esperado {expected} em {method} {path}.",
        )
        require(len(payload) <= 65536, "Resposta excedeu o limite do smoke.")
        try:
            return json.loads(payload)
        except (ValueError, UnicodeDecodeError) as failure:
            raise SmokeFailure("Resposta local não é JSON válido.") from failure


def offers_smoke(api_base, simulator_base):
    token = os.environ.get("DEMO_OPERATOR_TOKEN", "")
    require(
        len(token) >= 32,
        "Configure DEMO_OPERATOR_TOKEN do processo da API, sem imprimir seu valor.",
    )
    api, simulator = Client(api_base), Client(simulator_base)
    operator = "Bearer " + token
    offers = api.request("GET", "/v1/offers")
    require(isinstance(offers, list), "Catálogo inválido.")
    offer = next((item for item in offers if item.get("id") == "demo-offer"), {})
    require(
        offer.get("available_units") == 1,
        "A seed precisa de uma unidade livre. Use banco isolado; o smoke não executa reset.",
    )
    denied = api.request("GET", "/v1/operator/orders", expected=401)
    require(
        denied.get("error_code") == "OPERATOR_UNAUTHORIZED",
        "Operador sem token não foi recusado corretamente.",
    )
    api.request("GET", "/v1/operator/orders", token=operator)
    order = api.request(
        "POST", "/v1/orders", expected=201, body={"offer_id": "demo-offer"}
    )
    order_id = order.get("id")
    require(isinstance(order_id, str) and bool(order_id), "Pedido sem ID.")
    require(
        order.get("amount_cents") == offer.get("price_cents"),
        "Preço diverge do catálogo autoritativo.",
    )
    require(order.get("state") == "pending_payment", "Estado inicial inesperado.")
    path = "/v1/orders/" + urllib.parse.quote(order_id, safe="")
    operator_path = "/v1/operator/orders/" + urllib.parse.quote(order_id, safe="")
    order_token = order.get("order_access_token")
    require(
        isinstance(order_token, str) and len(order_token) == 64,
        "Reserva sem credencial própria.",
    )
    customer_auth = "Bearer " + order_token
    denied = api.request("GET", path, expected=401)
    require(
        denied.get("error_code") == "ORDER_UNAUTHORIZED",
        "Alias legado permitiu consulta pública por ID.",
    )
    require(
        api.request("GET", path, token=customer_auth).get("id") == order_id,
        "Alias autenticado não consultou o pedido.",
    )
    customer_path = "/v1/customer/orders/" + urllib.parse.quote(order_id, safe="")
    denied = api.request("GET", customer_path, expected=401)
    require(
        denied.get("error_code") == "ORDER_UNAUTHORIZED",
        "ID de pedido autorizou consulta.",
    )
    require(
        api.request("GET", customer_path, token=customer_auth).get("id") == order_id,
        "Credencial não consulta o próprio pedido.",
    )
    denied = api.request(
        "POST", "/v1/orders", expected=412, body={"offer_id": "demo-offer"}
    )
    require(
        denied.get("error_code") == "OFFER_UNAVAILABLE",
        "Última unidade não foi protegida.",
    )
    denied = api.request(
        "POST", operator_path + "/pickup-token", expected=412, token=operator
    )
    require(
        denied.get("error_code") == "PICKUP_NOT_ELIGIBLE",
        "Retirada pendente não foi recusada.",
    )
    checkout_key = "smoke-checkout-" + uuid.uuid4().hex
    grant = api.request(
        "POST",
        customer_path + "/checkout",
        expected=201,
        token=customer_auth,
        idempotency_key=checkout_key,
    )
    require(
        grant.get("order_id") == order_id
        and grant.get("amount_cents") == order["amount_cents"]
        and grant.get("mode") == "local_simulation"
        and "pix_copy_paste" not in grant,
        "Checkout local sem vínculo/identificação correta ou com Pix indevido.",
    )
    checkout_id, access_token = grant.get("checkout_id"), grant.get("access_token")
    require(
        isinstance(checkout_id, str)
        and isinstance(access_token, str)
        and len(access_token) == 64,
        "Sessão sem credencial restrita.",
    )
    checkout_path = "/v1/checkout-sessions/" + urllib.parse.quote(checkout_id, safe="")
    denied = api.request("GET", checkout_path, expected=401)
    require(
        denied.get("error_code") == "CHECKOUT_UNAUTHORIZED",
        "Sessão permitiu consulta anônima.",
    )
    session_auth = "Bearer " + access_token
    require(
        api.request("GET", checkout_path, token=session_auth).get("status")
        == "pending",
        "Estado inicial da sessão diverge.",
    )
    replay_grant = api.request(
        "POST",
        customer_path + "/checkout",
        token=customer_auth,
        idempotency_key=checkout_key,
    )
    require(
        replay_grant.get("checkout_id") == checkout_id
        and replay_grant.get("charge_id") == grant.get("charge_id"),
        "Replay materializou outra sessão/cobrança.",
    )
    denied = api.request(
        "POST", operator_path + "/pickup-token", expected=401, token=session_auth
    )
    require(
        denied.get("error_code") == "OPERATOR_UNAUTHORIZED",
        "Credencial de leitura autorizou operação.",
    )
    charge = simulator.request("POST", "/v1/charges", body={"order_id": order_id})
    replay = simulator.request("POST", "/v1/charges", body={"order_id": order_id})
    require(
        charge.get("id") == replay.get("id")
        and charge.get("amount_cents") == order["amount_cents"],
        "Cobrança não é idempotente/autoritativa.",
    )
    charge_path = "/v1/charges/" + urllib.parse.quote(order_id, safe="")
    simulator.request("POST", charge_path + "/pay")
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        current = api.request("GET", customer_path, token=customer_auth)
        if current.get("state") == "paid":
            break
        time.sleep(0.1)
    else:
        raise SmokeFailure(
            "Worker não confirmou o pedido no prazo; consulte logs sanitizados."
        )
    paid_charge = simulator.request("GET", charge_path)
    require(
        paid_charge.get("status") == "paid"
        and paid_charge.get("order_state") == "paid",
        "Estado financeiro/operacional divergente.",
    )
    require(
        api.request("GET", checkout_path, token=session_auth).get("status") == "paid",
        "Sessão não refletiu pagamento confirmado pelo worker.",
    )
    duplicate = simulator.request("POST", charge_path + "/pay", expected=409)
    require(
        duplicate.get("error_code") == "ORDER_NOT_ELIGIBLE",
        "Pagamento terminal repetido não foi recusado.",
    )
    issued = api.request(
        "POST", operator_path + "/pickup-token", expected=201, token=operator
    )
    rotated = api.request(
        "POST", operator_path + "/pickup-token", expected=201, token=operator
    )
    require(
        bool(issued.get("pickup_token")) and bool(rotated.get("pickup_token")),
        "Token de retirada ausente.",
    )
    invalid = api.request(
        "POST", operator_path + "/pickup", expected=409, body=issued, token=operator
    )
    require(
        invalid.get("error_code") == "PICKUP_TOKEN_INVALID",
        "Rotação não invalidou token anterior.",
    )
    picked = api.request(
        "POST", operator_path + "/pickup", body=rotated, token=operator
    )
    require(bool(picked.get("picked_up_at")), "Retirada não persistiu timestamp.")
    duplicate = api.request(
        "POST", operator_path + "/pickup", expected=409, body=rotated, token=operator
    )
    require(
        duplicate.get("error_code") == "PICKUP_ALREADY_DONE",
        "Retirada duplicada não foi recusada.",
    )
    require(
        api.request("GET", customer_path, token=customer_auth).get("picked_up_at")
        == picked["picked_up_at"],
        "Retirada não foi persistida.",
    )
    print(
        "PASS offers: reserva/estoque, sessão restrita/idempotente, webhook/worker e retirada única."
    )


def woovi_simulator_smoke(base):
    client = Client(base)
    reference = "examples-smoke-" + uuid.uuid4().hex
    created = client.request(
        "POST",
        "/api/v1/charge",
        token="simulator",
        body={"correlationID": reference, "value": 1250, "expiresIn": 300},
    )
    charge = created.get("charge", {})
    identifier = charge.get("identifier")
    require(
        isinstance(identifier, str) and bool(identifier), "Simulador sem identifier."
    )
    fetched = client.request(
        "GET",
        "/api/v1/charge/" + urllib.parse.quote(identifier, safe=""),
        token="simulator",
    )
    require(
        fetched.get("charge", {}).get("identifier") == identifier,
        "Consulta não recuperou cobrança criada.",
    )
    require(
        charge.get("correlationID") == reference and charge.get("value") == 1250,
        "Contrato create diverge.",
    )
    require(
        fetched.get("charge", {}).get("value") == 1250
        and fetched.get("charge", {}).get("status") == "ACTIVE",
        "Contrato get diverge.",
    )
    print(
        "PASS woovi-simulator: create/get local; não valida pagamento, pedidos das demos ou sandbox."
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=["offers", "woovi-simulator"])
    parser.add_argument(
        "--ack-local-write",
        action="store_true",
        help="Autoriza criar dados sintéticos e consumir uma unidade na instância local escolhida.",
    )
    args = parser.parse_args()
    try:
        require(
            args.ack_local_write,
            "Passe --ack-local-write; use instância/banco isolados para smoke.",
        )
        if args.mode == "offers":
            offers_smoke(
                os.environ.get("API_BASE_URL", "http://127.0.0.1:8080"),
                os.environ.get("SIMULATOR_BASE_URL", "http://127.0.0.1:8081"),
            )
        else:
            woovi_simulator_smoke(
                os.environ.get("WOOVI_SIMULATOR_BASE_URL", "http://127.0.0.1:8081")
            )
    except SmokeFailure as failure:
        print("FAIL:", str(failure))
        return 1
    except (ValueError, KeyError, TypeError, AttributeError):
        # Do not echo malformed bodies, credentials, or backend exceptions.
        print(
            "FAIL: smoke não concluído. Verifique setup, seed e estados no guia local; não repita POST incerto nem resete dados para contornar erro."
        )
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
