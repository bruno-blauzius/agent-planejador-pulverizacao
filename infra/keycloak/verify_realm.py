"""Verify, against the live Keycloak, that the tokens carry exactly what the contract says.

    python infra/keycloak/verify_realm.py

For each development user (``seed_dev_users.py``) it asks Keycloak for the example access token
(``evaluate-scopes``, no login, no password) and compares ``resource_access.agentes-agro.roles``
with the permission table of ``services/api/api/openapi.yaml``. A user in no group (the
provisioner's service account) must get no product role. Nothing is written to the realm.
"""

import os
import re
import sys
import urllib.parse
from collections import defaultdict
from collections.abc import Callable, Mapping, Sequence
from pathlib import Path
from typing import Any, Protocol

import apply_product as app
import seed_dev_users as seed

OPENAPI = app.HERE.parent.parent / "services" / "api" / "api" / "openapi.yaml"
PROFILES = ("super_admin", "admin", "planning_operator")
REQUIRED_CLAIMS = ("sub", "email", "email_verified")
ROW = re.compile(
    r"^\s*\|\s*`(?P<permission>[a-z_.]+)`[^|]*"
    r"\|\s*(?P<super_admin>sim|não)\s*\|\s*(?P<admin>sim|não)\s*"
    r"\|\s*(?P<planning_operator>sim|não)\s*\|\s*$"
)


def load_contract(path: Path = OPENAPI) -> dict[str, set[str]]:
    """The permission table of the API contract: profile -> permissions it grants."""
    profiles: dict[str, set[str]] = defaultdict(set)
    found = False
    for line in path.read_text(encoding="utf-8").splitlines():
        match = ROW.match(line)
        if not match:
            continue
        found = True
        for profile in PROFILES:
            if match.group(profile) == "sim":
                profiles[profile].add(match.group("permission"))
    if not found:
        raise app.ConfigError(f"permission table not found in {path.name}")
    return dict(profiles)


class Realm(Protocol):
    def find_client(self, client_id: str) -> dict[str, Any] | None: ...
    def find_user(self, username: str) -> dict[str, Any] | None: ...
    def service_account_user_id(self, client_uuid: str) -> str: ...
    def example_access_token(self, client_uuid: str, user_id: str) -> dict[str, Any]: ...


class RestRealm:
    def __init__(self, client: app.AdminClient) -> None:
        self._client = client

    def find_client(self, client_id: str) -> dict[str, Any] | None:
        return self._client.find_client(client_id)

    def find_user(self, username: str) -> dict[str, Any] | None:
        return seed.find_user(self._client, username)

    def service_account_user_id(self, client_uuid: str) -> str:
        return self._client.service_account_user_id(client_uuid)

    def example_access_token(self, client_uuid: str, user_id: str) -> dict[str, Any]:
        path = (
            f"/clients/{client_uuid}/evaluate-scopes/generate-example-access-token"
            f"?scope=openid&userId={urllib.parse.quote(user_id, safe='')}"
        )
        token: dict[str, Any] = self._client.request("GET", path, ok=(200,))[1]
        return token


def verify(
    realm: Realm,
    product_client: str,
    contract: Mapping[str, set[str]],
    unassigned_client: str,
    out: Callable[[str], None] = print,
) -> list[str]:
    """Return the problems found (empty when the realm matches the contract)."""
    client = realm.find_client(product_client)
    if client is None:
        return [f"client {product_client} does not exist; run `make keycloak`"]
    problems: list[str] = []

    for seed_user in seed.SEED_USERS:
        user = realm.find_user(seed_user.username)
        if user is None:
            problems.append(f"{seed_user.username} does not exist; run `make keycloak-seed`")
            continue
        token = realm.example_access_token(client["id"], user["id"])
        expected = {seed_user.role} | set(contract.get(seed_user.role, set()))
        found = _check_token(token, product_client, expected)
        out(f"{seed_user.username} ({seed_user.role}): {'ok' if not found else 'FAILED'}")
        problems.extend(f"{seed_user.username}: {problem}" for problem in found)

    provisioner = realm.find_client(unassigned_client)
    if provisioner is None:
        problems.append(f"client {unassigned_client} does not exist")
        return problems
    user_id = realm.service_account_user_id(provisioner["id"])
    token = realm.example_access_token(client["id"], user_id)
    found = _check_token(token, product_client, set(), is_person=False)
    out(
        f"user in no group ({unassigned_client} service account): {'ok' if not found else 'FAILED'}"
    )
    problems.extend(f"user in no group: {problem}" for problem in found)
    return problems


def _check_token(
    token: Mapping[str, Any],
    product_client: str,
    expected_roles: set[str],
    *,
    is_person: bool = True,
) -> list[str]:
    problems: list[str] = []
    access = token.get("resource_access") or {}
    roles = set((access.get(product_client) or {}).get("roles", []))
    if roles != expected_roles:
        missing, extra = sorted(expected_roles - roles), sorted(roles - expected_roles)
        problems.append(f"{product_client} roles differ (missing {missing}, extra {extra})")
    other_clients = sorted(set(access) - {product_client})
    if other_clients:
        problems.append(f"roles of other clients in the token: {other_clients}")
    if "realm_access" in token:
        problems.append("the token carries realm_access")
    audience = token.get("aud")
    audiences = [audience] if isinstance(audience, str) else list(audience or [])
    if product_client not in audiences:
        problems.append(f"aud does not include {product_client}")
    if token.get("azp") != product_client:
        problems.append(f"azp is {token.get('azp')!r}, expected {product_client!r}")
    if is_person:
        problems.extend(f"claim {name} is missing" for name in REQUIRED_CLAIMS if name not in token)
    return problems


def main(
    argv: Sequence[str] | None = None,
    environ: Mapping[str, str] | None = None,
    out: Callable[[str], None] = print,
) -> int:
    del argv
    env = {
        **app.read_env_file(app.DEFAULT_ENV_FILE),
        **(os.environ if environ is None else environ),
    }
    try:
        settings = app.load_settings(env)
        state = app.load_desired_state(app.DEFAULT_SPEC, settings.placeholders)
        realm_name = settings.realm or state.realm
        if realm_name == app.FORBIDDEN_REALM:
            raise app.ConfigError(f"refusing to use the {app.FORBIDDEN_REALM!r} realm")
        contract = load_contract()
        client = app.AdminClient(settings, realm_name)
        try:
            problems = verify(
                RestRealm(client),
                state.product_client,
                contract,
                next(c.client_id for c in state.clients if c.client_id != state.product_client),
                out,
            )
        finally:
            client.close()
    except (app.ConfigError, app.KeycloakError) as err:
        out(f"error: {err}")
        return 1
    except OSError as err:
        out(f"error: cannot reach Keycloak: {err}")
        return 1
    for problem in problems:
        out(f"problem: {problem}")
    out("realm matches the contract" if not problems else f"{len(problems)} problem(s)")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
