#!/usr/bin/env python3
"""Run the local reservations browser flow against one isolated PostgreSQL schema."""

from __future__ import annotations

import os
import secrets
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / "reservas" / "backend"
WEB = ROOT / "reservas" / "web"


def runtime_environment() -> dict[str, str]:
    return {
        key: os.environ[key]
        for key in ("PATH", "HOME", "MISE_DATA_DIR", "MISE_CONFIG_DIR")
        if key in os.environ
    }


def database() -> tuple[urllib.parse.SplitResult, str]:
    parsed = urllib.parse.urlsplit(os.environ.get("TEST_DATABASE_URL", ""))
    host = parsed.hostname or ""
    if (
        parsed.scheme not in {"postgres", "postgresql"}
        or host not in {"127.0.0.1", "::1"}
        or parsed.port is None
        or parsed.path.lstrip("/") not in {"reservations_test", "reservations_ci"}
        or parsed.username is None
    ):
        raise RuntimeError(
            "TEST_DATABASE_URL must target reservations_test or reservations_ci on loopback."
        )
    return parsed, host


def available_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def psql(parsed: urllib.parse.SplitResult, host: str, command: str) -> None:
    environment = os.environ.copy()
    if parsed.password is not None:
        environment["PGPASSWORD"] = urllib.parse.unquote(parsed.password)
    environment["PGSSLMODE"] = "disable"
    subprocess.run(
        [
            "psql",
            "-X",
            "-v",
            "ON_ERROR_STOP=1",
            "--host",
            host,
            "--port",
            str(parsed.port),
            "--username",
            urllib.parse.unquote(parsed.username or ""),
            "--dbname",
            parsed.path.lstrip("/"),
            "--command",
            command,
        ],
        env=environment,
        check=True,
        stdout=subprocess.DEVNULL,
    )


def wait_for_url(url: str, process: subprocess.Popen[bytes], timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(
                "A local reservations application exited before becoming ready."
            )
        try:
            with urllib.request.urlopen(url, timeout=1) as response:
                if 200 <= response.status < 300:
                    return
        except (urllib.error.URLError, TimeoutError):
            time.sleep(0.2)
    raise RuntimeError(
        "A local reservations application did not become ready before timeout."
    )


def stop(process: subprocess.Popen[bytes] | None) -> None:
    if process is None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=10)
    except subprocess.TimeoutExpired:
        pass
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    if process.poll() is None:
        process.wait(timeout=5)


def main() -> None:
    parsed, host = database()
    schema = f"reservations_smoke_{uuid.uuid4().hex}"
    psql(parsed, host, f'CREATE SCHEMA "{schema}"')
    api: subprocess.Popen[bytes] | None = None
    web: subprocess.Popen[bytes] | None = None
    log_root = Path(tempfile.mkdtemp(prefix="reservations-smoke-", dir="/tmp/opencode"))
    api_log_path = log_root / "api.log"
    web_log_path = log_root / "web.log"
    try:
        api_port = available_port()
        web_port = available_port()
        operator_token = secrets.token_hex(32)
        simulator_token = secrets.token_hex(32)
        query = urllib.parse.parse_qsl(parsed.query, keep_blank_values=True)
        query = [(key, value) for key, value in query if key != "options"]
        query.append(("options", f"-csearch_path={schema},public"))
        database_url = urllib.parse.urlunsplit(
            parsed._replace(query=urllib.parse.urlencode(query))
        )

        api_environment = runtime_environment()
        api_environment.update(
            {
                "DATABASE_URL": database_url,
                "DEMO_OPERATOR_TOKEN": operator_token,
                "DEMO_SIMULATOR_TOKEN": simulator_token,
                "LISTEN_ADDR": f"127.0.0.1:{api_port}",
            }
        )
        with api_log_path.open("wb") as api_log:
            api = subprocess.Popen(
                ["mise", "exec", "--", "go", "run", "./cmd/server"],
                cwd=BACKEND,
                env=api_environment,
                stdout=api_log,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
        api_url = f"http://127.0.0.1:{api_port}"
        wait_for_url(f"{api_url}/healthz", api, 60)

        web_environment = runtime_environment()
        web_environment.update(
            {
                "API_BASE_URL": api_url,
                "DEMO_OPERATOR_TOKEN": operator_token,
                "DEMO_SIMULATOR_TOKEN": simulator_token,
            }
        )
        with web_log_path.open("wb") as web_log:
            web = subprocess.Popen(
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
                    str(web_port),
                    "--strictPort",
                ],
                cwd=WEB,
                env=web_environment,
                stdout=web_log,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
        web_url = f"http://127.0.0.1:{web_port}"
        try:
            wait_for_url(web_url, web, 60)
        except RuntimeError as error:
            detail = web_log_path.read_text(errors="replace")[-4000:]
            raise RuntimeError(f"{error}\nWeb log:\n{detail}") from error

        browser_environment = runtime_environment()
        browser_environment["BROWSER_SMOKE_BASE_URL"] = web_url
        subprocess.run(
            ["mise", "exec", "--", "npm", "run", "test:browser"],
            cwd=WEB,
            env=browser_environment,
            check=True,
        )
    finally:
        stop(web)
        stop(api)
        psql(parsed, host, f'DROP SCHEMA IF EXISTS "{schema}" CASCADE')


if __name__ == "__main__":
    main()
