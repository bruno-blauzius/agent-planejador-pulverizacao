"""Apply the Agentes Agro part of the shared Keycloak realm (ADR 0015).

The desired state lives in ``agentes-agro.json``. This script creates what is missing and
corrects drift in the objects the product owns: its clients, client roles, composite roles
and the groups under ``/agentes-agro``. It is idempotent, never deletes anything, never
touches objects outside the product prefix and never prints secrets.

    python infra/keycloak/apply_product.py                  # apply
    python infra/keycloak/apply_product.py --check          # report drift, change nothing
    python infra/keycloak/apply_product.py --init-secrets   # also generate missing client secrets
"""

import argparse
import http.client
import json
import os
import secrets
import sys
import time
import urllib.parse
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from string import Template
from typing import Any, Literal, Protocol

HERE = Path(__file__).resolve().parent
DEFAULT_SPEC = HERE / "agentes-agro.json"
DEFAULT_ENV_FILE = HERE.parent.parent / ".env"

MIN_SECRET_LENGTH = 32
REQUEST_TIMEOUT_SECONDS = 15
TOKEN_EXPIRY_MARGIN_SECONDS = 10
PAGE_SIZE = 1000
FORBIDDEN_REALM = "master"
SECRET_VARIABLES = (
    "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET",
    "KEYCLOAK_PROVISIONER_CLIENT_SECRET",
)

# Who a client role mapping belongs to: a group, a user (a service account) or a client (its scope).
Owner = Literal["groups", "users", "scope"]


class ConfigError(Exception):
    """The configuration or the spec is invalid; nothing was changed."""


class KeycloakError(Exception):
    """Keycloak answered with an unexpected status."""

    def __init__(self, method: str, path: str, status: int, detail: str) -> None:
        super().__init__(f"{method} {path} -> HTTP {status}: {detail}")


# --- Configuration ------------------------------------------------------------------------


@dataclass(frozen=True)
class Settings:
    """Where Keycloak is, how to authenticate, and the values the spec placeholders take."""

    base_url: str
    realm: str | None
    auth_realm: str
    admin_user: str | None
    admin_password: str | None
    admin_client_id: str | None
    admin_client_secret: str | None
    placeholders: Mapping[str, str]


def read_env_file(path: Path) -> dict[str, str]:
    """Read KEY=VALUE lines; a missing file yields no values."""
    values: dict[str, str] = {}
    if not path.is_file():
        return values
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, _, value = stripped.partition("=")
        values[key.strip()] = value.strip().strip("\"'")
    return values


def load_settings(env: Mapping[str, str]) -> Settings:
    """Build the settings from environment values (the process env wins over the .env file)."""
    base_url = env.get("KEYCLOAK_URL", "http://localhost:8180").rstrip("/")
    parts = urllib.parse.urlsplit(base_url)
    if parts.scheme not in ("http", "https") or not parts.hostname:
        raise ConfigError(f"KEYCLOAK_URL must be an http(s) URL, got {base_url!r}")

    admin_user = env.get("KEYCLOAK_ADMIN_USER") or None
    admin_password = env.get("KEYCLOAK_ADMIN_PASSWORD") or None
    admin_client_id = env.get("KEYCLOAK_ADMIN_CLIENT_ID") or None
    admin_client_secret = env.get("KEYCLOAK_ADMIN_CLIENT_SECRET") or None
    if not ((admin_user and admin_password) or (admin_client_id and admin_client_secret)):
        raise ConfigError(
            "set KEYCLOAK_ADMIN_USER and KEYCLOAK_ADMIN_PASSWORD, "
            "or KEYCLOAK_ADMIN_CLIENT_ID and KEYCLOAK_ADMIN_CLIENT_SECRET"
        )

    api_port = env.get("API_HOST_PORT", "8080")
    web_port = env.get("WEB_HOST_PORT", "3000")
    placeholders = {
        "API_URL": env.get("AGENTES_AGRO_API_URL", f"http://localhost:{api_port}").rstrip("/"),
        "API_INTERNAL_URL": env.get("AGENTES_AGRO_API_INTERNAL_URL", "http://agro-api:8080").rstrip(
            "/"
        ),
        "WEB_URL": env.get("AGENTES_AGRO_WEB_URL", f"http://localhost:{web_port}").rstrip("/"),
        "AGENTES_AGRO_CLIENT_SECRET": _client_secret(env, SECRET_VARIABLES[0]),
        "PROVISIONER_CLIENT_SECRET": _client_secret(env, SECRET_VARIABLES[1]),
    }
    return Settings(
        base_url=base_url,
        realm=env.get("KEYCLOAK_REALM") or None,
        auth_realm=env.get("KEYCLOAK_AUTH_REALM", "master"),
        admin_user=admin_user,
        admin_password=admin_password,
        admin_client_id=admin_client_id,
        admin_client_secret=admin_client_secret,
        placeholders=placeholders,
    )


