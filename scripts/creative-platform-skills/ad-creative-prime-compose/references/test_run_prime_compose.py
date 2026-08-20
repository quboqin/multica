import json
import unittest
from unittest.mock import patch

import run_prime_compose


class CompletedProcess:
    def __init__(self, returncode=0, stdout="", stderr=""):
        self.returncode = returncode
        self.stdout = stdout
        self.stderr = stderr


class RunPrimeComposeTests(unittest.TestCase):
    def test_build_command_uses_only_backend_prime_compose_cli(self):
        self.assertEqual(
            run_prime_compose.build_command("order-1", "variant-2", "json"),
            [
                "multica",
                "creative",
                "order",
                "prime-compose",
                "order-1",
                "--variant",
                "variant-2",
                "--output",
                "json",
            ],
        )

    @patch("run_prime_compose.subprocess.run")
    def test_successful_backend_result_is_returned(self, run):
        payload = {"status": "completed", "order_id": "order-1"}
        run.return_value = CompletedProcess(stdout=json.dumps(payload))

        with patch.dict(run_prime_compose.os.environ, {}, clear=True):
            result = run_prime_compose.run_prime_compose("order-1", "variant-2", "json")

        self.assertEqual(result, payload)
        run.assert_called_once()
        self.assertEqual(run.call_args.args[0], [
            "multica",
            "creative",
            "order",
            "prime-compose",
            "order-1",
            "--variant",
            "variant-2",
            "--output",
            "json",
        ])
        self.assertFalse(run.call_args.kwargs["check"])
        self.assertTrue(run.call_args.kwargs["capture_output"])
        self.assertTrue(run.call_args.kwargs["text"])
        self.assertEqual(run.call_args.kwargs["encoding"], "utf-8")
        self.assertEqual(run.call_args.kwargs["errors"], "replace")
        self.assertEqual(run.call_args.kwargs["env"]["MULTICA_HTTP_TIMEOUT"], "5m")

    @patch("run_prime_compose.subprocess.run")
    def test_success_signal_is_accepted(self, run):
        run.return_value = CompletedProcess(stdout='{"success": true}')

        self.assertEqual(
            run_prime_compose.run_prime_compose("order-1", "variant-2", "json"),
            {"success": True},
        )

    @patch("run_prime_compose.subprocess.run")
    def test_missing_completion_signal_fails(self, run):
        run.return_value = CompletedProcess(stdout='{"status": "running"}')

        with self.assertRaisesRegex(ValueError, "not successful or completed"):
            run_prime_compose.run_prime_compose("order-1", "variant-2", "json")

    @patch("run_prime_compose.subprocess.run")
    def test_command_failure_fails_without_parsing_output(self, run):
        run.return_value = CompletedProcess(returncode=2, stderr="backend failed")

        with self.assertRaisesRegex(RuntimeError, "exit code 2"):
            run_prime_compose.run_prime_compose("order-1", "variant-2", "json")


if __name__ == "__main__":
    unittest.main()
