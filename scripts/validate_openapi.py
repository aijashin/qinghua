#!/usr/bin/env python3
"""Validate local OpenAPI references and route coverage without network access.

This is a lightweight repository consistency check. If openapi-spec-validator is
installed, the full OpenAPI 3.1 validator also runs. No app data or .env is read.
"""
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent.parent
METHODS = {"get", "put", "post", "delete", "options", "head", "patch", "trace"}


def check():
    document = json.loads((ROOT / "docs/openapi.json").read_text())
    assert document.get("openapi", "").startswith("3.1."), "Expected OpenAPI 3.1"
    refs = []

    def resolve(ref):
        assert ref.startswith("#/"), f"External reference is not locally verified: {ref}"
        value = document
        for part in ref[2:].split("/"):
            value = value[part.replace("~1", "/").replace("~0", "~")]
        return value

    def walk(value):
        if isinstance(value, dict):
            if "$ref" in value:
                refs.append(value["$ref"])
                resolve(value["$ref"])
            for child in value.values():
                walk(child)
        elif isinstance(value, list):
            for child in value:
                walk(child)

    walk(document)
    actual = set()
    ids = set()
    for path, path_item in document["paths"].items():
        assert path.startswith("/"), f"Invalid path: {path}"
        path_names = set(re.findall(r"\{([^}]+)\}", path))
        for method, operation in path_item.items():
            if method not in METHODS:
                continue
            actual.add((method.upper(), path))
            identity = operation["operationId"]
            assert identity not in ids, f"Duplicate operationId: {identity}"
            ids.add(identity)
            assert operation.get("responses"), f"Missing responses: {method} {path}"
            parameters = path_item.get("parameters", []) + operation.get("parameters", [])
            parameters = [resolve(p["$ref"]) if "$ref" in p else p for p in parameters]
            declared = {p["name"] for p in parameters if p["in"] == "path" and p.get("required") is True}
            assert declared == path_names, f"Path parameter mismatch: {method} {path}"

    routes = (ROOT / "backend/internal/app/app.go").read_text()
    expected = set(re.findall(r'"(GET|POST|PATCH|DELETE|PUT|HEAD|OPTIONS) /api/v1([^" ]*)"\s*,', routes))
    # The application registers collection/detail GET routes in one finite loop.
    loop = re.search(r'for _, kind := range \[\]string\{([^}]+)\}', routes)
    assert loop, "Expected entity route registration loop"
    for kind in re.findall(r'"([a-z-]+)"', loop.group(1)):
        expected.add(("GET", "/" + kind))
        expected.add(("GET", "/" + kind + "/{id}"))
    assert actual == expected, f"Route drift: missing={sorted(expected-actual)}, extra={sorted(actual-expected)}"
    try:
        from openapi_spec_validator import validate_spec
    except ImportError:
        full = "not installed; full specification validation not run"
    else:
        validate_spec(document)
        full = "passed"
    print(f"PASS: OpenAPI JSON, {len(document['paths'])} paths, {len(actual)} operations, {len(refs)} local references, unique operation IDs and Go route coverage")
    print(f"Full OpenAPI 3.1 validation: {full}")


if __name__ == "__main__":
    try:
        check()
    except (AssertionError, KeyError, ValueError) as exc:
        print(f"FAIL: {exc}", file=sys.stderr)
        raise SystemExit(1)
