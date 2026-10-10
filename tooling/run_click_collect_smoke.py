#!/usr/bin/env python3
"""Run the click-and-collect browser journey against isolated local PostgreSQL schema."""

from __future__ import annotations

import argparse
import os
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / "click-collect" / "backend"
WEB = ROOT / "click-collect" / "web"
OUTPUT_ROOT = Path("/tmp/opencode")


def runtime_environment() -> dict[str, str]:
    return {
        key: os.environ[key]
        for key in ("PATH", "HOME", "MISE_DATA_DIR", "MISE_CONFIG_DIR")
        if key in os.environ
    }


def local_database_url() -> tuple[urllib.parse.SplitResult, str]:
    value = os.environ.get("TEST_DATABASE_URL", "")
    parsed = urllib.parse.urlsplit(value)
    host = parsed.hostname or ""
    if (
        parsed.scheme not in {"postgres", "postgresql"}
        or host not in {"127.0.0.1", "::1"}
        or parsed.port is None
        or parsed.path.lstrip("/") not in {"click_collect", "click_collect_test"}
        or parsed.username is None
        or parsed.password is None
    ):
        raise RuntimeError(
            "TEST_DATABASE_URL must explicitly target the click_collect or click_collect_test database on loopback."
        )
    return parsed, host


def available_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def create_schema(parsed: urllib.parse.SplitResult, host: str, schema: str) -> None:
    environment = os.environ.copy()
    environment["PGPASSWORD"] = urllib.parse.unquote(parsed.password or "")
    environment["PGSSLMODE"] = "disable"
    command = [
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
        f'CREATE SCHEMA "{schema}"',
    ]
    subprocess.run(command, env=environment, check=True, stdout=subprocess.DEVNULL)


def drop_schema(parsed: urllib.parse.SplitResult, host: str, schema: str) -> None:
    environment = os.environ.copy()
    environment["PGPASSWORD"] = urllib.parse.unquote(parsed.password or "")
    environment["PGSSLMODE"] = "disable"
    command = [
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
        f'DROP SCHEMA IF EXISTS "{schema}" CASCADE',
    ]
    subprocess.run(command, env=environment, check=True, stdout=subprocess.DEVNULL)


def wait_for_url(url: str, process: subprocess.Popen[bytes], timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError("A local application exited before becoming ready.")
        try:
            with urllib.request.urlopen(url, timeout=1) as response:
                if 200 <= response.status < 300:
                    return
        except (urllib.error.URLError, TimeoutError):
            time.sleep(0.2)
    raise RuntimeError("A local application did not become ready before the deadline.")


def stop_process(process: subprocess.Popen[bytes] | None) -> None:
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


def run_browser_iteration(
    parsed: urllib.parse.SplitResult,
    host: str,
    output: Path,
    iteration: int,
) -> None:
    schema = f"click_collect_smoke_{uuid.uuid4().hex}"
    create_schema(parsed, host, schema)
    api_process: subprocess.Popen[bytes] | None = None
    web_process: subprocess.Popen[bytes] | None = None
    try:
        api_port = available_port()
        web_port = available_port()
        operator_token = secrets.token_hex(32)
        simulator_token = secrets.token_hex(32)
        query = urllib.parse.parse_qsl(parsed.query, keep_blank_values=True)
        query = [(key, value) for key, value in query if key != "options"]
        query.append(("options", f"-csearch_path={schema}"))
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
        api_process = subprocess.Popen(
            ["mise", "exec", "--", "go", "run", "./cmd/server"],
            cwd=BACKEND,
            env=api_environment,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        api_url = f"http://127.0.0.1:{api_port}"
        wait_for_url(f"{api_url}/healthz", api_process, 45)

        web_environment = runtime_environment()
        web_environment.update(
            {
                "API_BASE_URL": api_url,
                "DEMO_OPERATOR_TOKEN": operator_token,
                "DEMO_SIMULATOR_TOKEN": simulator_token,
            }
        )
        web_process = subprocess.Popen(
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
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        web_url = f"http://127.0.0.1:{web_port}"
        wait_for_url(web_url, web_process, 45)

        browser_environment = runtime_environment()
        browser_environment.update(
            {
                "CLICK_COLLECT_WEB_BASE_URL": web_url,
                "BROWSER_SMOKE_OUTPUT_DIR": str(output / f"browser-{iteration}"),
            }
        )
        (output / f"browser-{iteration}").mkdir(parents=True, exist_ok=True)
        subprocess.run(
            [
                "mise",
                "exec",
                "--",
                "npm",
                "run",
                "test:browser",
                "--",
                "--config=playwright.config.ts",
            ],
            cwd=WEB,
            env=browser_environment,
            check=True,
        )
    finally:
        stop_process(web_process)
        stop_process(api_process)
        drop_schema(parsed, host, schema)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", action="store_true", required=True)
    args = parser.parse_args()
    del args
    try:
        parsed, host = local_database_url()
        OUTPUT_ROOT.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(
            prefix="click-collect-smoke-", dir=OUTPUT_ROOT
        ) as output_directory:
            output = Path(output_directory)
            for iteration in range(1, 3):
                run_browser_iteration(parsed, host, output, iteration)
                print(f"PASS click-collect browser iteration {iteration}/2")
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"FAIL click-collect browser smoke: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
