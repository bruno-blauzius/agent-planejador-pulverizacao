import itertools
import json
import re
import tempfile
import threading
import unittest
from collections import defaultdict
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

import apply_product as app
import verify_realm as verify

SPEC = app.DEFAULT_SPEC
OPENAPI = app.HERE.parent.parent / "services" / "api" / "api" / "openapi.yaml"

AGRO_SECRET = "a" * 64
PROVISIONER_SECRET = "b" * 64
PLACEHOLDERS = {
    "API_URL": "http://localhost:8080",
    "API_INTERNAL_URL": "http://agro-api:8080",
    "WEB_URL": "http://localhost:3000",
    "AGENTES_AGRO_CLIENT_SECRET": AGRO_SECRET,
    "PROVISIONER_CLIENT_SECRET": PROVISIONER_SECRET,
}
ADMIN_ENV = {"KEYCLOAK_ADMIN_USER": "admin", "KEYCLOAK_ADMIN_PASSWORD": "pw"}


REALM_DEFAULT_SCOPES = ("web-origins", "acr", "roles", "profile", "basic", "email")
REALM_OPTIONAL_SCOPES = ("address", "phone", "offline_access", "organization", "microprofile-jwt")
COMPLIANT_REALM: dict[str, Any] = {
    "registrationAllowed": False,
    "verifyEmail": True,
    "duplicateEmailsAllowed": False,
    "sslRequired": "external",
    "eventsEnabled": True,
    "adminEventsEnabled": True,
    "bruteForceProtected": True,
    "passwordPolicy": "length(8) and digits(1) and lowerCase(1) and notUsername",
    "revokeRefreshToken": True,
    "adminPermissionsEnabled": True,
}


