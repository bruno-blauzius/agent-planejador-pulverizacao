"""Read-only checks of the realm settings the product depends on (never writes to the realm).

The realm is shared with other products and owned by the platform, so this module only
reports: ``violations`` break an assumption of the product (``--check`` fails on them) and
``warnings`` are recommendations the platform decides (see PLATFORM-REQUESTS.md).
"""

import re
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any


@dataclass(frozen=True)
class RealmReport:
    violations: list[str] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)


def evaluate(realm: Mapping[str, Any], requirements: Mapping[str, Any]) -> RealmReport:
    """Compare the realm representation with the ``realmRequirements`` section of the spec."""
    violations: list[str] = []
    warnings: list[str] = []

    for name, wanted in requirements.get("equals", {}).items():
        if realm.get(name) != wanted:
            violations.append(f"{name} is {_show(realm.get(name))}, expected {_show(wanted)}")

    for name, allowed in requirements.get("oneOf", {}).items():
        if realm.get(name) not in allowed:
            violations.append(
                f"{name} is {_show(realm.get(name))}, expected one of {', '.join(allowed)}"
            )

    policy = requirements.get("passwordPolicy")
    if policy:
        violations.extend(
            _password_policy_violations(str(realm.get("passwordPolicy") or ""), policy)
        )

    for name, wanted in requirements.get("warnWhenNot", {}).items():
        if realm.get(name) != wanted:
            warnings.append(
                f"{name} is {_show(realm.get(name))}; the platform should set it to {_show(wanted)}"
            )
    return RealmReport(violations, warnings)


def _password_policy_violations(policy: str, wanted: Mapping[str, Any]) -> list[str]:
    problems: list[str] = []
    length = re.search(r"length\((\d+)\)", policy)
    minimum = int(wanted.get("minLength", 0))
    if minimum and (not length or int(length.group(1)) < minimum):
        problems.append(f"passwordPolicy needs length({minimum}) or more")
    for rule in wanted.get("required", []):
        if not re.search(rf"\b{re.escape(rule)}\(\d+\)", policy):
            problems.append(f"passwordPolicy needs {rule}")
    return problems


def _show(value: Any) -> str:
    return (
        "unset" if value is None else str(value).lower() if isinstance(value, bool) else str(value)
    )
