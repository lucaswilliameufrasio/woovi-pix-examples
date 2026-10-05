#!/usr/bin/env python3
"""Run offers smoke against isolated schemas on the documented local database."""

import json
import argparse
import os
from pathlib import Path
import secrets
import signal
import subprocess
import tempfile
import time
import re
import uuid

from smoke import offers_smoke, SmokeFailure
from web_smoke import web_smoke


ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / "ofertas-relampago/backend"
DB_PROJECT = os.environ.get("SMOKE_COMPOSE_PROJECT", "woovi-offers")
DB_PORT = int(os.environ.get("SMOKE_DATABASE_PORT", "55440"))


def listening_addresses(text):
    api_addr, simulator_addr = None, None
    for line in text.splitlines():
        if "API listening" in line and "addr=" in line:
            api_addr = line.split("addr=", 1)[1].split()[0]
        if "simulator listening" in line and "addr=" in line:
            simulator_addr = line.split("addr=", 1)[1].split()[0]
    return api_addr, simulator_addr


def checked(command, cwd=BACKEND):
    result = subprocess.run(command, cwd=cwd, capture_output=True, check=False)
    if result.returncode:
        raise SmokeFailure(
            "Comando local falhou; confira mise/Go/Docker e PostgreSQL healthy. Saída omitida para evitar exposição."
        )
    return result.stdout.decode().strip()


def run_web_smoke(api_addr, simulator_addr, logs):
    log_path = logs / "web.log"
    env = {
        "PATH": os.environ["PATH"],
        "HOME": os.environ["HOME"],
        "NO_COLOR": "1",
        "API_BASE_URL": "http://" + api_addr,
    }
    with log_path.open("w") as output:
        process = subprocess.Popen(
            [
                "mise",
                "exec",
                "--",
                "npm",
                "run",
                "dev",
                "--",
                "--host",
                "127.0.0.1",
                "--port",
                "0",
                "--strictPort",
            ],
            cwd=ROOT / "ofertas-relampago/web",
            env=env,
            stdout=output,
            stderr=output,
            start_new_session=True,
        )
        try:
            deadline = time.monotonic() + 30
            base = None
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise SmokeFailure(
                        "Servidor web smoke encerrou; confira npm ci/mise. Log não exibido."
                    )
                match = re.search(
                    r"Local:\s+(http://127\.0\.0\.1:\d+)", log_path.read_text()
                )
                if match:
                    base = match.group(1)
                    break
                time.sleep(0.1)
            if base is None:
                raise SmokeFailure("Startup web não forneceu endereço local no prazo.")
            web_smoke(base, "http://" + api_addr, "http://" + simulator_addr)
        finally:
            # The group belongs only to this runner's npm/Vite subprocesses.
            try:
                os.killpg(process.pid, signal.SIGINT)
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
            except ProcessLookupError:
                pass