def _client_secret(env: Mapping[str, str], name: str) -> str:
    value = env.get(name, "")
    if len(value) < MIN_SECRET_LENGTH:
        raise ConfigError(
            f"{name} must have at least {MIN_SECRET_LENGTH} characters "
            "(run with --init-secrets to generate it)"
        )
    return value


def init_secrets(env_file: Path, env: Mapping[str, str], out: Callable[[str], None]) -> list[str]:
    """Generate the client secrets that are missing or empty and store them in ``env_file``.

    Existing values are never replaced, and the generated values are never printed.
    Returns the names of the variables that were generated.
    """
    missing = [name for name in SECRET_VARIABLES if len(env.get(name, "")) < MIN_SECRET_LENGTH]
    if not missing:
        return []

    content = ""
    if env_file.is_file():
        # newline="" keeps the file's own line endings, so the rewrite does not change them.
        with env_file.open(encoding="utf-8", newline="") as handle:
            content = handle.read()
    ending = "\r\n" if "\r\n" in content else "\n"
    lines = content.splitlines()
    for name in missing:
        value = secrets.token_hex(32)
        slot = next((i for i, line in enumerate(lines) if line.strip() == f"{name}="), None)
        if slot is None:
            lines.append(f"{name}={value}")
        else:
            lines[slot] = f"{name}={value}"
    env_file.write_text(ending.join(lines) + ending, encoding="utf-8", newline="")
    out(f"generated {', '.join(missing)} in {env_file.name}")
    return missing


# --- Desired state ------------------------------------------------------------------------


@dataclass(frozen=True)
class ClientSpec:
    client_id: str
    representation: dict[str, Any]


@dataclass(frozen=True)
class RoleSpec:
    name: str
    description: str
    composites: tuple[str, ...] = ()


@dataclass(frozen=True)
class GroupSpec:
    path: str
    role: str | None = None


@dataclass(frozen=True)
class RoleGrant:
    client_id: str
    target_client_id: str
    roles: tuple[str, ...]


@dataclass(frozen=True)
class ClientScopeAssignment:
    client_id: str
    default: tuple[str, ...]


@dataclass(frozen=True)
class DesiredState:
    product: str
    realm: str
    product_client: str
    clients: tuple[ClientSpec, ...]
    roles: tuple[RoleSpec, ...]
    groups: tuple[GroupSpec, ...]
    service_account_grants: tuple[RoleGrant, ...]
    scope_mapping_grants: tuple[RoleGrant, ...]
    client_scopes: tuple[ClientScopeAssignment, ...]


