"""Tests for the read-only audit, the development seed and the token verification."""

import itertools
import unittest
from collections import defaultdict
from typing import Any

import apply_product as app
import audit
import seed_dev_users as seed
import verify_realm as verify

from tests.test_apply_product import OPENAPI, PLACEHOLDERS, SPEC, FakeKeycloak
from tests.test_apply_product import reconcile as run_reconcile

OUT_SECRET_MARKERS = ("password", "secret")


class AuditTest(unittest.TestCase):
    def setUp(self) -> None:
        self.fake = FakeKeycloak()
        run_reconcile(self.fake)
        client = self.fake.find_client("agentes-agro")
        assert client is not None
        self.product = str(client["id"])
        state = app.load_desired_state(SPEC, PLACEHOLDERS)
        self.role_names = [role.name for role in state.roles]
        self.groups = []
        for spec in state.groups:
            if spec.role:
                group = self.fake.find_group(spec.path)
                assert group is not None
                self.groups.append(audit.RoleGroup(spec.path, str(group["id"]), spec.role))

    def person(self, user_id: str, **fields: Any) -> dict[str, Any]:
        return {
            "id": user_id,
            "enabled": True,
            "emailVerified": True,
            "email": "alice@corp.com",
            **fields,
        }

    def run_audit(self) -> tuple[audit.AuditResult, str]:
        lines: list[str] = []
        result = audit.run_audit(
            self.fake, self.groups, self.product, self.role_names, lines.append
        )
        return result, "\n".join(lines)

    def test_a_clean_realm_has_no_violations(self) -> None:
        self.fake.members[self.groups[1].group_id] = [self.person("11111111-aaaa")]

        result, text = self.run_audit()

        self.assertEqual((result.violations, result.warnings), ([], []))
        self.assertIn("1 user(s) in role groups, 0 violation(s)", text)

    def test_a_user_in_two_role_groups_is_a_violation(self) -> None:
        for group in self.groups[:2]:
            self.fake.members[group.group_id] = [self.person("22222222-bbbb")]

        result, _ = self.run_audit()

        self.assertEqual(len(result.violations), 1)
        self.assertIn("22222222", result.violations[0])
        self.assertIn("exactly one", result.violations[0])

    def test_a_product_role_assigned_directly_is_a_violation(self) -> None:
        self.fake.direct_users[(self.product, "admin")] = [self.person("33333333-cccc")]

        result, _ = self.run_audit()

        self.assertEqual(len(result.violations), 1)
        self.assertIn("directly", result.violations[0])

    def test_disabled_and_unverified_members_are_warnings(self) -> None:
        self.fake.members[self.groups[0].group_id] = [
            self.person("44444444-dddd", enabled=False),
            self.person("55555555-eeee", emailVerified=False),
        ]

        result, _ = self.run_audit()

        self.assertEqual(result.violations, [])
        self.assertEqual(len(result.warnings), 2)

    def test_the_output_never_shows_a_full_e_mail_or_a_full_id(self) -> None:
        self.fake.members[self.groups[0].group_id] = [
            self.person("66666666-ffff-4000-8000-000000000000", emailVerified=False)
        ]

        _, text = self.run_audit()

        self.assertNotIn("alice@corp.com", text)
        self.assertIn("a***@c***.com", text)
        self.assertNotIn("66666666-ffff", text)

    def test_the_audit_never_writes(self) -> None:
        writes = self.fake.writes
        self.fake.members[self.groups[0].group_id] = [self.person("77777777-aaaa")]

        self.run_audit()

        self.assertEqual(self.fake.writes, writes)


class FakeDirectory:
    def __init__(self, fake: FakeKeycloak) -> None:
        self._fake = fake
        self._ids = itertools.count(1)
        self.users: dict[str, dict[str, Any]] = {}
        self.passwords: dict[str, str] = {}
        self.membership: dict[str, set[str]] = defaultdict(set)
        self.deleted: list[str] = []

    def find_group(self, path: str) -> dict[str, Any] | None:
        return self._fake.find_group(path)

    def find_user(self, username: str) -> dict[str, Any] | None:
        return next((dict(u) for u in self.users.values() if u["username"] == username), None)

    def create_user(self, representation: dict[str, Any]) -> str:
        user_id = f"user-{next(self._ids)}"
        self.users[user_id] = {"id": user_id, **representation}
        return user_id

    def update_user(self, user_id: str, representation: dict[str, Any]) -> None:
        self.users[user_id] = {**representation, "id": user_id}

    def set_password(self, user_id: str, password: str) -> None:
        self.passwords[user_id] = password

    def list_user_group_ids(self, user_id: str) -> set[str]:
        return set(self.membership[user_id])

    def add_user_to_group(self, user_id: str, group_id: str) -> None:
        self.membership[user_id].add(group_id)

    def remove_user_from_group(self, user_id: str, group_id: str) -> None:
        self.membership[user_id].discard(group_id)

    def delete_user(self, user_id: str) -> None:
        self.deleted.append(self.users.pop(user_id)["username"])