def one_run(binary, logs, web=False, mobile=False):
    schema = "smoke_" + uuid.uuid4().hex
    checked(
        [
            "docker",
            "compose",
            "-p",
            DB_PROJECT,
            "exec",
            "-T",
            "postgres",
            "psql",
            "-U",
            "offers",
            "-d",
            "offers",
            "-v",
            "ON_ERROR_STOP=1",
            "-c",
            "CREATE SCHEMA " + schema,
        ]
    )
    token = secrets.token_hex(32)
    env = dict(os.environ)
    env.update(
        {
            "DATABASE_URL": f"postgres://offers:offers-local-only@127.0.0.1:{DB_PORT}/offers?sslmode=disable&search_path="
            + schema,
            "DEMO_MODE": "true",
            "DEMO_OPERATOR_TOKEN": token,
            "API_ADDR": "127.0.0.1:0",
            "SIMULATOR_ADDR": "127.0.0.1:0",
        }
    )
    log_path = logs / (schema + ".log")
    with log_path.open("w") as output:
        process = subprocess.Popen(
            [str(binary)], cwd=BACKEND, env=env, stdout=output, stderr=output
        )
        previous_token = os.environ.get("DEMO_OPERATOR_TOKEN")
        try:
            deadline = time.monotonic() + 20
            api_addr, simulator_addr = None, None
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise SmokeFailure(
                        "Backend smoke encerrou antes de servir; log local não exibido."
                    )
                api_addr, simulator_addr = listening_addresses(log_path.read_text())
                if api_addr and simulator_addr:
                    break
                time.sleep(0.1)
            if not api_addr or not simulator_addr:
                raise SmokeFailure(
                    "Startup não forneceu endereços locais dentro do prazo."
                )
            os.environ["DEMO_OPERATOR_TOKEN"] = token
            if mobile:
                mobile_env = {
                    "PATH": os.environ["PATH"],
                    "HOME": os.environ["HOME"],
                    "MOBILE_SMOKE_API_BASE": "http://" + api_addr,
                }
                result = subprocess.run(
                    ["fvm", "flutter", "test", "test/backend_integration_test.dart"],
                    cwd=ROOT / "ofertas-relampago/mobile",
                    env=mobile_env,
                    capture_output=True,
                    check=False,
                    timeout=120,
                )
                if result.returncode:
                    raise SmokeFailure(
                        "Teste Flutter/HTTP real falhou; saída omitida para não expor credenciais."
                    )
            elif web:
                run_web_smoke(api_addr, simulator_addr, logs)
            else:
                offers_smoke("http://" + api_addr, "http://" + simulator_addr)
            sql = (
                "SELECT json_build_object('orders',(SELECT count(*) FROM "
                + schema
                + (
                    ".orders WHERE state='pending_payment'),'events',(SELECT count(*) FROM "
                    if mobile
                    else ".orders WHERE state='paid' AND picked_up_at IS NOT NULL),'events',(SELECT count(*) FROM "
                )
                + schema
                + ".payment_events WHERE processed_at IS NOT NULL),'charges',(SELECT count(*) FROM "
                + schema
                + ".simulated_charges WHERE status='paid'),'sessions',(SELECT count(*) FROM "
                + schema
                + ".local_checkout_sessions),'keys',(SELECT count(*) FROM "
                + schema
                + ".local_checkout_keys),'grants',(SELECT count(*) FROM "
                + schema
                + ".local_checkout_grants),'order_grants',(SELECT count(*) FROM "
                + schema
                + ".order_access_grants));"
            )
            persisted = checked(
                [
                    "docker",
                    "compose",
                    "-p",
                    DB_PROJECT,
                    "exec",
                    "-T",
                    "postgres",
                    "psql",
                    "-U",
                    "offers",
                    "-d",
                    "offers",
                    "-At",
                    "-v",
                    "ON_ERROR_STOP=1",
                    "-c",
                    sql,
                ]
            )
            expected = {
                "orders": 1,
                "events": 1,
                "charges": 1,
                "sessions": 1,
                "keys": 1,
                "grants": 2,
                "order_grants": 1,
            }
            if mobile:
                expected.update(events=0, charges=0, sessions=0, keys=0, grants=0)
            if json.loads(persisted) != expected:
                raise SmokeFailure(
                    "Persistência PostgreSQL não corresponde ao fluxo confirmado."
                )
            if mobile:
                print(
                    "PASS mobile client/HTTP/PostgreSQL: reserva única, credencial segura simulada, restart, consulta privada e estoque autoritativo. Não é E2E de dispositivo."
                )
            else:
                print(
                    "PASS PostgreSQL: pedido retirado, evento/cobrança únicos, sessão/chave únicas e duas credenciais de leitura."
                )
        finally:
            if previous_token is None:
                os.environ.pop("DEMO_OPERATOR_TOKEN", None)
            else:
                os.environ["DEMO_OPERATOR_TOKEN"] = previous_token
            if process.poll() is None:
                process.send_signal(signal.SIGINT)
                try:
                    process.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            print("Schema sintético mantido para inspeção:", schema)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument(
        "--web",
        action="store_true",
        help="Inclui BFF SvelteKit real por HTTP, sem browser, em vez do fluxo HTTP direto.",
    )
    modes.add_argument(
        "--mobile",
        action="store_true",
        help="Cliente Flutter real contra backend/PostgreSQL isolados; keystore substituído, sem dispositivo.",
    )
    args = parser.parse_args()
    try:
        temp_root = Path("/tmp/opencode") if Path("/tmp/opencode").is_dir() else None
        with tempfile.TemporaryDirectory(
            prefix="offers-smoke-", dir=temp_root
        ) as directory:
            workspace = Path(directory)
            binary = workspace / "offers"
            checked(
                ["mise", "exec", "--", "go", "build", "-o", str(binary), "./cmd/offers"]
            )
            for _ in range(2):
                one_run(binary, workspace, web=args.web, mobile=args.mobile)
        print(
            "PASS isolated smoke: duas execuções completas, sem reset ou chamada PSP."
        )
    except SmokeFailure as failure:
        print("FAIL isolated smoke:", str(failure))
        return 1
    except (OSError, ValueError, KeyError):
        print(
            "FAIL isolated smoke: confira ferramentas, PostgreSQL local e contrato; não apague dados para contornar erro."
        )
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