def load_desired_state(path: Path, placeholders: Mapping[str, str]) -> DesiredState:
    """Read and validate the spec, resolving ``${PLACEHOLDER}`` values from ``placeholders``."""
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as err:
        raise ConfigError(f"cannot read the spec {path}: {err}") from err
    data = _substitute(raw, placeholders)

    product = data["product"]
    permissions = [(item["name"], item["description"]) for item in data["permissions"]]
    profiles = data["profiles"]
    permission_names = [name for name, _ in permissions]
    profile_names = [profile["name"] for profile in profiles]

    _require(
        len(set(permission_names + profile_names)) == len(permission_names + profile_names),
        "permission and profile names must be unique",
    )
    for profile in profiles:
        unknown = set(profile["permissions"]) - set(permission_names)
        _require(
            not unknown, f"profile {profile['name']} uses unknown permissions {sorted(unknown)}"
        )

    roles = tuple(RoleSpec(name, description) for name, description in permissions) + tuple(
        RoleSpec(p["name"], p["description"], tuple(p["permissions"])) for p in profiles
    )

    clients = tuple(ClientSpec(c["clientId"], c) for c in data["clients"])
    for client in clients:
        _require(
            client.client_id.startswith(product),
            f"client {client.client_id!r} is outside the product prefix {product!r}",
        )
    _require(
        data["productClient"] in {c.client_id for c in clients},
        "productClient must be one of the declared clients",
    )

    groups = tuple(GroupSpec(g["path"], g.get("role")) for g in data["groups"])
    declared: set[str] = set()
    for group in groups:
        parent = group.path.rpartition("/")[0]
        _require(
            group.path == f"/{product}" or group.path.startswith(f"/{product}/"),
            f"group {group.path!r} is outside the product prefix",
        )
        _require(not parent or parent in declared, f"group {group.path!r} needs {parent!r} first")
        _require(
            group.role is None or group.role in profile_names,
            f"group {group.path!r} maps an unknown profile {group.role!r}",
        )
        declared.add(group.path)

    declared_clients = {c.client_id for c in clients}
    service_account_grants = tuple(
        RoleGrant(g["client"], g["target"], tuple(g["roles"]))
        for g in data.get("serviceAccountRoles", [])
    )
    scope_mapping_grants = tuple(
        RoleGrant(g["client"], g["target"], tuple(g["roles"]))
        for g in data.get("scopeMappings", [])
    )
    client_scopes = tuple(
        ClientScopeAssignment(a["client"], tuple(a["default"]))
        for a in data.get("clientScopes", [])
    )
    for client_id in (
        *(g.client_id for g in service_account_grants),
        *(g.client_id for g in scope_mapping_grants),
        *(a.client_id for a in client_scopes),
    ):
        _require(client_id in declared_clients, f"grant for undeclared client {client_id!r}")

    return DesiredState(
        product,
        data["realm"],
        data["productClient"],
        clients,
        roles,
        groups,
        service_account_grants,
        scope_mapping_grants,
        client_scopes,
    )


def _substitute(node: Any, placeholders: Mapping[str, str]) -> Any:
    """Resolve placeholders after parsing, so a value can never break the JSON."""
    if isinstance(node, str):
        try:
            return Template(node).substitute(placeholders)
        except KeyError as err:
            raise ConfigError(f"unknown placeholder ${err.args[0]} in the spec") from err
        except ValueError as err:
            raise ConfigError(f"invalid placeholder in the spec: {err}") from err
    if isinstance(node, list):
        return [_substitute(item, placeholders) for item in node]
    if isinstance(node, dict):
        return {key: _substitute(value, placeholders) for key, value in node.items()}
    return node


def _require(condition: bool, message: str) -> None:
    if not condition:
        raise ConfigError(message)


# --- What the reconciler needs from Keycloak ----------------------------------------------


class Keycloak(Protocol):
    """Port implemented by ``AdminClient`` (REST) and by the in-memory fake of the tests."""

    def find_client(self, client_id: str) -> dict[str, Any] | None: ...
    def create_client(self, representation: dict[str, Any]) -> str: ...
    def update_client(self, client_uuid: str, representation: dict[str, Any]) -> None: ...
    def get_client_secret(self, client_uuid: str) -> str: ...
    def list_protocol_mappers(self, client_uuid: str) -> list[dict[str, Any]]: ...
    def create_protocol_mapper(self, client_uuid: str, mapper: dict[str, Any]) -> None: ...
    def update_protocol_mapper(self, client_uuid: str, mapper: dict[str, Any]) -> None: ...
    def list_client_roles(self, client_uuid: str) -> dict[str, dict[str, Any]]: ...
    def create_client_role(self, client_uuid: str, name: str, description: str) -> None: ...
    def update_client_role(self, client_uuid: str, name: str, description: str) -> None: ...
    def get_composites(self, client_uuid: str, role_name: str) -> set[str]: ...
    def add_composites(
        self, client_uuid: str, role_name: str, roles: list[dict[str, Any]]
    ) -> None: ...
    def remove_composites(
        self, client_uuid: str, role_name: str, roles: list[dict[str, Any]]
    ) -> None: ...
    def find_group(self, path: str) -> dict[str, Any] | None: ...
    def create_group(self, name: str, parent_id: str | None) -> str: ...
    def list_child_groups(self, group_id: str) -> list[dict[str, Any]]: ...
    def service_account_user_id(self, client_uuid: str) -> str: ...
    def get_default_client_scopes(self, client_uuid: str) -> set[str]: ...
    def add_default_client_scope(self, client_uuid: str, scope_name: str) -> None: ...
    def get_role_mappings(self, owner: Owner, owner_id: str, client_uuid: str) -> set[str]: ...
    def add_role_mappings(
        self, owner: Owner, owner_id: str, client_uuid: str, roles: list[dict[str, Any]]
    ) -> None: ...
    def remove_role_mappings(
        self, owner: Owner, owner_id: str, client_uuid: str, roles: list[dict[str, Any]]
    ) -> None: ...