PASSWORDS = {
    "KEYCLOAK_SEED_PASSWORD_SUPER_ADMIN": "Aa1-super-admin",
    "KEYCLOAK_SEED_PASSWORD_ADMIN": "Aa1-admin-pass",
    "KEYCLOAK_SEED_PASSWORD_PLANNING_OPERATOR": "Aa1-operator-pw",
}


class SeedTest(unittest.TestCase):
    def setUp(self) -> None:
        self.fake = FakeKeycloak()
        run_reconcile(self.fake)
        self.directory = FakeDirectory(self.fake)
        self.state = app.load_desired_state(SPEC, PLACEHOLDERS)
        self.lines: list[str] = []

    def seeder(self, env: dict[str, str] | None = None) -> seed.Seeder:
        return seed.Seeder(
            self.directory, self.state, PASSWORDS if env is None else env, self.lines.append
        )

    def test_creates_one_complete_user_per_role_in_its_own_subgroup(self) -> None:
        created = self.seeder().seed()

        self.assertEqual(created, 3)
        for seed_user in seed.SEED_USERS:
            user = self.directory.find_user(seed_user.username)
            assert user is not None
            self.assertTrue(user["emailVerified"])
            self.assertTrue(user["enabled"])
            self.assertTrue(user["firstName"] and user["lastName"])
            self.assertTrue(user["email"].endswith("@agentes-agro.test"))
            path = next(g.path for g in self.state.groups if g.role == seed_user.role)
            group = self.fake.find_group(path)
            assert group is not None
            self.assertEqual(self.directory.membership[user["id"]], {group["id"]})

    def test_running_twice_creates_nothing_and_keeps_the_passwords(self) -> None:
        self.seeder().seed()
        before = dict(self.directory.passwords)
        self.lines.clear()

        created = self.seeder({}).seed()

        self.assertEqual(created, 0)
        self.assertEqual(self.directory.passwords, before)
        self.assertEqual(self.lines, [])

    def test_a_user_in_the_wrong_group_is_moved(self) -> None:
        self.seeder().seed()
        user = self.directory.find_user("agro-dev-admin")
        assert user is not None
        wrong = self.fake.find_group("/agentes-agro/super-admins")
        assert wrong is not None
        self.directory.membership[user["id"]].add(wrong["id"])

        self.seeder({}).seed()

        own = self.fake.find_group("/agentes-agro/admins")
        assert own is not None
        self.assertEqual(self.directory.membership[user["id"]], {own["id"]})

    def test_reset_passwords_sets_them_again(self) -> None:
        self.seeder().seed()
        self.directory.passwords.clear()

        self.seeder().seed(reset_passwords=True)

        self.assertEqual(len(self.directory.passwords), 3)

    def test_a_missing_or_short_password_is_refused_before_creating_anything(self) -> None:
        env = {**PASSWORDS, "KEYCLOAK_SEED_PASSWORD_ADMIN": "short"}

        with self.assertRaises(app.ConfigError) as caught:
            self.seeder(env).seed()

        self.assertIn("KEYCLOAK_SEED_PASSWORD_ADMIN", str(caught.exception))
        self.assertNotIn("short", "\n".join(self.lines))

    def test_the_output_never_contains_a_password(self) -> None:
        self.seeder().seed(reset_passwords=True)

        for password in PASSWORDS.values():
            self.assertNotIn(password, "\n".join(self.lines))

    def test_a_missing_group_asks_for_make_keycloak(self) -> None:
        empty = FakeKeycloak()
        seeder = seed.Seeder(FakeDirectory(empty), self.state, PASSWORDS, self.lines.append)

        with self.assertRaises(app.ConfigError) as caught:
            seeder.seed()

        self.assertIn("make keycloak", str(caught.exception))

    def test_cleanup_deletes_only_the_seed_users(self) -> None:
        self.seeder().seed()
        self.directory.create_user({"username": "someone-else", "email": "x@corp.com"})
        impostor = self.directory.find_user("agro-dev-admin")
        assert impostor is not None
        self.directory.users[impostor["id"]]["email"] = "real.person@corp.com"

        removed = self.seeder({}).cleanup()

        self.assertEqual(removed, 2)
        self.assertNotIn("agro-dev-admin", self.directory.deleted)
        self.assertIsNotNone(self.directory.find_user("someone-else"))
        self.assertIsNotNone(self.directory.find_user("agro-dev-admin"))

    def test_only_localhost_is_accepted(self) -> None:
        for url in ("http://localhost:8180", "http://127.0.0.1:8180", "http://[::1]:8180"):
            seed.ensure_local(url)
        for url in ("https://sso.example.com", "http://keycloak:8080", "http://10.0.0.5:8180"):
            with self.subTest(url), self.assertRaises(app.ConfigError):
                seed.ensure_local(url)

    def test_main_refuses_a_remote_host_before_any_request(self) -> None:
        env = {
            "KEYCLOAK_URL": "https://sso.example.com",
            "KEYCLOAK_ADMIN_USER": "admin",
            "KEYCLOAK_ADMIN_PASSWORD": "pw",
            "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": "a" * 64,
            "KEYCLOAK_PROVISIONER_CLIENT_SECRET": "b" * 64,
        }

        code = seed.main([], environ=env, out=self.lines.append)

        self.assertEqual(code, 1)
        self.assertIn("only localhost", self.lines[0])


