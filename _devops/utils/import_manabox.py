#!/usr/bin/env python3
"""
Import a ManaBox collection export (CSV) into the Tamiyo API.

For each row:
  - Gets or creates the Storage matching "Binder Name" (type = "Binder Type").
  - If "Binder Type" is "deck", also gets or creates a Deck with the same name.
  - Creates one Card per physical copy ("Quantity"), attached to the storage.
  - If the row belongs to a deck, links each created card to that deck.

Tamiyo is multi-tenant: every endpoint except /health, /auth/register and
/auth/login requires a Bearer token. This script takes an account's email
and password, uses them to register a new account (or log in if that
account already exists), and sends the resulting JWT on every request it
makes. Everything it creates ends up scoped to that one account.

Usage:
    pip install requests
    python3 import_manabox.py path/to/ManaBox_Collection.csv --email you@example.com --password "at-least-8-chars"
    python3 import_manabox.py path/to/ManaBox_Collection.csv --email you@example.com --password "..." --dry-run
    TAMIYO_API_URL=http://localhost:8080 python3 import_manabox.py collection.csv --email you@example.com --password "..."

    # Omit --password to be prompted for it interactively (not echoed, not left in shell history):
    python3 import_manabox.py path/to/ManaBox_Collection.csv --email you@example.com
"""

import argparse
import csv
import getpass
import os
import sys

import requests

DEFAULT_API_URL = "http://localhost:8080"
# The CSV has no "format" column for decks — adjust here if needed.
DEFAULT_DECK_FORMAT = "commander"


class TamiyoClient:
    def __init__(self, base_url: str, dry_run: bool = False):
        self.base_url = base_url.rstrip("/")
        self.dry_run = dry_run
        self.session = requests.Session()

    def authenticate(self, email: str, password: str) -> None:
        """
        Get a JWT for (email, password) and attach it to every subsequent
        request on this session.

        Tries to register the account first; if it already exists (409),
        falls back to logging in with the same credentials. This lets the
        same command be re-run safely without having to remember whether
        the account was already created on a previous run.
        """
        if self.dry_run:
            print(f"[dry-run] would authenticate as {email}")
            return

        token = None

        resp = self.session.post(
            f"{self.base_url}/auth/register",
            json={"email": email, "password": password},
        )
        if resp.status_code == 201:
            token = resp.json()["token"]
            print(f"Registered new account for {email}.")
        elif resp.status_code == 409:
            # Account already exists: fall back to logging in.
            resp = self.session.post(
                f"{self.base_url}/auth/login",
                json={"email": email, "password": password},
            )
            if resp.status_code == 200:
                token = resp.json()["token"]
                print(f"Account {email} already exists, logged in instead.")
            else:
                print(
                    f"ERROR: could not log in as {email} "
                    f"(account already exists, but login failed with "
                    f"{resp.status_code}: {resp.text}).",
                    file=sys.stderr,
                )
                sys.exit(1)
        else:
            print(
                f"ERROR: could not register {email} "
                f"({resp.status_code}: {resp.text}).",
                file=sys.stderr,
            )
            sys.exit(1)

        self.session.headers["Authorization"] = f"Bearer {token}"

    def _post(self, path: str, json: dict) -> dict:
        if self.dry_run:
            return {"id": -1, **json}
        resp = self.session.post(f"{self.base_url}{path}", json=json)
        resp.raise_for_status()
        return resp.json()

    def _put(self, path: str) -> None:
        if self.dry_run:
            return
        resp = self.session.put(f"{self.base_url}{path}")
        resp.raise_for_status()

    def list_storages(self) -> list[dict]:
        resp = self.session.get(f"{self.base_url}/storage")
        resp.raise_for_status()
        return resp.json()

    def create_storage(self, name: str, storage_type: str) -> dict:
        return self._post("/storage", {"name": name, "type": storage_type})

    def list_decks(self) -> list[dict]:
        resp = self.session.get(f"{self.base_url}/deck")
        resp.raise_for_status()
        return resp.json()

    def create_deck(self, name: str, deck_format: str) -> dict:
        return self._post("/deck", {"name": name, "format": deck_format})

    def create_card(
        self,
        name: str,
        scryfall_id: str,
        set_code: str,
        collector_number: str,
        foil: bool,
        storage_id: int,
    ) -> dict:
        return self._post(
            "/cards",
            {
                "name": name,
                "scryfall_id": scryfall_id,
                "set_code": set_code,
                "collector_number": collector_number,
                "foil": foil,
                "storage_id": storage_id,
            },
        )

    def link_card_to_deck(self, deck_id: int, card_id: int) -> None:
        self._put(f"/deck/{deck_id}/cards/{card_id}")


