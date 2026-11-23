"""Read-only audit of who holds the product roles in the realm.

The rule of the product (ADR 0015, D3) is: a user gets access by being in **exactly one** of the
role subgroups, and nowhere else grants a product role. The audit prints counts and short ids
with masked e-mails, never full e-mails, names or secrets.
"""

from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from typing import Any, Protocol


class AuditSource(Protocol):
    """What the audit reads from Keycloak (implemented by ``AdminClient`` and by the tests)."""

    def list_group_members(self, group_id: str) -> list[dict[str, Any]]: ...
    def list_users_with_role(self, client_uuid: str, role_name: str) -> list[dict[str, Any]]: ...


@dataclass(frozen=True)
class RoleGroup:
    path: str
    group_id: str
    role: str


@dataclass
class AuditResult:
    violations: list[str] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)


def run_audit(
    source: AuditSource,
    groups: Sequence[RoleGroup],
    product_client_uuid: str,
    role_names: Sequence[str],
    out: Callable[[str], None],
) -> AuditResult:
    result = AuditResult()
    memberships: dict[str, list[str]] = {}
    people: dict[str, Mapping[str, Any]] = {}

    for group in groups:
        members = source.list_group_members(group.group_id)
        out(f"{group.path}: {len(members)} member(s)")
        for member in members:
            memberships.setdefault(member["id"], []).append(group.path)
            people[member["id"]] = member

    for user_id, paths in memberships.items():
        if len(paths) > 1:
            result.violations.append(
                f"user {_short(user_id)} is in {len(paths)} role groups ({', '.join(paths)}); "
                "exactly one is the rule"
            )

    for role in role_names:
        for direct in source.list_users_with_role(product_client_uuid, role):
            result.violations.append(
                f"user {_short(direct['id'])} has the product role {role} assigned directly, "
                "outside a role group"
            )

    for user_id, user in people.items():
        if not user.get("enabled", True):
            result.warnings.append(
                f"user {_short(user_id)} ({_mask(user.get('email'))}) is disabled "
                "but still in a group"
            )
        if not user.get("emailVerified", False):
            result.warnings.append(
                f"user {_short(user_id)} ({_mask(user.get('email'))}) has not verified the e-mail"
            )

    for message in result.violations:
        out(f"violation: {message}")
    for message in result.warnings:
        out(f"warning: {message}")
    out(
        f"audit: {len(people)} user(s) in role groups, "
        f"{len(result.violations)} violation(s), {len(result.warnings)} warning(s)"
    )
    return result


def _short(user_id: str) -> str:
    return user_id[:8]


def _mask(email: object) -> str:
    """``alice@example.com`` -> ``a***@e***.com``; enough to recognise, not to contact."""
    if not isinstance(email, str) or "@" not in email:
        return "no e-mail"
    local, _, domain = email.partition("@")
    name, _, suffix = domain.rpartition(".")
    return f"{local[:1]}***@{name[:1]}***.{suffix}" if name else f"{local[:1]}***@***"
