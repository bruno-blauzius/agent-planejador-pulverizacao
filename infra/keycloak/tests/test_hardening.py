"""Tests for the privilege, client, realm and file hardening of the apply script."""

import os
import tempfile
import unittest
from pathlib import Path
from typing import Any
from unittest import mock

import apply_product as app
import realm_checks

from tests.test_apply_product import ADMIN_ENV, AGRO_SECRET, PROVISIONER_SECRET, FakeKeycloak
from tests.test_apply_product import reconcile as run_reconcile


class HardeningBase(unittest.TestCase):
    def setUp(self) -> None:
        self.fake = FakeKeycloak()
        run_reconcile(self.fake)
        self.product = self.uuid("agentes-agro")
        self.provisioner = self.uuid("agentes-agro-provisioner")

    def uuid(self, client_id: str) -> str:
        client = self.fake.find_client(client_id)
        assert client is not None
        return str(client["id"])

    def group_id(self, path: str) -> str:
        group = self.fake.find_group(path)
        assert group is not None
        return str(group["id"])


class PrivilegeBlindSpotTest(HardeningBase):
    """Anything added by hand to a product group or role, from anywhere, is revoked (A1)."""

    def test_realm_admin_added_to_a_group_is_revoked(self) -> None:
        admins = self.group_id("/agentes-agro/admins")
        self.fake.realm_group_mappings[admins]["realm-admin"] = {"id": "r1", "name": "realm-admin"}

        reconciler, lines = run_reconcile(self.fake)

        self.assertEqual(self.fake.realm_group_mappings[admins], {})
        self.assertEqual(reconciler.changes, 1)
        self.assertIn("realm/realm-admin", lines[0])

    def test_role_of_another_client_on_a_group_is_revoked(self) -> None:
        admins = self.group_id("/agentes-agro/admins")
        management = self.fake.realm_management
        self.fake.mappings[("groups", admins, management)].add("manage-clients")

        run_reconcile(self.fake)

        self.assertEqual(self.fake.mappings[("groups", admins, management)], set())
        self.assertEqual(self.fake.mappings[("groups", admins, self.product)], {"admin"})

    def test_a_group_with_no_role_holds_nothing(self) -> None:
        root = self.group_id("/agentes-agro")
        self.fake.mappings[("groups", root, self.product)].add("super_admin")

        run_reconcile(self.fake)

        self.assertEqual(self.fake.mappings[("groups", root, self.product)], set())

    def test_realm_admin_composite_on_a_profile_role_is_removed(self) -> None:
        self._inject_composite("admin", "realm-admin")

        reconciler, lines = run_reconcile(self.fake)

        self.assertEqual(self.fake.foreign_composites[(self.product, "admin")], [])
        self.assertEqual(reconciler.changes, 1)
        self.assertIn("realm-management/realm-admin", lines[0])

    def test_foreign_composite_on_a_permission_role_is_removed(self) -> None:
        self._inject_composite("planner.use", "manage-clients")

        run_reconcile(self.fake)

        self.assertEqual(self.fake.foreign_composites[(self.product, "planner.use")], [])
        self.assertEqual(
            self.fake.composites[(self.product, "admin")],
            set(
                next(
                    r
                    for r in app.load_desired_state(app.DEFAULT_SPEC, _placeholders()).roles
                    if r.name == "admin"
                ).composites
            ),
        )

    def test_check_reports_the_privilege_without_revoking_it(self) -> None:
        admins = self.group_id("/agentes-agro/admins")
        self.fake.realm_group_mappings[admins]["realm-admin"] = {"id": "r1", "name": "realm-admin"}
        writes = self.fake.writes

        reconciler, _ = run_reconcile(self.fake, apply_changes=False)

        self.assertEqual(reconciler.changes, 1)
        self.assertEqual(self.fake.writes, writes)

    def _inject_composite(self, role: str, foreign_role: str) -> None:
        management = self.fake.realm_management
        rep = {
            "id": f"foreign-{foreign_role}",
            "name": foreign_role,
            "clientRole": True,
            "containerId": management,
        }
        self.fake.foreign_composites[(self.product, role)].append(
            app.RoleRef(management, "realm-management", foreign_role, rep)
        )


def _placeholders() -> dict[str, str]:
    return {
        "API_URL": "http://localhost:8080",
        "API_INTERNAL_URL": "http://agro-api:8080",
        "WEB_URL": "http://localhost:3000",
        "AGENTES_AGRO_CLIENT_SECRET": AGRO_SECRET,
        "PROVISIONER_CLIENT_SECRET": PROVISIONER_SECRET,
    }