class FakeKeycloak:
    """In-memory stand-in with the semantics of the Keycloak calls the reconciler uses."""

    def __init__(self) -> None:
        self.writes = 0
        self._ids = itertools.count(1)
        self.clients: dict[str, dict[str, Any]] = {}
        self.secrets: dict[str, str] = {}
        self.mappers: dict[str, list[dict[str, Any]]] = defaultdict(list)
        self.roles: dict[str, dict[str, dict[str, Any]]] = defaultdict(dict)
        self.composites: dict[tuple[str, str], set[str]] = defaultdict(set)
        self.groups: dict[str, dict[str, Any]] = {}
        self.mappings: dict[tuple[str, str, str], set[str]] = defaultdict(set)
        self.default_scopes: dict[str, set[str]] = defaultdict(set)
        self.optional_scopes: dict[str, set[str]] = defaultdict(set)
        self.foreign_composites: dict[tuple[str, str], list[app.RoleRef]] = defaultdict(list)
        self.realm_group_mappings: dict[str, dict[str, dict[str, Any]]] = defaultdict(dict)
        self.realm: dict[str, Any] = dict(COMPLIANT_REALM)
        self.members: dict[str, list[dict[str, Any]]] = defaultdict(list)
        self.direct_users: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
        self.realm_management = self._seed_client(
            "realm-management", ["manage-users", "view-users", "manage-clients"]
        )

    def _new_id(self) -> str:
        return f"id-{next(self._ids)}"

    def _seed_client(self, client_id: str, role_names: list[str]) -> str:
        client_uuid = self._new_id()
        self.clients[client_uuid] = {"id": client_uuid, "clientId": client_id}
        for name in role_names:
            self.roles[client_uuid][name] = {"id": self._new_id(), "name": name}
        return client_uuid

    def _write(self) -> None:
        self.writes += 1

    def get_realm(self) -> dict[str, Any]:
        return dict(self.realm)

    def client_id_of(self, client_uuid: str) -> str:
        return str(self.clients[client_uuid]["clientId"])

    # Clients

    def find_client(self, client_id: str) -> dict[str, Any] | None:
        for client in self.clients.values():
            if client["clientId"] == client_id:
                return dict(client)
        return None

    def create_client(self, representation: dict[str, Any]) -> str:
        self._write()
        client_uuid = self._new_id()
        body = {k: v for k, v in representation.items() if k != "secret"}
        self.clients[client_uuid] = {"id": client_uuid, **body}
        self.secrets[client_uuid] = representation.get("secret", "")
        # What Keycloak attaches to every new client; the reconciler prunes what is not declared.
        self.default_scopes[client_uuid] = set(REALM_DEFAULT_SCOPES)
        self.optional_scopes[client_uuid] = set(REALM_OPTIONAL_SCOPES)
        if representation.get("serviceAccountsEnabled"):
            self.default_scopes[client_uuid].add("service_account")
        return client_uuid

    def update_client(self, client_uuid: str, representation: dict[str, Any]) -> None:
        self._write()
        body = {k: v for k, v in representation.items() if k != "secret"}
        self.clients[client_uuid] = {"id": client_uuid, **body}
        if "secret" in representation:
            self.secrets[client_uuid] = representation["secret"]

    def get_client_secret(self, client_uuid: str) -> str:
        return self.secrets.get(client_uuid, "")

    def list_protocol_mappers(self, client_uuid: str) -> list[dict[str, Any]]:
        return [dict(m) for m in self.mappers[client_uuid]]

    def create_protocol_mapper(self, client_uuid: str, mapper: dict[str, Any]) -> None:
        self._write()
        self.mappers[client_uuid].append({"id": self._new_id(), **mapper})

    def update_protocol_mapper(self, client_uuid: str, mapper: dict[str, Any]) -> None:
        self._write()
        self.mappers[client_uuid] = [
            mapper if m["id"] == mapper["id"] else m for m in self.mappers[client_uuid]
        ]

    def delete_protocol_mapper(self, client_uuid: str, mapper_id: str) -> None:
        self._write()
        self.mappers[client_uuid] = [m for m in self.mappers[client_uuid] if m["id"] != mapper_id]

    # Roles

    def list_client_roles(self, client_uuid: str) -> dict[str, dict[str, Any]]:
        return {name: dict(role) for name, role in self.roles[client_uuid].items()}

    def create_client_role(self, client_uuid: str, name: str, description: str) -> None:
        self._write()
        self.roles[client_uuid][name] = {
            "id": self._new_id(),
            "name": name,
            "description": description,
        }

    def update_client_role(self, client_uuid: str, name: str, description: str) -> None:
        self._write()
        self.roles[client_uuid][name]["description"] = description

    def get_composites(self, client_uuid: str, role_name: str) -> list[app.RoleRef]:
        own = [
            app.RoleRef(
                client_uuid, self.client_id_of(client_uuid), name, self.roles[client_uuid][name]
            )
            for name in sorted(self.composites[(client_uuid, role_name)])
        ]
        return own + list(self.foreign_composites[(client_uuid, role_name)])

    def add_composites(self, client_uuid: str, role_name: str, roles: list[dict[str, Any]]) -> None:
        self._write()
        self.composites[(client_uuid, role_name)] |= {role["name"] for role in roles}

    def remove_composites(
        self, client_uuid: str, role_name: str, roles: list[dict[str, Any]]
    ) -> None:
        self._write()
        ids = {role["id"] for role in roles}
        key = (client_uuid, role_name)
        self.composites[key] = {
            n for n in self.composites[key] if self.roles[client_uuid][n]["id"] not in ids
        }
        self.foreign_composites[key] = [
            ref for ref in self.foreign_composites[key] if ref.representation["id"] not in ids
        ]

    # Groups

    def find_group(self, path: str) -> dict[str, Any] | None:
        return self.groups.get(path)

    def create_group(self, name: str, parent_id: str | None) -> str:
        self._write()
        parent_path = next((p for p, g in self.groups.items() if g["id"] == parent_id), "")
        group_id = self._new_id()
        self.groups[f"{parent_path}/{name}"] = {"id": group_id, "name": name}
        return group_id

    def list_child_groups(self, group_id: str) -> list[dict[str, Any]]:
        parent = next((p for p, g in self.groups.items() if g["id"] == group_id), None)
        return [
            g
            for p, g in self.groups.items()
            if parent is not None and p.rpartition("/")[0] == parent
        ]

    def list_group_role_mappings(self, group_id: str) -> list[app.RoleRef]:
        refs = [
            app.RoleRef(None, "realm", name, rep)
            for name, rep in self.realm_group_mappings[group_id].items()
        ]
        for (owner, owner_id, client_uuid), names in self.mappings.items():
            if owner == "groups" and owner_id == group_id:
                refs.extend(
                    app.RoleRef(
                        client_uuid,
                        self.client_id_of(client_uuid),
                        name,
                        self.roles[client_uuid].get(name, {"id": name, "name": name}),
                    )
                    for name in sorted(names)
                )
        return refs

    def remove_group_role_mappings(
        self, group_id: str, container_id: str | None, roles: list[dict[str, Any]]
    ) -> None:
        self._write()
        names = {role["name"] for role in roles}
        if container_id is None:
            for name in names:
                self.realm_group_mappings[group_id].pop(name, None)
        else:
            self.mappings[("groups", group_id, container_id)] -= names

    def list_group_members(self, group_id: str) -> list[dict[str, Any]]:
        return list(self.members[group_id])

    def list_users_with_role(self, client_uuid: str, role_name: str) -> list[dict[str, Any]]:
        return list(self.direct_users[(client_uuid, role_name)])

    # Service accounts, client scopes and role mappings

    def service_account_user_id(self, client_uuid: str) -> str:
        return f"service-account-{client_uuid}"

    def _scopes(self, kind: str) -> dict[str, set[str]]:
        return self.default_scopes if kind == "default" else self.optional_scopes

    def get_client_scopes(self, client_uuid: str, kind: Any) -> set[str]:
        return set(self._scopes(kind)[client_uuid])

    def add_client_scope(self, client_uuid: str, kind: Any, scope_name: str) -> None:
        self._write()
        self._scopes(kind)[client_uuid].add(scope_name)

    def remove_client_scope(self, client_uuid: str, kind: Any, scope_name: str) -> None:
        self._write()
        self._scopes(kind)[client_uuid].discard(scope_name)

    def get_role_mappings(self, owner: str, owner_id: str, client_uuid: str) -> set[str]:
        return set(self.mappings[(owner, owner_id, client_uuid)])

    def add_role_mappings(
        self, owner: str, owner_id: str, client_uuid: str, roles: list[dict[str, Any]]
    ) -> None:
        self._write()
        self.mappings[(owner, owner_id, client_uuid)] |= {role["name"] for role in roles}

    def remove_role_mappings(
        self, owner: str, owner_id: str, client_uuid: str, roles: list[dict[str, Any]]
    ) -> None:
        self._write()
        self.mappings[(owner, owner_id, client_uuid)] -= {role["name"] for role in roles}


