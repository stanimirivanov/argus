"""Validate the contract manifest, schemas, and compatibility corpus offline."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, FormatChecker

CONTRACTS = Path(__file__).resolve().parents[1]


def read_json(path: Path) -> Any:
    """Read a UTF-8 JSON document."""
    return json.loads(path.read_text(encoding="utf-8"))


def validate_contracts() -> tuple[int, int]:
    """Validate all manifest-owned schemas and fixtures without network access."""
    manifest = read_json(CONTRACTS / "manifest.json")
    if manifest.get("bundleVersion") != "0.1.0":
        raise ValueError("manifest bundleVersion must be 0.1.0")

    contracts = manifest.get("contracts")
    if not isinstance(contracts, list) or not contracts:
        raise ValueError("manifest contracts must be a non-empty array")

    declared_schemas: set[str] = set()
    declared_fixtures: set[str] = set()
    fixture_count = 0
    for contract in contracts:
        schema_path = contract["schema"]
        if schema_path in declared_schemas:
            raise ValueError(f"duplicate schema in manifest: {schema_path}")
        declared_schemas.add(schema_path)

        schema = read_json(CONTRACTS / schema_path)
        Draft202012Validator.check_schema(schema)
        validator = Draft202012Validator(schema, format_checker=FormatChecker())

        for fixture in contract["fixtures"]:
            fixture_path = fixture["path"]
            if fixture_path in declared_fixtures:
                raise ValueError(f"duplicate fixture in manifest: {fixture_path}")
            declared_fixtures.add(fixture_path)
            document = read_json(CONTRACTS / fixture_path)
            schema_valid = not list(validator.iter_errors(document))
            if schema_valid != fixture["schemaValid"]:
                expectation = "valid" if fixture["schemaValid"] else "invalid"
                raise ValueError(f"{fixture_path}: expected schema {expectation}")
            fixture_count += 1

    actual_schemas = {
        path.relative_to(CONTRACTS).as_posix()
        for path in (CONTRACTS / "schemas").rglob("*.schema.json")
    }
    actual_fixtures = {
        path.relative_to(CONTRACTS).as_posix()
        for path in (CONTRACTS / "fixtures").rglob("*.json")
    }
    if actual_schemas != declared_schemas:
        raise ValueError("manifest must own every schema exactly once")
    if actual_fixtures != declared_fixtures:
        raise ValueError("manifest must own every fixture exactly once")

    return len(contracts), fixture_count


if __name__ == "__main__":
    contract_count, checked_fixtures = validate_contracts()
    print(
        f"Validated {contract_count} contract and {checked_fixtures} fixtures offline."
    )