class ClientHardeningTest(HardeningBase):
    def test_product_mapper_emits_only_the_product_roles_in_the_access_token(self) -> None:
        mappers = {m["name"]: m for m in self.fake.mappers[self.product]}

        config = mappers["client-roles-agentes-agro"]["config"]
        self.assertEqual(
            mappers["client-roles-agentes-agro"]["protocolMapper"],
            "oidc-usermodel-client-role-mapper",
        )
        self.assertEqual(config["usermodel.clientRoleMapping.clientId"], "agentes-agro")
        self.assertEqual(config["claim.name"], "resource_access.agentes-agro.roles")
        self.assertEqual(config["multivalued"], "true")
        self.assertEqual(config["access.token.claim"], "true")
        self.assertEqual(config["id.token.claim"], "false")
        self.assertEqual(config["userinfo.token.claim"], "false")

    def test_an_undeclared_mapper_is_removed_from_a_product_client(self) -> None:
        self.fake.mappers[self.product].append(
            {
                "id": "m-extra",
                "name": "realm-roles-leak",
                "protocolMapper": "oidc-usermodel-realm-role-mapper",
            }
        )

        reconciler, lines = run_reconcile(self.fake)

        self.assertNotIn("realm-roles-leak", {m["name"] for m in self.fake.mappers[self.product]})
        self.assertEqual(reconciler.changes, 1)
        self.assertIn("realm-roles-leak", lines[0])

    def test_scopes_that_are_not_declared_are_removed_from_both_lists(self) -> None:
        self.fake.default_scopes[self.product].update({"web-origins", "roles"})
        self.fake.optional_scopes[self.product].update({"address", "offline_access"})
        self.fake.default_scopes[self.provisioner].add("profile")

        run_reconcile(self.fake)

        self.assertEqual(
            self.fake.default_scopes[self.product], {"acr", "profile", "basic", "email"}
        )
        self.assertEqual(self.fake.optional_scopes[self.product], set())
        self.assertEqual(
            self.fake.default_scopes[self.provisioner], {"service_account", "basic", "kc-admin-api"}
        )

    def test_a_scope_that_moved_to_the_wrong_list_comes_back(self) -> None:
        self.fake.default_scopes[self.product].remove("email")
        self.fake.optional_scopes[self.product].add("email")

        run_reconcile(self.fake)

        self.assertIn("email", self.fake.default_scopes[self.product])
        self.assertNotIn("email", self.fake.optional_scopes[self.product])

    def test_a_scope_in_both_lists_of_the_spec_is_rejected(self) -> None:
        import json

        data = json.loads(app.DEFAULT_SPEC.read_text(encoding="utf-8"))
        data["clientScopes"][0]["optional"] = ["email"]
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "spec.json"
            path.write_text(json.dumps(data), encoding="utf-8")
            with self.assertRaises(app.ConfigError):
                app.load_desired_state(path, _placeholders())

    def test_the_provisioner_token_is_short_lived(self) -> None:
        attributes = self.fake.clients[self.provisioner]["attributes"]

        self.assertEqual(attributes["access.token.lifespan"], "120")
        self.assertEqual(attributes["client_credentials.use_refresh_token"], "false")

    def test_both_clients_carry_the_security_baseline(self) -> None:
        for client_uuid in (self.product, self.provisioner):
            attributes = self.fake.clients[client_uuid]["attributes"]
            for name in (
                "oauth2.device.authorization.grant.enabled",
                "oidc.ciba.grant.enabled",
                "standard.token.exchange.enabled",
                "require.pushed.authorization.requests",
            ):
                self.assertEqual(attributes[name], "false", name)
            self.assertEqual(attributes["access.token.signed.response.alg"], "RS256")
        self.assertEqual(
            self.fake.clients[self.product]["attributes"]["use.refresh.tokens"], "true"
        )

    def test_baseline_drift_is_corrected(self) -> None:
        attributes = self.fake.clients[self.product]["attributes"]
        attributes["oauth2.device.authorization.grant.enabled"] = "true"
        attributes["use.refresh.tokens"] = "false"

        reconciler, _ = run_reconcile(self.fake)

        attributes = self.fake.clients[self.product]["attributes"]
        self.assertEqual(attributes["oauth2.device.authorization.grant.enabled"], "false")
        self.assertEqual(attributes["use.refresh.tokens"], "true")
        self.assertEqual(reconciler.changes, 1)

    def test_an_undeclared_known_security_attribute_is_drift(self) -> None:
        self.fake.clients[self.product]["attributes"]["dpop.bound.access.tokens"] = "true"

        reconciler, _ = run_reconcile(self.fake)

        self.assertEqual(
            self.fake.clients[self.product]["attributes"]["dpop.bound.access.tokens"], "false"
        )
        self.assertEqual(reconciler.changes, 1)

    def test_an_unknown_attribute_is_left_alone(self) -> None:
        self.fake.clients[self.product]["attributes"]["some.other.attribute"] = "x"

        reconciler, _ = run_reconcile(self.fake)

        self.assertEqual(reconciler.changes, 0)

    def test_a_residual_admin_url_is_cleared_and_a_missing_one_is_not_drift(self) -> None:
        self.fake.clients[self.product]["adminUrl"] = "http://localhost:3000"
        run_reconcile(self.fake)
        self.assertEqual(self.fake.clients[self.product]["adminUrl"], "")

        del self.fake.clients[self.product]["adminUrl"]
        reconciler, _ = run_reconcile(self.fake)
        self.assertEqual(reconciler.changes, 0)