def reconcile(
    fake: FakeKeycloak, *, apply_changes: bool = True
) -> tuple[app.Reconciler, list[str]]:
    lines: list[str] = []
    state = app.load_desired_state(SPEC, PLACEHOLDERS)
    reconciler = app.Reconciler(fake, state, apply_changes=apply_changes, out=lines.append)
    reconciler.run()
    return reconciler, lines


class ReconcileTest(unittest.TestCase):
    def setUp(self) -> None:
        self.fake = FakeKeycloak()
        self.state = app.load_desired_state(SPEC, PLACEHOLDERS)
        self.product_uuid = ""

    def client_uuid(self, client_id: str) -> str:
        client = self.fake.find_client(client_id)
        assert client is not None
        return str(client["id"])

    def apply_once(self) -> None:
        reconcile(self.fake)
        self.product_uuid = self.client_uuid("agentes-agro")

    def test_first_run_creates_clients_roles_groups_and_grants(self) -> None:
        self.apply_once()

        self.assertIsNotNone(self.fake.find_client("agentes-agro-provisioner"))
        self.assertEqual(self.fake.secrets[self.product_uuid], AGRO_SECRET)
        expected_roles = {role.name for role in self.state.roles}
        self.assertEqual(set(self.fake.roles[self.product_uuid]), expected_roles)
        for role in self.state.roles:
            self.assertEqual(
                self.fake.composites[(self.product_uuid, role.name)], set(role.composites)
            )
        self.assertEqual(
            set(self.fake.groups),
            {group.path for group in self.state.groups},
        )
        admins = self.fake.groups["/agentes-agro/admins"]["id"]
        self.assertEqual(self.fake.mappings[("groups", admins, self.product_uuid)], {"admin"})
        provisioner = self.client_uuid("agentes-agro-provisioner")
        self.assertEqual(
            self.fake.mappings[
                ("users", f"service-account-{provisioner}", self.fake.realm_management)
            ],
            {"manage-users", "view-users"},
        )
        self.assertEqual(
            {m["name"] for m in self.fake.mappers[self.product_uuid]},
            {"audience-agentes-agro", "client-roles-agentes-agro"},
        )
        self.assertEqual(
            self.fake.mappings[("scope", provisioner, self.fake.realm_management)],
            {"manage-users", "view-users"},
        )
        self.assertEqual(
            self.fake.default_scopes[provisioner], {"service_account", "basic", "kc-admin-api"}
        )
        self.assertEqual(self.fake.optional_scopes[provisioner], set())
        self.assertEqual(
            self.fake.default_scopes[self.product_uuid], {"acr", "profile", "basic", "email"}
        )

    def test_second_run_changes_nothing(self) -> None:
        self.apply_once()
        writes = self.fake.writes

        reconciler, _ = reconcile(self.fake)

        self.assertEqual(reconciler.changes, 0)
        self.assertEqual(self.fake.writes, writes)

    def test_check_mode_reports_changes_without_writing(self) -> None:
        reconciler, lines = reconcile(self.fake, apply_changes=False)

        self.assertGreater(reconciler.changes, 0)
        self.assertEqual(self.fake.writes, 0)
        self.assertTrue(all(line.startswith("would ") for line in lines))

    def test_composite_drift_is_corrected_in_both_directions(self) -> None:
        self.apply_once()
        composites = self.fake.composites[(self.product_uuid, "admin")]
        composites.discard("applications.confirm")
        composites.add("companies.manage")

        reconcile(self.fake)

        expected = next(r for r in self.state.roles if r.name == "admin").composites
        self.assertEqual(self.fake.composites[(self.product_uuid, "admin")], set(expected))

    def test_group_with_extra_product_role_is_revoked(self) -> None:
        self.apply_once()
        admins = self.fake.groups["/agentes-agro/admins"]["id"]
        self.fake.mappings[("groups", admins, self.product_uuid)].add("super_admin")

        reconcile(self.fake)

        self.assertEqual(self.fake.mappings[("groups", admins, self.product_uuid)], {"admin"})

    def test_service_account_keeps_least_privilege(self) -> None:
        self.apply_once()
        provisioner = self.client_uuid("agentes-agro-provisioner")
        key = ("users", f"service-account-{provisioner}", self.fake.realm_management)
        self.fake.mappings[key].add("manage-clients")

        reconcile(self.fake)

        self.assertEqual(self.fake.mappings[key], {"manage-users", "view-users"})

    def test_provisioner_scope_is_kept_to_the_two_roles_it_needs(self) -> None:
        self.apply_once()
        provisioner = self.client_uuid("agentes-agro-provisioner")
        key = ("scope", provisioner, self.fake.realm_management)
        self.fake.mappings[key].discard("view-users")
        self.fake.mappings[key].add("manage-clients")

        reconcile(self.fake)

        self.assertEqual(self.fake.mappings[key], {"manage-users", "view-users"})

    def test_provisioner_gets_the_admin_api_scope_back(self) -> None:
        self.apply_once()
        provisioner = self.client_uuid("agentes-agro-provisioner")
        self.fake.default_scopes[provisioner].remove("kc-admin-api")

        reconciler, lines = reconcile(self.fake)

        self.assertIn("kc-admin-api", self.fake.default_scopes[provisioner])
        self.assertEqual(reconciler.changes, 1)
        self.assertIn("kc-admin-api", lines[0])

    def test_client_and_secret_drift_are_corrected(self) -> None:
        self.apply_once()
        self.fake.clients[self.product_uuid]["redirectUris"] = ["http://evil.example/callback"]
        self.fake.clients[self.product_uuid]["fullScopeAllowed"] = True
        self.fake.secrets[self.product_uuid] = "c" * 64

        reconciler, lines = reconcile(self.fake)

        client = self.fake.clients[self.product_uuid]
        self.assertEqual(
            client["redirectUris"],
            ["http://localhost:8080/auth/callback", "http://localhost:8080/auth/login"],
        )
        self.assertFalse(client["fullScopeAllowed"])
        self.assertEqual(self.fake.secrets[self.product_uuid], AGRO_SECRET)
        self.assertEqual(reconciler.changes, 1)
        self.assertIn("secret", lines[0])

    def test_undeclared_objects_only_warn_and_are_never_deleted(self) -> None:
        self.apply_once()
        self.fake.roles[self.product_uuid]["stray_role"] = {"id": "x", "name": "stray_role"}
        self.fake.groups["/agentes-agro/stray"] = {"id": "y", "name": "stray"}

        reconciler, lines = reconcile(self.fake)

        self.assertEqual(reconciler.changes, 0)
        self.assertEqual(reconciler.warnings, 2)
        self.assertIn("stray_role", self.fake.roles[self.product_uuid])
        self.assertIn("/agentes-agro/stray", self.fake.groups)
        self.assertTrue(any("stray_role" in line for line in lines))

    def test_output_never_contains_the_secrets(self) -> None:
        _, lines = reconcile(self.fake)

        output = "\n".join(lines)
        self.assertNotIn(AGRO_SECRET, output)
        self.assertNotIn(PROVISIONER_SECRET, output)


