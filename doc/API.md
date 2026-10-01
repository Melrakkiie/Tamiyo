# Tamiyo API Documentation

Tamiyo is a REST API for managing a Magic: The Gathering card collection — cards, physical storage (binders, boxes, deckboxes), and decks.

**Base URL (local dev):** `http://localhost:8080`
**Content-Type:** `application/json` for all request and response bodies.

---

## Table of Contents

- [Authentication](#authentication)
- [Conventions](#conventions)
- [Cards](#cards)
- [Storage](#storage)
- [Decks](#decks)
- [Deck ↔ Card relationship](#deck--card-relationship)
- [Error reference](#error-reference)

---

## Authentication

Tamiyo is multi-tenant. `/health`, `/auth/register`, and `/auth/login` are public; every other endpoint requires a Bearer token and is scoped to the authenticated account — you only ever see or modify your own cards, storages, and decks.

### `POST /auth/register`

Create an account.

**Body**

| Field | Type | Required | Notes |
|---|---|---|---|
| `email` | string | Yes | Must be a valid email. |
| `password` | string | Yes | Minimum 8 characters. |

**Response `201 Created`**
```json
{ "token": "eyJhbGciOi..." }
```

**Errors:** `400` missing/invalid field · `409` email already registered (`"email already registered"`)

---

### `POST /auth/login`

Exchange credentials for a JWT.

**Body**

| Field | Type | Required |
|---|---|---|
| `email` | string | Yes |
| `password` | string | Yes |

**Response `200 OK`**
```json
{ "token": "eyJhbGciOi..." }
```

**Errors:** `400` missing/invalid field · `401` invalid email or password

> The same error is returned for "no such account" and "wrong password", by design — this prevents an attacker from using the endpoint to discover which emails have an account.

### Using the token

Send it on every other request:
```
Authorization: Bearer <token>
```
Tokens are valid for 7 days. There's no refresh endpoint yet — log in again once it expires.

---

## Conventions

### Timestamps
Every resource carries `added` and `updated` timestamps, generated and maintained by the database — they can never be set or modified through the API.

### Optional relations
Fields like `storage_id` or `commander_id` are nullable. A card doesn't have to belong to a storage, and a deck doesn't have to have a commander.

### Partial updates
All `PATCH` endpoints accept a partial body — only the fields you include are modified. Omitted fields are left untouched.

### Idempotent linking
`PUT /deck/:id/cards/:card_id` and `DELETE /deck/:id/cards/:card_id` are idempotent: calling them multiple times with the same parameters always converges to the same state, without erroring on repeat calls.

---

## Cards

> All endpoints below require `Authorization: Bearer <token>` and only ever operate on the authenticated account's own cards.

### `GET /cards`

List all cards, optionally filtered by storage.

**Query parameters**

| Param | Type | Required | Description |
|---|---|---|---|
| `storage_id` | int | No | Only return cards belonging to this storage. |

**Example**
```
GET /cards?storage_id=1
```

**Response `200 OK`**
```json
[
  {
    "id": 1,
    "name": "Black Lotus",
    "scryfall_id": "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd",
    "set_code": "lea",
    "collector_number": 232,
    "foil": false,
    "storage_id": 1,
    "added": "2026-01-15 10:30:00",
    "updated": "2026-01-15 10:30:00"
  }
]
```

**Errors:** `400` if `storage_id` is not a valid integer.

---

### `GET /cards/:id`

Fetch a single card by ID.

**Response `200 OK`**
```json
{
  "id": 1,
  "name": "Black Lotus",
  "scryfall_id": "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd",
  "set_code": "lea",
  "collector_number": 232,
  "foil": false,
  "storage_id": 1,
  "added": "2026-01-15 10:30:00",
  "updated": "2026-01-15 10:30:00"
}
```

**Errors:** `400` invalid id · `404` card not found

---

### `POST /cards`

Create a new card.

**Body**

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | Yes | |
| `scryfall_id` | string (uuid) | Yes | Must be a valid UUID. |
| `set_code` | string | Yes | |
| `collector_number` | int | Yes | Must be > 0. |
| `foil` | bool | No | Defaults to `false`. |
| `storage_id` | int | No | Must reference an existing storage if provided. |

**Example**
```json
{
  "name": "Counterspell",
  "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
  "set_code": "mh2",
  "collector_number": 267,
  "foil": false,
  "storage_id": 1
}
```

**Response `201 Created`** — the created card, including its generated `id`, `added`, and `updated`.

**Errors**
- `400` — missing required field, invalid UUID, invalid `collector_number`, or `storage_id` doesn't reference an existing storage (`"storage_id does not reference an existing storage"`)
- `500` — unexpected server/database error

---

### `PATCH /cards/:id`

Partially update a card. Any subset of the fields below can be sent.

**Body** *(all fields optional)*

| Field | Type | Notes |
|---|---|---|
| `name` | string | |
| `set_code` | string | |
| `collector_number` | int | Must be > 0 if provided. |
| `foil` | bool | |
| `storage_id` | int | Must reference an existing storage if provided. |

> `scryfall_id`, `added`, and `updated` can never be modified after creation.

**Example — move a card to a different storage**
```json
{ "storage_id": 3 }
```

**Response `200 OK`** — the full, updated card.

**Errors:** `400` invalid id / invalid body / `storage_id` doesn't reference an existing storage · `404` card not found

---

### `DELETE /cards/:id`

Delete a card. If the card is linked to any decks (including as a commander), those links are cleaned up automatically:
- Removed from `card_deck` (deck contents) via cascade.
- Any deck's `commander_id` pointing to this card is set to `null`.

**Response `204 No Content`**

**Errors:** `400` invalid id · `404` card not found

---

## Storage

A **storage** represents a physical place where cards live — a binder, a deckbox, a box, etc.

> All endpoints below require `Authorization: Bearer <token>` and only ever operate on the authenticated account's own storages.

### `GET /storage`

List all storages, each annotated with its current card count.

**Response `200 OK`**
```json
[
  {
    "id": 1,
    "name": "Vintage Collection",
    "type": "binder",
    "card_count": 3,
    "added": "2026-01-15 10:30:00",
    "updated": "2026-01-15 10:30:00"
  }
]
```

---

### `GET /storage/:id`

Fetch a single storage by ID.

**Response `200 OK`** — same shape as above, single object.

**Errors:** `400` invalid id · `404` storage not found

---

### `POST /storage`

Create a new storage.

**Body**

| Field | Type | Required |
|---|---|---|
| `name` | string | Yes |
| `type` | string | Yes |

**Example**
```json
{ "name": "Trade Binder", "type": "binder" }
```

**Response `201 Created`**

**Errors:** `400` missing required field

---

### `PATCH /storage/:id`

Partially update a storage.

**Body** *(all optional)*

| Field | Type |
|---|---|
| `name` | string |
| `type` | string |

**Response `200 OK`** — the full, updated storage.

**Errors:** `400` invalid id / invalid body · `404` storage not found

---

### `DELETE /storage/:id`

Delete a storage. Any card currently in this storage has its `storage_id` set to `null` — cards are never deleted as a side effect.

**Response `204 No Content`**

**Errors:** `400` invalid id · `404` storage not found

---

## Decks

> All endpoints below require `Authorization: Bearer <token>` and only ever operate on the authenticated account's own decks. A deck's `commander_id` must reference a card owned by the same account; a card can only be added to a deck owned by the same account.

### `GET /deck`

List all decks, each annotated with its current card count.

**Response `200 OK`**
```json
[
  {
    "id": 1,
    "name": "Kess Commander",
    "format": "commander",
    "commander_id": 12,
    "card_count": 4,
    "added": "2026-01-15 10:30:00",
    "updated": "2026-01-15 10:30:00"
  }
]
```

---

### `GET /deck/:id`

Fetch a single deck by ID.

**Errors:** `400` invalid id · `404` deck not found

---

### `POST /deck`

Create a new deck.

**Body**

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | Yes | |
| `format` | string | Yes | e.g. `commander`, `modern`, `standard`. |
| `commander_id` | int | No | Must reference an existing card if provided. |

**Example**
```json
{ "name": "Izzet Control", "format": "legacy" }
```

**Response `201 Created`**

**Errors:** `400` missing required field, or `commander_id` doesn't reference an existing card (`"commander_id does not reference an existing card"`)

---

### `PATCH /deck/:id`

Partially update a deck.

**Body** *(all optional)*

| Field | Type | Notes |
|---|---|---|
| `name` | string | |
| `format` | string | |
| `commander_id` | int | Must reference an existing card if provided. |
| `clear_storage_id` | bool | Set to `true` to explicitly remove the current commander (set `commander_id` to `null`). |

**Example — clear the commander**
```json
{ "clear_storage_id": true }
```

**Response `200 OK`** — the full, updated deck.

**Errors:** `400` invalid id / invalid body / invalid `commander_id` · `404` deck not found

---

### `DELETE /deck/:id`

Delete a deck and all of its card associations (`card_deck` rows are removed via cascade). Cards themselves are never deleted.

**Response `204 No Content`**

**Errors:** `400` invalid id · `404` deck not found

---

## Deck ↔ Card relationship

### `GET /deck/:id/cards`

List every card currently in a deck.

**Response `200 OK`**
```json
[
  {
    "id": 4,
    "name": "Lightning Bolt",
    "scryfall_id": "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d",
    "set_code": "2xm",
    "collector_number": 129,
    "foil": true,
    "storage_id": 2,
    "added": "2026-01-15 10:30:00",
    "updated": "2026-01-15 10:30:00"
  }
]
```

Returns an empty array `[]` if the deck has no cards.

**Errors:** `400` invalid id · `404` deck not found

---

### `PUT /deck/:id/cards/:card_id`

Add a card to a deck. **Idempotent** — calling this again with the same `id`/`card_id` succeeds silently if the card is already in the deck.

**No request body.**

**Response `204 No Content`**

**Errors**
- `400` — invalid `id` or `card_id` (not an integer)
- `404` — deck not found (`"deck not found"`) or card not found (`"card not found"`)
- `500` — unexpected server/database error

---

### `DELETE /deck/:id/cards/:card_id`

Remove a card from a deck. **Idempotent** — always returns success, whether or not the card was actually in the deck (or whether the deck/card exist at all). There's no distinct "not linked" error: the end state — "this card is not in this deck" — is what's guaranteed.

**Response `204 No Content`**

**Errors:** `400` invalid `id` or `card_id` · `500` unexpected server/database error

---

## Error reference

All error responses share the same shape:
```json
{ "error": "human-readable message" }
```

| Status | Meaning |
|---|---|
| `400 Bad Request` | Malformed input: invalid id, invalid JSON body, failed field validation, or a referenced resource ID (`storage_id`, `commander_id`) doesn't exist — including when that ID belongs to another account. |
| `401 Unauthorized` | Missing/malformed `Authorization` header, invalid or expired token, or (on `/auth/login`) wrong email/password. |
| `404 Not Found` | The resource identified by the URL doesn't exist for the authenticated account. A resource that exists but belongs to another account also returns `404`, not `403` — this avoids confirming that an ID exists at all. |
| `409 Conflict` | Email already registered (`/auth/register`). |
| `500 Internal Server Error` | Unexpected failure (database unreachable, etc). |

### Validation rules summary

| Field | Rule |
|---|---|
| `scryfall_id` | Must be a valid UUID |
| `collector_number` | Must be an integer > 0 |
| `storage_id` (on cards) | Must reference an existing storage row, if provided |
| `commander_id` (on decks) | Must reference an existing card row, if provided |
