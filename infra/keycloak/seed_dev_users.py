"""Seed three development users, one per product role, in the local Keycloak.

    python infra/keycloak/seed_dev_users.py                    # create or complete the users
    python infra/keycloak/seed_dev_users.py --reset-passwords  # set the .env passwords again
    python infra/keycloak/seed_dev_users.py --cleanup          # delete only what this script made

Passwords come from ``KEYCLOAK_SEED_PASSWORD_*`` in the ``.env`` and never live in the
repository. The script refuses any host that is not the local machine, is idempotent and only
ever touches the users listed below (and only while their e-mail is on the ``.test`` domain).
"""

import argparse
import os
import sys
import urllib.parse
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

import apply_product as app

LOCAL_HOSTS = frozenset({"localhost", "127.0.0.1", "::1"})
SEED_EMAIL_DOMAIN = "agentes-agro.test"
MIN_PASSWORD_LENGTH = 8


@dataclass(frozen=True)
class SeedUser:
    username: str
    role: str
    first_name: str
    last_name: str
    password_variable: str

    @property
    def email(self) -> str:
        return f"{self.username}@{SEED_EMAIL_DOMAIN}"


SEED_USERS = (
    SeedUser(
        "agro-dev-super-admin",
        "super_admin",
        "Dev",
        "Super Admin",
        "KEYCLOAK_SEED_PASSWORD_SUPER_ADMIN",
    ),
    SeedUser("agro-dev-admin", "admin", "Dev", "Admin", "KEYCLOAK_SEED_PASSWORD_ADMIN"),
    SeedUser(
        "agro-dev-planning-operator",
        "planning_operator",
        "Dev",
        "Operador",
        "KEYCLOAK_SEED_PASSWORD_PLANNING_OPERATOR",
    ),
)


class UserDirectory(Protocol):
    """What the seed needs from Keycloak (``RestUserDirectory`` and the test fake)."""

    def find_group(self, path: str) -> dict[str, Any] | None: ...
    def find_user(self, username: str) -> dict[str, Any] | None: ...
    def create_user(self, representation: dict[str, Any]) -> str: ...
    def update_user(self, user_id: str, representation: dict[str, Any]) -> None: ...
    def set_password(self, user_id: str, password: str) -> None: ...
    def list_user_group_ids(self, user_id: str) -> set[str]: ...
    def add_user_to_group(self, user_id: str, group_id: str) -> None: ...
    def remove_user_from_group(self, user_id: str, group_id: str) -> None: ...
    def delete_user(self, user_id: str) -> None: ...


class RestUserDirectory:
    def __init__(self, client: app.AdminClient) -> None:
        self._client = client

    def find_group(self, path: str) -> dict[str, Any] | None:
        return self._client.find_group(path)

    def find_user(self, username: str) -> dict[str, Any] | None:
        return find_user(self._client, username)

    def create_user(self, representation: dict[str, Any]) -> str:
        _, _, headers = self._client.request("POST", "/users", representation, ok=(201,))
        location = headers.get("location", "")
        if not location:
            raise app.KeycloakError("POST", "/users", 201, "no Location header in the response")
        return location.rstrip("/").rsplit("/", 1)[-1]

    def update_user(self, user_id: str, representation: dict[str, Any]) -> None:
        self._client.request("PUT", f"/users/{user_id}", representation, ok=(204,))

    def set_password(self, user_id: str, password: str) -> None:
        body = {"type": "password", "value": password, "temporary": False}
        self._client.request("PUT", f"/users/{user_id}/reset-password", body, ok=(204,))

    def list_user_group_ids(self, user_id: str) -> set[str]:
        groups = self._client.request("GET", f"/users/{user_id}/groups")[1]
        return {group["id"] for group in groups}

    def add_user_to_group(self, user_id: str, group_id: str) -> None:
        self._client.request("PUT", f"/users/{user_id}/groups/{group_id}", ok=(204,))

    def remove_user_from_group(self, user_id: str, group_id: str) -> None:
        self._client.request("DELETE", f"/users/{user_id}/groups/{group_id}", ok=(204,))

    def delete_user(self, user_id: str) -> None:
        self._client.request("DELETE", f"/users/{user_id}", ok=(204,))


def find_user(client: app.AdminClient, username: str) -> dict[str, Any] | None:
    """The user with exactly this username, if any."""
    path = f"/users?exact=true&username={urllib.parse.quote(username, safe='')}"
    users = client.request("GET", path)[1]
    return dict(users[0]) if users else None


def ensure_local(base_url: str) -> None:
    host = (urllib.parse.urlsplit(base_url).hostname or "").lower()
    if host not in LOCAL_HOSTS:
        raise app.ConfigError(
            f"refusing to seed development users on {host!r}: only localhost is allowed"
        )