class SpecTest(unittest.TestCase):
    def load_mutated(self, mutate: Any) -> app.DesiredState:
        data = json.loads(SPEC.read_text(encoding="utf-8"))
        mutate(data)
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "spec.json"
            path.write_text(json.dumps(data), encoding="utf-8")
            return app.load_desired_state(path, PLACEHOLDERS)

    def test_client_outside_the_product_prefix_is_rejected(self) -> None:
        with self.assertRaises(app.ConfigError):
            self.load_mutated(lambda d: d["clients"][0].update(clientId="other-product"))

    def test_group_outside_the_product_prefix_is_rejected(self) -> None:
        with self.assertRaises(app.ConfigError):
            self.load_mutated(lambda d: d["groups"].append({"path": "/other/admins"}))

    def test_profile_with_unknown_permission_is_rejected(self) -> None:
        with self.assertRaises(app.ConfigError):
            self.load_mutated(lambda d: d["profiles"][0]["permissions"].append("nope.nope"))

    def test_group_needs_its_parent_declared_first(self) -> None:
        with self.assertRaises(app.ConfigError):
            self.load_mutated(lambda d: d["groups"].reverse())

    def test_unknown_placeholder_is_rejected(self) -> None:
        with self.assertRaises(app.ConfigError):
            app.load_desired_state(SPEC, {"API_URL": "http://localhost:8080"})

    def test_scope_mapping_for_an_undeclared_client_is_rejected(self) -> None:
        with self.assertRaises(app.ConfigError):
            self.load_mutated(lambda d: d["scopeMappings"][0].update(client="agentes-agro-ghost"))