# --- Reconciliation -----------------------------------------------------------------------


class Reconciler:
    """Brings Keycloak to the desired state, or only reports what would change."""

    def __init__(
        self,
        keycloak: Keycloak,
        state: DesiredState,
        *,
        apply_changes: bool,
        out: Callable[[str], None] = print,
    ) -> None:
        self._kc = keycloak
        self._state = state
        self._apply = apply_changes
        self._out = out
        self.changes = 0
        self.warnings = 0

    def run(self) -> None:
        client_uuids = {
            spec.client_id: self._reconcile_client(spec) for spec in self._state.clients
        }
        product_uuid = client_uuids[self._state.product_client]
        self._reconcile_roles(product_uuid)
        self._reconcile_groups(product_uuid)
        self._reconcile_grants(
            self._state.service_account_grants, client_uuids, "users", "the service account"
        )
        self._reconcile_grants(self._state.scope_mapping_grants, client_uuids, "scope", "the scope")
        self._reconcile_client_scopes(client_uuids)

    def _change(self, message: str) -> bool:
        """Record a change; return whether it must be applied now."""
        self.changes += 1
        self._out(("" if self._apply else "would ") + message)
        return self._apply

    def _warn(self, message: str) -> None:
        self.warnings += 1
        self._out(f"warning: {message}")

    # Clients

    def _reconcile_client(self, spec: ClientSpec) -> str | None:
        body = {k: v for k, v in spec.representation.items() if k != "protocolMappers"}
        current = self._kc.find_client(spec.client_id)
        if current is None:
            client_uuid = None
            if self._change(f"create client {spec.client_id}"):
                client_uuid = self._kc.create_client(body)
        else:
            client_uuid = current["id"]
            drift = _client_drift(current, body)
            if "secret" in body and self._kc.get_client_secret(client_uuid) != body["secret"]:
                drift.append("secret")
            if drift and self._change(f"update client {spec.client_id} ({', '.join(drift)})"):
                self._kc.update_client(client_uuid, _client_update(current, body, drift))
        self._reconcile_mappers(spec, client_uuid)
        return client_uuid

    def _reconcile_mappers(self, spec: ClientSpec, client_uuid: str | None) -> None:
        wanted = spec.representation.get("protocolMappers", [])
        existing = (
            {m["name"]: m for m in self._kc.list_protocol_mappers(client_uuid)}
            if client_uuid
            else {}
        )
        for mapper in wanted:
            have = existing.get(mapper["name"])
            if have is None:
                if self._change(f"create mapper {mapper['name']} on {spec.client_id}"):
                    self._kc.create_protocol_mapper(client_uuid, mapper)
                continue
            if _mapper_drifts(have, mapper) and self._change(
                f"update mapper {mapper['name']} on {spec.client_id}"
            ):
                merged_config = {**have.get("config", {}), **mapper["config"]}
                self._kc.update_protocol_mapper(client_uuid, {**have, "config": merged_config})

    # Roles

    def _reconcile_roles(self, client_uuid: str | None) -> None:
        current = self._kc.list_client_roles(client_uuid) if client_uuid else {}
        for role in self._state.roles:
            have = current.get(role.name)
            if have is None:
                if self._change(f"create role {role.name}"):
                    self._kc.create_client_role(client_uuid, role.name, role.description)
                continue
            if (have.get("description") or "") != role.description and self._change(
                f"update role {role.name} (description)"
            ):
                self._kc.update_client_role(client_uuid, role.name, role.description)

        declared = {role.name for role in self._state.roles}
        for name in sorted(set(current) - declared):
            self._warn(f"role {name} exists in the product client but is not declared")

        known = self._kc.list_client_roles(client_uuid) if client_uuid and self._apply else current
        for role in self._state.roles:
            wanted = set(role.composites)
            have_names = (
                self._kc.get_composites(client_uuid, role.name)
                if client_uuid and role.name in known
                else set()
            )
            missing, extra = sorted(wanted - have_names), sorted(have_names - wanted)
            if missing and self._change(f"add composites to {role.name}: {', '.join(missing)}"):
                self._kc.add_composites(client_uuid, role.name, [known[n] for n in missing])
            if extra and self._change(f"remove composites from {role.name}: {', '.join(extra)}"):
                self._kc.remove_composites(client_uuid, role.name, [known[n] for n in extra])

    # Groups

    def _reconcile_groups(self, client_uuid: str | None) -> None:
        roles = self._kc.list_client_roles(client_uuid) if client_uuid else {}
        group_ids: dict[str, str | None] = {}
        for spec in self._state.groups:
            group_id = self._ensure_group(spec.path, group_ids)
            group_ids[spec.path] = group_id
            have = (
                self._kc.get_role_mappings("groups", group_id, client_uuid)
                if group_id and client_uuid
                else set()
            )
            wanted = {spec.role} if spec.role else set()
            self._sync_mappings(
                "groups", f"group {spec.path}", group_id, client_uuid, wanted, have, roles
            )
        self._warn_undeclared_groups(group_ids)

    def _ensure_group(self, path: str, group_ids: Mapping[str, str | None]) -> str | None:
        existing = self._kc.find_group(path)
        if existing is not None:
            return existing["id"]
        parent_path, _, name = path.rpartition("/")
        parent_id = group_ids.get(parent_path) if parent_path else None
        if self._change(f"create group {path}"):
            return self._kc.create_group(name, parent_id)
        return None

    def _warn_undeclared_groups(self, group_ids: Mapping[str, str | None]) -> None:
        declared = {spec.path for spec in self._state.groups}
        for path, group_id in group_ids.items():
            if not group_id:
                continue
            for child in self._kc.list_child_groups(group_id):
                child_path = f"{path}/{child['name']}"
                if child_path not in declared:
                    self._warn(f"group {child_path} exists but is not declared")

    # Service account roles and scope mappings

    def _reconcile_grants(
        self,
        grants: Sequence[RoleGrant],
        client_uuids: Mapping[str, str | None],
        owner: Owner,
        what: str,
    ) -> None:
        for grant in grants:
            target = self._kc.find_client(grant.target_client_id)
            if target is None:
                raise ConfigError(f"client {grant.target_client_id!r} does not exist in the realm")
            roles = self._kc.list_client_roles(target["id"])
            unknown = set(grant.roles) - set(roles)
            if unknown:
                raise ConfigError(f"{grant.target_client_id} has no roles {sorted(unknown)}")
            client_uuid = client_uuids[grant.client_id]
            owner_id = self._owner_id(owner, client_uuid)
            have = self._kc.get_role_mappings(owner, owner_id, target["id"]) if owner_id else set()
            self._sync_mappings(
                owner,
                f"{what} of {grant.client_id} ({grant.target_client_id})",
                owner_id,
                target["id"],
                set(grant.roles),
                have,
                roles,
            )

    def _owner_id(self, owner: Owner, client_uuid: str | None) -> str | None:
        """The id a mapping hangs from: the service account user, or the client itself."""
        if client_uuid is None:
            return None
        return self._kc.service_account_user_id(client_uuid) if owner == "users" else client_uuid

    # Client scopes

    def _reconcile_client_scopes(self, client_uuids: Mapping[str, str | None]) -> None:
        for assignment in self._state.client_scopes:
            client_uuid = client_uuids[assignment.client_id]
            have = self._kc.get_default_client_scopes(client_uuid) if client_uuid else set()
            missing = sorted(set(assignment.default) - have)
            if missing and self._change(
                f"add default client scopes to {assignment.client_id}: {', '.join(missing)}"
            ):
                for name in missing:
                    self._kc.add_default_client_scope(client_uuid, name)

    # Shared

    def _sync_mappings(
        self,
        owner: Owner,
        label: str,
        owner_id: str | None,
        client_uuid: str | None,
        wanted: set[str],
        have: set[str],
        roles_by_name: Mapping[str, dict[str, Any]],
    ) -> None:
        missing, extra = sorted(wanted - have), sorted(have - wanted)
        if missing and self._change(f"grant {', '.join(missing)} to {label}"):
            self._kc.add_role_mappings(
                owner, owner_id, client_uuid, [roles_by_name[n] for n in missing]
            )
        if extra and self._change(f"revoke {', '.join(extra)} from {label}"):
            self._kc.remove_role_mappings(
                owner, owner_id, client_uuid, [roles_by_name[n] for n in extra]
            )