class RealmRequirementsTest(unittest.TestCase):
    REQUIREMENTS = app.load_desired_state(app.DEFAULT_SPEC, _placeholders()).realm_requirements

    def evaluate(self, **overrides: Any) -> realm_checks.RealmReport:
        from tests.test_apply_product import COMPLIANT_REALM

        return realm_checks.evaluate({**COMPLIANT_REALM, **overrides}, self.REQUIREMENTS)

    def test_a_compliant_realm_has_no_findings(self) -> None:
        report = self.evaluate()

        self.assertEqual((report.violations, report.warnings), ([], []))

    def test_each_broken_requirement_is_a_violation(self) -> None:
        cases = {
            "registrationAllowed": True,
            "verifyEmail": False,
            "duplicateEmailsAllowed": True,
            "sslRequired": "none",
            "eventsEnabled": False,
            "adminEventsEnabled": False,
            "bruteForceProtected": False,
            "passwordPolicy": "length(6) and digits(1)",
        }
        for name, value in cases.items():
            with self.subTest(name):
                report = self.evaluate(**{name: value})

                self.assertTrue(report.violations, name)
                self.assertTrue(any(name in v for v in report.violations), report.violations)

    def test_ssl_required_all_is_accepted(self) -> None:
        self.assertEqual(self.evaluate(sslRequired="all").violations, [])

    def test_password_policy_needs_length_digits_and_lowercase(self) -> None:
        report = self.evaluate(passwordPolicy="notUsername")

        self.assertEqual(len(report.violations), 3)

    def test_platform_recommendations_are_warnings_not_violations(self) -> None:
        report = self.evaluate(revokeRefreshToken=False, adminPermissionsEnabled=False)

        self.assertEqual(report.violations, [])
        self.assertEqual(len(report.warnings), 2)

    def test_only_check_fails_on_a_violation_and_nothing_is_written(self) -> None:
        report = self.evaluate(registrationAllowed=True)
        lines: list[str] = []

        failed_in_check = app._report_realm(report, True, lines.append)
        failed_in_apply = app._report_realm(report, False, lines.append)

        self.assertTrue(failed_in_check)
        self.assertFalse(failed_in_apply)
        self.assertIn("realm requirement not met", lines[0])
        self.assertIn("warning: realm requirement", lines[1])
        self.assertFalse(hasattr(FakeKeycloak(), "update_realm"))


class TransportSecurityTest(unittest.TestCase):
    def env(self, url: str) -> dict[str, str]:
        return {
            **ADMIN_ENV,
            "KEYCLOAK_URL": url,
            "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": AGRO_SECRET,
            "KEYCLOAK_PROVISIONER_CLIENT_SECRET": PROVISIONER_SECRET,
        }

    def test_clear_text_to_a_remote_host_is_refused(self) -> None:
        for url in (
            "http://keycloak.example.com",
            "http://10.0.0.5:8180",
            "http://localhost.evil.com",
        ):
            with self.subTest(url), self.assertRaises(app.ConfigError) as caught:
                app.load_settings(self.env(url))
            self.assertIn("https", str(caught.exception))

    def test_clear_text_is_fine_on_the_local_machine_and_the_compose_network(self) -> None:
        for url in (
            "http://localhost:8180",
            "http://127.0.0.1:8180",
            "http://[::1]:8180",
            "http://keycloak:8080",
        ):
            with self.subTest(url):
                self.assertEqual(app.load_settings(self.env(url)).base_url, url)

    def test_https_is_fine_anywhere(self) -> None:
        settings = app.load_settings(self.env("https://sso.example.com"))

        self.assertEqual(settings.base_url, "https://sso.example.com")