class ContractTest(unittest.TestCase):
    """The realm must implement the permission table the API contract documents (ADR 0015, D3)."""

    def contract_table(self) -> dict[str, set[str]]:
        return verify.load_contract(OPENAPI)

    def test_realm_profiles_match_the_openapi_permission_table(self) -> None:
        state = app.load_desired_state(SPEC, PLACEHOLDERS)
        realm = {role.name: set(role.composites) for role in state.roles if role.composites}

        self.assertEqual(realm, dict(self.contract_table()))

    def test_every_realm_permission_is_in_the_openapi_permission_enum(self) -> None:
        state = app.load_desired_state(SPEC, PLACEHOLDERS)
        permissions = {role.name for role in state.roles if not role.composites}
        enum = set(re.findall(r"^\s+- ([a-z_]+\.[a-z_]+)\s*$", OPENAPI.read_text("utf-8"), re.M))

        self.assertTrue(permissions <= enum, sorted(permissions - enum))


class SettingsAndSecretsTest(unittest.TestCase):
    def env(self, **overrides: str) -> dict[str, str]:
        base = {
            **ADMIN_ENV,
            "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": AGRO_SECRET,
            "KEYCLOAK_PROVISIONER_CLIENT_SECRET": PROVISIONER_SECRET,
        }
        return {**base, **overrides}

    def test_short_secret_is_rejected(self) -> None:
        with self.assertRaises(app.ConfigError):
            app.load_settings(self.env(KEYCLOAK_PROVISIONER_CLIENT_SECRET="short"))

    def test_admin_credentials_are_required(self) -> None:
        with self.assertRaises(app.ConfigError):
            app.load_settings({k: v for k, v in self.env().items() if "ADMIN" not in k})

    def test_non_http_url_is_rejected(self) -> None:
        with self.assertRaises(app.ConfigError):
            app.load_settings(self.env(KEYCLOAK_URL="file:///etc/passwd"))

    def test_urls_default_from_the_compose_ports(self) -> None:
        settings = app.load_settings(self.env(API_HOST_PORT="8002", WEB_HOST_PORT="3100"))

        self.assertEqual(settings.placeholders["API_URL"], "http://localhost:8002")
        self.assertEqual(settings.placeholders["WEB_URL"], "http://localhost:3100")

    def test_master_realm_is_refused_before_any_request(self) -> None:
        lines: list[str] = []

        code = app.main([], environ=self.env(KEYCLOAK_REALM="master"), out=lines.append)

        self.assertEqual(code, 1)
        self.assertTrue(any("master" in line for line in lines))

    def test_check_cannot_be_combined_with_init_secrets(self) -> None:
        with self.assertRaises(SystemExit):
            app.main(["--check", "--init-secrets"], environ=self.env())

    def test_init_secrets_fills_missing_values_and_keeps_existing_ones(self) -> None:
        lines: list[str] = []
        with tempfile.TemporaryDirectory() as folder:
            env_file = Path(folder) / ".env"
            env_file.write_text(
                f"A=1\r\nKEYCLOAK_PROVISIONER_CLIENT_SECRET={PROVISIONER_SECRET}\r\n"
                "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET=\r\n",
                encoding="utf-8",
                newline="",
            )

            created = app.init_secrets(env_file, app.read_env_file(env_file), lines.append)

            values = app.read_env_file(env_file)
            text = env_file.read_bytes().decode("utf-8")
        self.assertEqual(created, ["KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET"])
        self.assertGreaterEqual(len(values["KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET"]), 32)
        self.assertEqual(values["KEYCLOAK_PROVISIONER_CLIENT_SECRET"], PROVISIONER_SECRET)
        self.assertEqual(values["A"], "1")
        self.assertNotIn("\n", text.replace("\r\n", ""))
        self.assertNotIn(values["KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET"], "\n".join(lines))

    def test_init_secrets_is_a_no_op_when_everything_is_set(self) -> None:
        with tempfile.TemporaryDirectory() as folder:
            env_file = Path(folder) / ".env"

            created = app.init_secrets(env_file, self.env(), lambda _: None)

            self.assertEqual(created, [])
            self.assertFalse(env_file.exists())