def _client_drift(current: Mapping[str, Any], desired: Mapping[str, Any]) -> list[str]:
    """Names of the desired fields whose current value differs (secret is checked apart)."""
    drift: list[str] = []
    for key, wanted in desired.items():
        if key == "secret":
            continue
        have = current.get(key)
        if key == "attributes":
            have_attrs = have or {}
            if any(str(have_attrs.get(name)) != str(value) for name, value in wanted.items()):
                drift.append(key)
        elif isinstance(wanted, list):
            if sorted(have or []) != sorted(wanted):
                drift.append(key)
        elif have != wanted:
            drift.append(key)
    return drift


def _client_update(
    current: Mapping[str, Any], desired: Mapping[str, Any], drift: Sequence[str]
) -> dict[str, Any]:
    """Full representation for the PUT: what exists, overlaid with what is desired."""
    body = {k: v for k, v in current.items() if k not in ("secret", "protocolMappers")}
    body.update({k: v for k, v in desired.items() if k not in ("secret", "attributes")})
    body["attributes"] = {**(current.get("attributes") or {}), **desired.get("attributes", {})}
    if "secret" in drift:
        body["secret"] = desired["secret"]
    return body


def _mapper_drifts(current: Mapping[str, Any], desired: Mapping[str, Any]) -> bool:
    have_config = current.get("config") or {}
    return (
        current.get("protocol") != desired["protocol"]
        or current.get("protocolMapper") != desired["protocolMapper"]
        or any(str(have_config.get(k)) != str(v) for k, v in desired["config"].items())
    )