class EnvFileTest(unittest.TestCase):
    def write(self, folder: str, text: str) -> Path:
        path = Path(folder) / ".env"
        path.write_bytes(text.encode())
        return path

    def test_a_short_line_is_replaced_in_place_without_duplicating_the_name(self) -> None:
        with tempfile.TemporaryDirectory() as folder:
            path = self.write(folder, "A=1\nKEYCLOAK_PROVISIONER_CLIENT_SECRET=short\nB=2\n")

            app.set_env_values(path, {"KEYCLOAK_PROVISIONER_CLIENT_SECRET": "x" * 40})

            lines = path.read_text(encoding="utf-8").splitlines()
        self.assertEqual(lines, ["A=1", "KEYCLOAK_PROVISIONER_CLIENT_SECRET=" + "x" * 40, "B=2"])

    def test_duplicates_of_the_same_name_collapse_into_one(self) -> None:
        with tempfile.TemporaryDirectory() as folder:
            path = self.write(folder, "S=\nA=1\nS=old\n")

            app.set_env_values(path, {"S": "new"})

            self.assertEqual(path.read_text(encoding="utf-8").splitlines(), ["S=new", "A=1"])

    def test_commented_lines_are_not_mistaken_for_the_variable(self) -> None:
        with tempfile.TemporaryDirectory() as folder:
            path = self.write(folder, "#S=commented\nA=1\n")

            app.set_env_values(path, {"S": "new"})

            self.assertEqual(
                path.read_text(encoding="utf-8").splitlines(), ["#S=commented", "A=1", "S=new"]
            )

    def test_the_write_is_atomic_and_leaves_no_temporary_file(self) -> None:
        with tempfile.TemporaryDirectory() as folder:
            path = self.write(folder, "A=1\r\n")

            app.set_env_values(path, {"S": "v"})

            self.assertEqual(path.read_bytes(), b"A=1\r\nS=v\r\n")
            self.assertEqual([p.name for p in Path(folder).iterdir()], [".env"])

    def test_a_failed_replace_keeps_the_original_and_cleans_up(self) -> None:
        with tempfile.TemporaryDirectory() as folder:
            path = self.write(folder, "A=1\n")
            with (
                mock.patch.object(os, "replace", side_effect=OSError("disk full")),
                self.assertRaises(OSError),
            ):
                app.set_env_values(path, {"S": "v"})

            self.assertEqual(path.read_text(encoding="utf-8"), "A=1\n")
            self.assertEqual([p.name for p in Path(folder).iterdir()], [".env"])


class ResponseHandlingTest(unittest.TestCase):
    def client_answering(self, status: int, body: bytes) -> app.AdminClient:
        settings = app.load_settings(
            {
                **ADMIN_ENV,
                "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": AGRO_SECRET,
                "KEYCLOAK_PROVISIONER_CLIENT_SECRET": PROVISIONER_SECRET,
            }
        )

        class Canned(app.AdminClient):
            def _send(
                self, method: str, path: str, body_: bytes | None, headers: dict[str, str]
            ) -> tuple[int, dict[str, str], bytes]:
                if "/token" in path:
                    return 200, {}, b'{"access_token": "t", "expires_in": 60}'
                return status, {}, body

        return Canned(settings, "inceres")

    def test_an_html_page_with_200_is_a_clean_error_not_a_traceback(self) -> None:
        client = self.client_answering(200, b"<html><body>Sign in to your account</body></html>")

        with self.assertRaises(app.KeycloakError) as caught:
            client.find_client("x")

        self.assertIn("not JSON", str(caught.exception))
        self.assertIn("HTTP 200", str(caught.exception))

    def test_an_html_token_response_is_a_clean_error(self) -> None:
        settings = app.load_settings(
            {
                **ADMIN_ENV,
                "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": AGRO_SECRET,
                "KEYCLOAK_PROVISIONER_CLIENT_SECRET": PROVISIONER_SECRET,
            }
        )

        class Html(app.AdminClient):
            def _send(
                self, method: str, path: str, body: bytes | None, headers: dict[str, str]
            ) -> tuple[int, dict[str, str], bytes]:
                return 200, {}, b"<html>captive portal</html>"

        with self.assertRaises(app.KeycloakError):
            Html(settings, "inceres").find_client("x")

    def test_an_empty_204_is_not_parsed(self) -> None:
        client = self.client_answering(204, b"")

        status, data, _ = client.request("PUT", "/x", ok=(204,))

        self.assertEqual((status, data), (204, None))

    def test_secret_comparison_is_by_value(self) -> None:
        self.assertTrue(app._same_secret("a" * 64, "a" * 64))
        self.assertFalse(app._same_secret("a" * 64, "a" * 63 + "b"))
        self.assertFalse(app._same_secret("", "a" * 64))


if __name__ == "__main__":
    unittest.main()