class FakeRealm:
    """Answers like Keycloak's example-token endpoint for a user's group-derived roles."""

    def __init__(self, contract: dict[str, set[str]]) -> None:
        self.contract = contract
        self.tokens: dict[str, dict[str, Any]] = {}
        self.users = {
            seed_user.username: {"id": f"id-{seed_user.role}"} for seed_user in seed.SEED_USERS
        }

    def find_client(self, client_id: str) -> dict[str, Any] | None:
        return {"id": f"uuid-{client_id}"}

    def find_user(self, username: str) -> dict[str, Any] | None:
        return self.users.get(username)

    def service_account_user_id(self, client_uuid: str) -> str:
        return "id-service-account"

    def example_access_token(self, client_uuid: str, user_id: str) -> dict[str, Any]:
        if user_id in self.tokens:
            return self.tokens[user_id]
        role = user_id.removeprefix("id-")
        roles = sorted({role} | self.contract.get(role, set())) if role in self.contract else []
        resource_access = {"agentes-agro": {"roles": roles}} if roles else {}
        return {
            "sub": user_id,
            "email": "x@agentes-agro.test",
            "email_verified": True,
            "aud": "agentes-agro",
            "azp": "agentes-agro",
            "resource_access": resource_access,
        }


class VerifyTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = verify.load_contract(OPENAPI)
        self.realm = FakeRealm(self.contract)

    def problems(self) -> list[str]:
        return verify.verify(
            self.realm, "agentes-agro", self.contract, "agentes-agro-provisioner", lambda _: None
        )

    def test_the_contract_lists_the_three_profiles(self) -> None:
        self.assertEqual(set(self.contract), {"super_admin", "admin", "planning_operator"})

    def test_a_matching_realm_has_no_problems(self) -> None:
        self.assertEqual(self.problems(), [])

    def test_a_missing_permission_is_reported(self) -> None:
        token = self.realm.example_access_token("u", "id-admin")
        token["resource_access"]["agentes-agro"]["roles"].remove("users.manage")
        self.realm.tokens["id-admin"] = token

        found = self.problems()

        self.assertEqual(len(found), 1)
        self.assertIn("users.manage", found[0])

    def test_realm_roles_and_other_client_roles_are_reported(self) -> None:
        token = self.realm.example_access_token("u", "id-planning_operator")
        token["realm_access"] = {"roles": ["offline_access"]}
        token["resource_access"]["realm-management"] = {"roles": ["realm-admin"]}
        self.realm.tokens["id-planning_operator"] = token

        found = " ".join(self.problems())

        self.assertIn("realm_access", found)
        self.assertIn("realm-management", found)

    def test_a_wrong_audience_or_authorized_party_is_reported(self) -> None:
        token = self.realm.example_access_token("u", "id-super_admin")
        token["aud"] = ["account"]
        token["azp"] = "other"
        self.realm.tokens["id-super_admin"] = token

        found = " ".join(self.problems())

        self.assertIn("aud", found)
        self.assertIn("azp", found)

    def test_a_user_in_no_group_must_not_receive_product_roles(self) -> None:
        token = self.realm.example_access_token("u", "id-service-account")
        token["resource_access"] = {"agentes-agro": {"roles": ["planner.use"]}}
        self.realm.tokens["id-service-account"] = token

        found = self.problems()

        self.assertEqual(len(found), 1)
        self.assertTrue(found[0].startswith("user in no group"))

    def test_a_missing_seed_user_points_to_make_keycloak_seed(self) -> None:
        del self.realm.users["agro-dev-admin"]

        found = self.problems()

        self.assertEqual(len(found), 1)
        self.assertIn("make keycloak-seed", found[0])


if __name__ == "__main__":
    unittest.main()
