"""Real HTTP BFF checks; not a JavaScript/browser or device E2E substitute."""

import http.cookies
import os
import re
import urllib.error
import urllib.parse
import urllib.request
import time

from smoke import Client, NoRedirect, SmokeFailure, local_url, require


def web_smoke(web_base, api_base, simulator_base):
    base = local_url(web_base)
    api, simulator = Client(api_base), Client(simulator_base)
    operator = "Bearer " + os.environ["DEMO_OPERATOR_TOKEN"]
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def page(path, method="GET", form=None, cookie=None, expected=200):
        headers = {"Accept": "text/html"}
        data = None
        if form is not None:
            data = urllib.parse.urlencode(form).encode()
            headers.update(
                {"Content-Type": "application/x-www-form-urlencoded", "Origin": base}
            )
        if cookie:
            headers["Cookie"] = cookie
        req = urllib.request.Request(
            base + path, headers=headers, data=data, method=method
        )
        try:
            response = opener.open(req, timeout=15)
        except urllib.error.HTTPError as failure:
            response = failure
        except (urllib.error.URLError, TimeoutError, OSError) as failure:
            raise SmokeFailure(
                "BFF HTTP local indisponível; POST pode ter resultado incerto."
            ) from failure
        with response:
            status, headers = response.code, response.headers
            payload = response.read(1048577)
        require(
            status == expected, "Status inesperado no BFF; corpo e cookies omitidos."
        )
        require(len(payload) <= 1048576, "HTML excede o limite do smoke.")
        require(
            headers.get("Cache-Control") == "no-store",
            "BFF deixou resposta de pedido cacheável.",
        )
        require(
            headers.get("Referrer-Policy") == "no-referrer",
            "BFF não restringiu o referrer da página de pedido.",
        )
        return payload.decode(), headers

    page("/")
    html, headers = page("/?/reserve", method="POST", form={"offer_id": "demo-offer"})
    cookies = http.cookies.SimpleCookie()
    for header in headers.get_all("Set-Cookie", []):
        cookies.load(header)
    owned = [
        (name, value)
        for name, value in cookies.items()
        if name.startswith("order_access_")
    ]
    require(len(owned) == 1, "BFF não criou uma única credencial de pedido em cookie.")
    name, grant = owned[0]
    order_id = name.removeprefix("order_access_")
    require(
        re.fullmatch(r"[a-f0-9]{32}", order_id) is not None,
        "Cookie não referencia pedido válido.",
    )
    require(
        grant["httponly"]
        and grant["samesite"].lower() == "strict"
        and grant["expires"],
        "Cookie sem HttpOnly/SameSite/prazo.",
    )
    require(
        grant["path"] == "/orders/" + order_id,
        "Cookie não está restrito ao caminho de pedido.",
    )
    require(
        "Reserva criada" in html
        and grant.value not in html
        and "order_access_token" not in html,
        "Credencial vazou no HTML da reserva ou reserva não apareceu.",
    )
    cookie = name + "=" + grant.value
    path = "/orders/" + order_id
    page(path, expected=401)
    html, _ = page(path, cookie=cookie)
    require(
        "Aguardando pagamento" in html and grant.value not in html,
        "Consulta web não está pendente ou expôs credencial.",
    )
    sessions = []
    for _ in range(2):
        html, _ = page(path + "?/checkout", method="POST", form={}, cookie=cookie)
        match = re.search(r"Sessão local ([a-f0-9-]{36})", html)
        require(
            match is not None
            and grant.value not in html
            and "access_token" not in html,
            "Sessão não abriu ou credencial vazou no HTML.",
        )
        sessions.append(match.group(1))
    require(sessions[0] == sessions[1], "BFF duplicou sessão ao repetir a mesma ação.")
    simulator.request("POST", "/v1/charges/" + order_id + "/pay")
    deadline = time.monotonic() + 10
    auth = "Bearer " + grant.value
    while time.monotonic() < deadline:
        if (
            api.request("GET", "/v1/customer/orders/" + order_id, token=auth).get(
                "state"
            )
            == "paid"
        ):
            break
        time.sleep(0.1)
    else:
        raise SmokeFailure("Worker não confirmou o pedido web no prazo.")
    html, _ = page(path, cookie=cookie)
    require(
        "Pagamento registrado" in html and grant.value not in html,
        "BFF não refletiu pagamento autoritativo ou expôs credencial.",
    )
    operator_path = "/v1/operator/orders/" + order_id
    pickup = api.request(
        "POST", operator_path + "/pickup-token", expected=201, token=operator
    )
    api.request("POST", operator_path + "/pickup", body=pickup, token=operator)
    require(
        api.request(
            "POST", operator_path + "/pickup", expected=409, body=pickup, token=operator
        ).get("error_code")
        == "PICKUP_ALREADY_DONE",
        "Retirada web duplicada não foi recusada.",
    )
    print(
        "PASS web HTTP: reserva/BFF/cookie privado, ID sem acesso, sessão idempotente, worker/status e retirada única. Não é E2E de browser."
    )
