"""Cross-language compatibility tests for the generated Python binding."""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

CONTRACTS = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(CONTRACTS / "generated" / "python"))

from argus_contracts import decode_kernel  # noqa: E402


class KernelCompatibilityTests(unittest.TestCase):
    """Run the manifest-owned corpus through the Python consumer boundary."""

    def test_compatibility_corpus(self) -> None:
        manifest = json.loads((CONTRACTS / "manifest.json").read_text(encoding="utf-8"))
        self.assertEqual(
            ["kernel/v1"], [item["name"] for item in manifest["contracts"]]
        )

        for fixture in manifest["contracts"][0]["fixtures"]:
            with self.subTest(fixture=fixture["path"]):
                data = (CONTRACTS / fixture["path"]).read_bytes()
                if fixture["bindingValid"]:
                    document = decode_kernel(data)
                    self.assertEqual(
                        "repo:github.com/example/orders", document.repository.id
                    )
                else:
                    with self.assertRaises(ValueError):
                        decode_kernel(data)


if __name__ == "__main__":
    unittest.main()