# --- Keycloak Admin REST API --------------------------------------------------------------


class AdminClient:
    """Keycloak Admin REST API over the standard library (ADR 0008: stdlib first)."""

    def __init__(self, settings: Settings, realm: str) -> None:
        parts = urllib.parse.urlsplit(settings.base_url)
        self._https = parts.scheme == "https"
        self._host = parts.hostname or ""
        self._port = parts.port
        self._prefix = parts.path.rstrip("/")
        self._settings = settings
        self._realm = realm
        self._cached_token: str | None = None
        self._expires_at = 0.0
        self._connection: http.client.HTTPConnection | None = None

    def close(self) -> None:
        if self._connection is not None:
            self._connection.close()
            self._connection = None

    # Transport

    def _send(
        self, method: str, path: str, body: bytes | None, headers: dict[str, str]
    ) -> tuple[int, dict[str, str], bytes]:
        # One keep-alive connection for the whole run: on Windows every new connection to
        # "localhost" waits for the IPv6 attempt to fail before falling back to IPv4.
        if self._connection is None:
            connection_class = (
                http.client.HTTPSConnection if self._https else http.client.HTTPConnection
            )
            self._connection = connection_class(
                self._host, self._port, timeout=REQUEST_TIMEOUT_SECONDS
            )
        try:
            return self._exchange(self._connection, method, path, body, headers)
        except (
            http.client.RemoteDisconnected,
            ConnectionResetError,
            ConnectionAbortedError,
            BrokenPipeError,
        ):
            # The server closed the idle connection; http.client reconnects on the next request.
            self._connection.close()
            return self._exchange(self._connection, method, path, body, headers)

    def _exchange(
        self,
        connection: http.client.HTTPConnection,
        method: str,
        path: str,
        body: bytes | None,
        headers: dict[str, str],
    ) -> tuple[int, dict[str, str], bytes]:
        connection.request(method, self._prefix + path, body=body, headers=headers)
        response = connection.getresponse()
        response_headers = {name.lower(): value for name, value in response.getheaders()}
        return response.status, response_headers, response.read()

    def _access_token(self) -> str:
        if self._cached_token and time.monotonic() < self._expires_at:
            return self._cached_token
        s = self._settings
        if s.admin_client_id and s.admin_client_secret:
            form = {
                "grant_type": "client_credentials",
                "client_id": s.admin_client_id,
                "client_secret": s.admin_client_secret,
            }
        else:
            form = {
                "grant_type": "password",
                "client_id": "admin-cli",
                "username": s.admin_user or "",
                "password": s.admin_password or "",
            }
        status, _, raw = self._send(
            "POST",
            f"/realms/{_quote(s.auth_realm)}/protocol/openid-connect/token",
            urllib.parse.urlencode(form).encode(),
            {"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
        )
        if status != 200:
            raise KeycloakError("POST", "admin token", status, _error_detail(raw))
        data = json.loads(raw)
        self._cached_token = data["access_token"]
        self._expires_at = (
            time.monotonic() + int(data.get("expires_in", 60)) - TOKEN_EXPIRY_MARGIN_SECONDS
        )
        return self._cached_token

    def _call(
        self,
        method: str,
        path: str,
        body: Any = None,
        *,
        ok: tuple[int, ...] = (200, 201, 204),
    ) -> tuple[int, Any, dict[str, str]]:
        payload = None if body is None else json.dumps(body).encode()
        headers = {"Authorization": f"Bearer {self._access_token()}", "Accept": "application/json"}
        if payload is not None:
            headers["Content-Type"] = "application/json"
        status, response_headers, raw = self._send(
            method, f"/admin/realms/{_quote(self._realm)}{path}", payload, headers
        )
        if status not in ok:
            raise KeycloakError(method, path, status, _error_detail(raw))
        return status, (json.loads(raw) if raw else None), response_headers

    def _get(self, path: str) -> Any:
        return self._call("GET", path, ok=(200,))[1]

    @staticmethod
    def _created_id(headers: Mapping[str, str]) -> str:
        location = headers.get("location", "")
        if not location:
            raise KeycloakError("POST", "create", 201, "no Location header in the response")
        return location.rstrip("/").rsplit("/", 1)[-1]

    # Clients

    def find_client(self, client_id: str) -> dict[str, Any] | None:
        clients = self._get(f"/clients?clientId={_quote(client_id)}")
        return clients[0] if clients else None

    def create_client(self, representation: dict[str, Any]) -> str:
        _, _, headers = self._call("POST", "/clients", representation, ok=(201,))
        return self._created_id(headers)

    def update_client(self, client_uuid: str, representation: dict[str, Any]) -> None:
        self._call("PUT", f"/clients/{client_uuid}", representation, ok=(204,))

    def get_client_secret(self, client_uuid: str) -> str:
        return self._get(f"/clients/{client_uuid}/client-secret").get("value", "")

    def list_protocol_mappers(self, client_uuid: str) -> list[dict[str, Any]]:
        return self._get(f"/clients/{client_uuid}/protocol-mappers/models")

    def create_protocol_mapper(self, client_uuid: str, mapper: dict[str, Any]) -> None:
        self._call("POST", f"/clients/{client_uuid}/protocol-mappers/models", mapper, ok=(201,))

    def update_protocol_mapper(self, client_uuid: str, mapper: dict[str, Any]) -> None:
        path = f"/clients/{client_uuid}/protocol-mappers/models/{mapper['id']}"
        self._call("PUT", path, mapper, ok=(204,))

    # Client roles

    def list_client_roles(self, client_uuid: str) -> dict[str, dict[str, Any]]:
        roles = self._get(f"/clients/{client_uuid}/roles?max={PAGE_SIZE}")
        return {role["name"]: role for role in roles}

    def create_client_role(self, client_uuid: str, name: str, description: str) -> None:
        body = {"name": name, "description": description}
        self._call("POST", f"/clients/{client_uuid}/roles", body, ok=(201,))

    def update_client_role(self, client_uuid: str, name: str, description: str) -> None:
        body = {"name": name, "description": description}
        self._call("PUT", f"/clients/{client_uuid}/roles/{_quote(name)}", body, ok=(204,))

    def get_composites(self, client_uuid: str, role_name: str) -> set[str]:
        composites = self._get(f"/clients/{client_uuid}/roles/{_quote(role_name)}/composites")
        return {
            role["name"]
            for role in composites
            if role.get("clientRole") and role.get("containerId") == client_uuid
        }

    def add_composites(self, client_uuid: str, role_name: str, roles: list[dict[str, Any]]) -> None:
        path = f"/clients/{client_uuid}/roles/{_quote(role_name)}/composites"
        self._call("POST", path, roles, ok=(204,))

    def remove_composites(
        self, client_uuid: str, role_name: str, roles: list[dict[str, Any]]
    ) -> None:
        path = f"/clients/{client_uuid}/roles/{_quote(role_name)}/composites"
        self._call("DELETE", path, roles, ok=(204,))

    # Groups

    def find_group(self, path: str) -> dict[str, Any] | None:
        segments = "/".join(_quote(part) for part in path.strip("/").split("/"))
        status, data, _ = self._call("GET", f"/group-by-path/{segments}", ok=(200, 404))
        return data if status == 200 else None

    def create_group(self, name: str, parent_id: str | None) -> str:
        path = "/groups" if parent_id is None else f"/groups/{parent_id}/children"
        _, _, headers = self._call("POST", path, {"name": name}, ok=(201,))
        return self._created_id(headers)

    def list_child_groups(self, group_id: str) -> list[dict[str, Any]]:
        return self._get(f"/groups/{group_id}/children?first=0&max={PAGE_SIZE}")

    # Service accounts, client scopes and role mappings

    def service_account_user_id(self, client_uuid: str) -> str:
        return self._get(f"/clients/{client_uuid}/service-account-user")["id"]

    def get_default_client_scopes(self, client_uuid: str) -> set[str]:
        scopes = self._get(f"/clients/{client_uuid}/default-client-scopes")
        return {scope["name"] for scope in scopes}

    def add_default_client_scope(self, client_uuid: str, scope_name: str) -> None:
        scopes = {scope["name"]: scope["id"] for scope in self._get("/client-scopes")}
        if scope_name not in scopes:
            raise ConfigError(f"client scope {scope_name!r} does not exist in the realm")
        path = f"/clients/{client_uuid}/default-client-scopes/{scopes[scope_name]}"
        self._call("PUT", path, ok=(204,))

    @staticmethod
    def _mapping_path(owner: Owner, owner_id: str, client_uuid: str) -> str:
        if owner == "scope":
            return f"/clients/{owner_id}/scope-mappings/clients/{client_uuid}"
        return f"/{owner}/{owner_id}/role-mappings/clients/{client_uuid}"

    def get_role_mappings(self, owner: Owner, owner_id: str, client_uuid: str) -> set[str]:
        roles = self._get(self._mapping_path(owner, owner_id, client_uuid))
        return {role["name"] for role in roles}

    def add_role_mappings(
        self, owner: Owner, owner_id: str, client_uuid: str, roles: list[dict[str, Any]]
    ) -> None:
        self._call("POST", self._mapping_path(owner, owner_id, client_uuid), roles, ok=(204,))

    def remove_role_mappings(
        self, owner: Owner, owner_id: str, client_uuid: str, roles: list[dict[str, Any]]
    ) -> None:
        self._call("DELETE", self._mapping_path(owner, owner_id, client_uuid), roles, ok=(204,))


def _quote(segment: str) -> str:
    return urllib.parse.quote(segment, safe="")


def _error_detail(raw: bytes) -> str:
    """The error text from a Keycloak response; never the request, so no secret leaks."""
    try:
        data = json.loads(raw)
    except json.JSONDecodeError:
        return raw[:200].decode("utf-8", errors="replace")
    if isinstance(data, dict):
        for key in ("errorMessage", "error_description", "error"):
            if data.get(key):
                return str(data[key])
    return "no details"


# --- Command line -------------------------------------------------------------------------


def main(
    argv: Sequence[str] | None = None,
    environ: Mapping[str, str] | None = None,
    out: Callable[[str], None] = print,
) -> int:
    parser = argparse.ArgumentParser(
        description="Apply the Agentes Agro clients, roles and groups to the shared Keycloak realm."
    )
    parser.add_argument("--check", action="store_true", help="report drift; change nothing")
    parser.add_argument(
        "--init-secrets", action="store_true", help="generate missing client secrets in the .env"
    )
    parser.add_argument("--spec", type=Path, default=DEFAULT_SPEC, help="desired state (JSON)")
    parser.add_argument("--env-file", type=Path, default=DEFAULT_ENV_FILE, help="the .env file")
    args = parser.parse_args(argv)
    if args.check and args.init_secrets:
        parser.error("--check changes nothing, so it cannot be combined with --init-secrets")

    env = {**read_env_file(args.env_file), **(os.environ if environ is None else environ)}
    try:
        if args.init_secrets and init_secrets(args.env_file, env, out):
            env = {**read_env_file(args.env_file), **(os.environ if environ is None else environ)}
        settings = load_settings(env)
        state = load_desired_state(args.spec, settings.placeholders)
        realm = settings.realm or state.realm
        if realm == FORBIDDEN_REALM:
            raise ConfigError(f"refusing to change the {FORBIDDEN_REALM!r} realm")
        client = AdminClient(settings, realm)
        try:
            reconciler = Reconciler(client, state, apply_changes=not args.check, out=out)
            out(f"realm {realm} at {settings.base_url}" + (" (check only)" if args.check else ""))
            reconciler.run()
        finally:
            client.close()
    except (ConfigError, KeycloakError) as err:
        out(f"error: {err}")
        return 1
    except OSError as err:
        out(f"error: cannot reach Keycloak: {err}")
        return 1

    if reconciler.changes == 0:
        out("up to date")
    elif args.check:
        out(f"{reconciler.changes} change(s) pending")
        return 1
    else:
        out(f"{reconciler.changes} change(s) applied")
    return 0


if __name__ == "__main__":
    sys.exit(main())