def get_or_create_storage(client: TamiyoClient, cache: dict, name: str, storage_type: str) -> int:
    if name in cache:
        return cache[name]
    created = client.create_storage(name, storage_type)
    cache[name] = created["id"]
    return created["id"]


def get_or_create_deck(client: TamiyoClient, cache: dict, name: str) -> int:
    if name in cache:
        return cache[name]
    created = client.create_deck(name, DEFAULT_DECK_FORMAT)
    cache[name] = created["id"]
    return created["id"]


def main():
    parser = argparse.ArgumentParser(description="Import a ManaBox CSV export into Tamiyo.")
    parser.add_argument("csv_path", help="Path to the ManaBox_Collection.csv file")
    parser.add_argument(
        "--api-url",
        default=os.environ.get("TAMIYO_API_URL", DEFAULT_API_URL),
        help=f"Tamiyo API base URL (default: {DEFAULT_API_URL})",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Parse the CSV and print what would happen, without calling the API.",
    )
    parser.add_argument(
        "--email",
        required=True,
        help="Email of the Tamiyo account to import into. "
        "Registers this account if it doesn't exist yet, otherwise logs in.",
    )
    parser.add_argument(
        "--password",
        default=None,
        help="Password for --email. If omitted, you'll be prompted for it "
        "(recommended, so it doesn't end up in your shell history).",
    )
    args = parser.parse_args()

    password = args.password
    if password is None and not args.dry_run:
        password = getpass.getpass(f"Password for {args.email}: ")
    elif password is None:
        password = "dry-run-placeholder"

    client = TamiyoClient(args.api_url, dry_run=args.dry_run)
    client.authenticate(args.email, password)

    print(f"Loading existing storages/decks from {args.api_url} ...")
    storage_cache = {s["name"]: s["id"] for s in client.list_storages()} if not args.dry_run else {}
    deck_cache = {d["name"]: d["id"] for d in client.list_decks()} if not args.dry_run else {}

    cards_created = 0
    cards_skipped = 0
    storages_created_before = len(storage_cache)
    decks_created_before = len(deck_cache)

    with open(args.csv_path, newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)

        for i, row in enumerate(reader, start=2):  # start=2: header is line 1
            binder_name = row["Binder Name"]
            binder_type = row["Binder Type"]  # "binder" or "deck"
            card_name = row["Name"]
            set_code = row["Set code"]
            scryfall_id = row["Scryfall ID"]
            collector_number = row["Collector number"]
            foil = row["Foil"] == "foil"
            quantity = int(row["Quantity"])

            storage_id = get_or_create_storage(client, storage_cache, binder_name, binder_type)

            deck_id = None
            if binder_type == "deck":
                deck_id = get_or_create_deck(client, deck_cache, binder_name)

            for _ in range(quantity):
                try:
                    card = client.create_card(
                        name=card_name,
                        scryfall_id=scryfall_id,
                        set_code=set_code,
                        collector_number=collector_number,
                        foil=foil,
                        storage_id=storage_id,
                    )
                    cards_created += 1

                    if deck_id is not None:
                        client.link_card_to_deck(deck_id, card["id"])

                except requests.HTTPError as e:
                    print(f"[line {i}] ERROR creating '{card_name}': {e}", file=sys.stderr)
                    cards_skipped += 1

            if cards_created % 200 == 0 and cards_created > 0:
                print(f"... {cards_created} cards created so far")

    print()
    print("=== Import summary ===")
    print(f"Mode:              {'DRY RUN (nothing sent)' if args.dry_run else 'LIVE'}")
    print(f"Cards created:     {cards_created}")
    print(f"Cards skipped:     {cards_skipped}")
    print(f"Storages created:  {len(storage_cache) - storages_created_before}")
    print(f"Decks created:     {len(deck_cache) - decks_created_before}")


if __name__ == "__main__":
    main()
