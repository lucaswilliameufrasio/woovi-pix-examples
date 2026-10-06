import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch
from run_offers_smoke import (
    listening_addresses,
    native_test_command,
    run_android_smoke,
    SmokeFailure,
    native_failure_summary,
    worker_environment,
)

spec = importlib.util.spec_from_file_location(
    "smoke", Path(__file__).with_name("smoke.py")
)
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


class SmokeSafetyTest(unittest.TestCase):
    def test_accepts_only_explicit_loopback_bases(self):
        for base in ("http://127.0.0.1:8080", "http://[::1]:8081/"):
            self.assertEqual(smoke.local_url(base), base.rstrip("/"))

    def test_rejects_remote_credentials_and_ambiguous_bases(self):
        for base in (
            "https://api.woovi-sandbox.com",
            "http://example.com:8080",
            "http://localhost:8080",
            "http://0.0.0.0:8080",
            "http://127.0.0.1",
            "http://user:password@127.0.0.1:8080",
            "http://127.0.0.1:8080/api",
            "http://127.0.0.1:8080?token=test",
            "http://127.0.0.1:8080#fragment",
            "http://127.0.0.1:invalid",
            "http://127.0.0.1:99999",
        ):
            with self.subTest(base=base), self.assertRaises(smoke.SmokeFailure):
                smoke.local_url(base)

    def test_does_not_follow_redirects(self):
        with self.assertRaises(smoke.SmokeFailure):
            smoke.NoRedirect().redirect_request(
                None, None, 302, "", {}, "https://example.com"
            )

    def test_parses_default_go_slog_startup(self):
        text = "2026/10/04 19:59:36 INFO simulator listening addr=127.0.0.1:41179\n2026/10/04 19:59:36 INFO API listening addr=127.0.0.1:36651"
        self.assertEqual(
            listening_addresses(text), ("127.0.0.1:36651", "127.0.0.1:41179")
        )

    def test_missing_startup_is_not_success(self):
        self.assertEqual(listening_addresses("startup failed"), (None, None))

    def test_native_process_restart_preserves_app_installation_without_secrets(self):
        for phase in ("create", "restore"):
            command = native_test_command(
                "emulator-5586", phase, "127.0.0.1:31001", "127.0.0.1:31002"
            )
            self.assertIn("--no-uninstall", command)
            self.assertIn("--dart-define=NATIVE_SMOKE_PHASE=" + phase, command)
            self.assertFalse(
                any("TOKEN" in value or "DATABASE" in value for value in command)
            )

    def test_native_runner_refuses_ambiguous_and_non_generic_devices(self):
        with patch("run_offers_smoke.checked") as command:
            for device in ("", "usb-device", "localhost:5555"):
                with self.assertRaises(SmokeFailure):
                    run_android_smoke("127.0.0.1:31001", "127.0.0.1:31002", device)
            command.assert_not_called()
            for name in ("", "different-profile\nOK"):
                command.return_value = name
                with self.assertRaises(SmokeFailure):
                    run_android_smoke(
                        "127.0.0.1:31001", "127.0.0.1:31002", "emulator-5586"
                    )

    def test_worker_environment_does_not_receive_operator_credential(self):
        api_env = {
            "DATABASE_URL": "postgres://local-fixture",
            "DEMO_MODE": "true",
            "DEMO_OPERATOR_TOKEN": "operator-secret-fixture",
        }
        sanitized = worker_environment(api_env)
        self.assertEqual(sanitized["DATABASE_URL"], api_env["DATABASE_URL"])
        self.assertEqual(sanitized["DEMO_MODE"], "true")
        self.assertNotIn("DEMO_OPERATOR_TOKEN", sanitized)
        self.assertIn("DEMO_OPERATOR_TOKEN", api_env)

    def test_native_failure_summary_keeps_assertion_and_redacts_sensitive_values(self):
        output = (
            b"Expected: <paid>\n"
            b"Actual: <pending>\n"
            b"Exception: Bearer secret-token-with-long-value\n"
            b"order id 0123456789abcdef0123456789abcdef\n"
            b"http://127.0.0.1:54321/v1/orders?token=private\n"
        )
        summary = native_failure_summary(output, b"")
        self.assertIn("Expected: <paid>", summary)
        self.assertIn("Actual: <pending>", summary)
        self.assertNotIn("secret-token-with-long-value", summary)
        self.assertNotIn("0123456789abcdef0123456789abcdef", summary)
        self.assertNotIn("127.0.0.1", summary)
        self.assertNotIn("token=private", summary)


if __name__ == "__main__":
    unittest.main()