class Seeder:
    def __init__(
        self,
        directory: UserDirectory,
        state: app.DesiredState,
        env: Mapping[str, str],
        out: Callable[[str], None] = print,
    ) -> None:
        self._dir = directory
        self._group_paths = {g.role: g.path for g in state.groups if g.role}
        self._env = env
        self._out = out

    def seed(self, *, reset_passwords: bool = False) -> int:
        """Create or complete the users; returns how many were created."""
        created = 0
        role_group_ids = self._role_group_ids()
        for seed_user in SEED_USERS:
            user = self._dir.find_user(seed_user.username)
            if user is None:
                password = self._password(seed_user)
                user_id = self._dir.create_user(self._representation(seed_user))
                self._dir.set_password(user_id, password)
                created += 1
                self._out(f"created user {seed_user.username}")
            else:
                user_id = user["id"]
                self._complete(seed_user, user)
                if reset_passwords:
                    self._dir.set_password(user_id, self._password(seed_user))
                    self._out(f"password of {seed_user.username} reset")
            self._place_in_group(seed_user, user_id, role_group_ids)
        return created

    def cleanup(self) -> int:
        """Delete the seed users, and only them: the username and the .test e-mail must match."""
        removed = 0
        for seed_user in SEED_USERS:
            user = self._dir.find_user(seed_user.username)
            if user is None:
                continue
            if user.get("email") != seed_user.email:
                self._out(f"warning: keeping {seed_user.username}: it was not created by the seed")
                continue
            self._dir.delete_user(user["id"])
            removed += 1
            self._out(f"deleted user {seed_user.username}")
        return removed

    def _role_group_ids(self) -> dict[str, str]:
        ids: dict[str, str] = {}
        for role, path in self._group_paths.items():
            group = self._dir.find_group(path)
            if group is None:
                raise app.ConfigError(f"group {path} does not exist; run `make keycloak` first")
            ids[role] = str(group["id"])
        return ids

    def _password(self, seed_user: SeedUser) -> str:
        value = self._env.get(seed_user.password_variable, "")
        if len(value) < MIN_PASSWORD_LENGTH:
            raise app.ConfigError(
                f"{seed_user.password_variable} must have at least {MIN_PASSWORD_LENGTH} characters"
            )
        return value

    @staticmethod
    def _representation(seed_user: SeedUser) -> dict[str, Any]:
        return {
            "username": seed_user.username,
            "email": seed_user.email,
            "firstName": seed_user.first_name,
            "lastName": seed_user.last_name,
            "enabled": True,
            "emailVerified": True,
            "requiredActions": [],
        }

    def _complete(self, seed_user: SeedUser, current: Mapping[str, Any]) -> None:
        wanted = self._representation(seed_user)
        differs = any(
            current.get(key) != value for key, value in wanted.items() if key != "requiredActions"
        )
        if differs:
            self._dir.update_user(current["id"], {**current, **wanted})
            self._out(f"updated user {seed_user.username}")

    def _place_in_group(
        self, seed_user: SeedUser, user_id: str, role_group_ids: Mapping[str, str]
    ) -> None:
        member_of = self._dir.list_user_group_ids(user_id)
        own = role_group_ids[seed_user.role]
        for group_id in sorted((set(role_group_ids.values()) & member_of) - {own}):
            self._dir.remove_user_from_group(user_id, group_id)
            self._out(f"removed {seed_user.username} from a group that is not its own")
        if own not in member_of:
            self._dir.add_user_to_group(user_id, own)
            self._out(f"added {seed_user.username} to {self._group_paths[seed_user.role]}")


def main(
    argv: Sequence[str] | None = None,
    environ: Mapping[str, str] | None = None,
    out: Callable[[str], None] = print,
) -> int:
    parser = argparse.ArgumentParser(description="Seed the Agentes Agro development users.")
    parser.add_argument("--reset-passwords", action="store_true", help="set the passwords again")
    parser.add_argument("--cleanup", action="store_true", help="delete the seed users")
    parser.add_argument("--spec", type=Path, default=app.DEFAULT_SPEC)
    parser.add_argument("--env-file", type=Path, default=app.DEFAULT_ENV_FILE)
    args = parser.parse_args(argv)
    if args.cleanup and args.reset_passwords:
        parser.error("--cleanup deletes the users, so --reset-passwords makes no sense with it")

    env = {**app.read_env_file(args.env_file), **(os.environ if environ is None else environ)}
    try:
        settings = app.load_settings(env)
        ensure_local(settings.base_url)
        state = app.load_desired_state(args.spec, settings.placeholders)
        realm = settings.realm or state.realm
        if realm == app.FORBIDDEN_REALM:
            raise app.ConfigError(f"refusing to change the {app.FORBIDDEN_REALM!r} realm")
        client = app.AdminClient(settings, realm)
        try:
            seeder = Seeder(RestUserDirectory(client), state, env, out)
            if args.cleanup:
                out(f"{seeder.cleanup()} user(s) deleted")
            else:
                created = seeder.seed(reset_passwords=args.reset_passwords)
                out(f"{created} user(s) created, {len(SEED_USERS) - created} already existed")
        finally:
            client.close()
    except (app.ConfigError, app.KeycloakError) as err:
        out(f"error: {err}")
        return 1
    except OSError as err:
        out(f"error: cannot reach Keycloak: {err}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