class StubKeycloak(ThreadingHTTPServer):
    """A tiny HTTP server that answers like the Keycloak endpoints AdminClient calls."""

    def __init__(self, *, drop_idle_connections: bool = False) -> None:
        super().__init__(("127.0.0.1", 0), StubHandler)
        self.drop_idle_connections = drop_idle_connections
        self.connections = 0
        self.authorizations: list[str | None] = []


class StubHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server: StubKeycloak

    def setup(self) -> None:
        super().setup()
        self.server.connections += 1

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002
        pass

    def reply(self, status: int, payload: Any) -> None:
        raw = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)
        # Closing without announcing it is what an idle timeout looks like to the client.
        self.close_connection = self.server.drop_idle_connections

    def do_POST(self) -> None:
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        self.reply(200, {"access_token": "stub-access", "expires_in": 60})

    def do_GET(self) -> None:
        self.server.authorizations.append(self.headers.get("Authorization"))
        if "/group-by-path/" in self.path:
            self.reply(404, {"error": "Group not found"})
        elif "clientId=forbidden" in self.path:
            self.reply(403, {"errorMessage": "no permission", "secret": "must-not-leak"})
        else:
            self.reply(200, [{"id": "client-1", "clientId": "x"}])


class AdminClientTest(unittest.TestCase):
    def serve(self, **options: Any) -> tuple[StubKeycloak, app.AdminClient]:
        server = StubKeycloak(**options)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(thread.join, 5)
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        settings = app.load_settings(
            {
                **ADMIN_ENV,
                "KEYCLOAK_URL": f"http://127.0.0.1:{server.server_address[1]}",
                "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": AGRO_SECRET,
                "KEYCLOAK_PROVISIONER_CLIENT_SECRET": PROVISIONER_SECRET,
            }
        )
        client = app.AdminClient(settings, "inceres")
        self.addCleanup(client.close)
        return server, client

    @staticmethod
    def found_id(client: app.AdminClient, client_id: str) -> str:
        found = client.find_client(client_id)
        assert found is not None
        return str(found["id"])

    def test_one_connection_serves_every_request_and_carries_the_bearer_token(self) -> None:
        server, client = self.serve()

        self.assertEqual(self.found_id(client, "x"), "client-1")
        self.assertEqual(self.found_id(client, "x"), "client-1")

        self.assertEqual(server.connections, 1)
        self.assertEqual(server.authorizations, ["Bearer stub-access"] * 2)

    def test_it_reconnects_when_the_server_drops_the_idle_connection(self) -> None:
        server, client = self.serve(drop_idle_connections=True)

        self.assertEqual(self.found_id(client, "x"), "client-1")
        self.assertEqual(self.found_id(client, "x"), "client-1")

        self.assertGreaterEqual(server.connections, 2)

    def test_a_missing_group_is_none_not_an_error(self) -> None:
        _, client = self.serve()

        self.assertIsNone(client.find_group("/agentes-agro/absent"))

    def test_errors_report_the_status_and_the_message_only(self) -> None:
        _, client = self.serve()

        with self.assertRaises(app.KeycloakError) as caught:
            client.find_client("forbidden")

        self.assertIn("HTTP 403", str(caught.exception))
        self.assertIn("no permission", str(caught.exception))
        self.assertNotIn("must-not-leak", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
