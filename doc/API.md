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

Tamiyo is multi-tenant. `/health`, `/auth/register`, `/auth/login`, `/auth/refresh`, and `/auth/logout` are public; every other endpoint — including `/auth/password` — requires a Bearer token and is scoped to the authenticated account. You only ever see or modify your own cards, storages, and decks.

1. **Register** an account:
```bash
   curl -X POST localhost:8080/auth/register \
     -H "Content-Type: application/json" \
     -d '{"email": "you@example.com", "password": "at-least-8-chars"}'
```
2. **Log in** (or reuse the pair from registration):
```bash
   curl -X POST localhost:8080/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email": "you@example.com", "password": "at-least-8-chars"}'
```
   Both return `{"token": "...", "refresh_token": "..."}`.
3. Send the access token on every other request:
```bash
   curl localhost:8080/cards -H "Authorization: Bearer <token>"
```
4. Access tokens are short-lived (15 minutes by default). Use the refresh token to get a new pair without logging in again:
```bash
   curl -X POST localhost:8080/auth/refresh \
     -H "Content-Type: application/json" \
     -d '{"refresh_token": "<refresh_token>"}'
```

`/health`, `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, `/auth/forgot-password`, and `/auth/reset-password` are the only public routes — everything else (`/cards`, `/storage`, `/deck`, `/auth/password`) requires a valid Bearer token and only ever returns or modifies that account's own data.

`/auth/register` and `/auth/login` are rate-limited per client IP (5 requests/minute by default) to blunt brute-force attempts.

### Forgotten password

```bash
curl -X POST localhost:8080/auth/forgot-password \
  -H "Content-Type: application/json" \
  -d '{"email": "you@example.com"}'
```

Without an `SMTP_HOST` configured, the reset email is logged to stdout instead of sent — the token is right there in the container logs, no real mail server needed for local development. See [`openapi.yaml`](./doc/openapi.yaml) for every auth-related environment variable (`JWT_*`, `AUTH_RATE_LIMIT_*`, `PASSWORD_RESET_*`, `SMTP_*`).

---

### `POST /auth/login`

Exchange credentials for a token pair.

**Body**

| Field | Type | Required |
|---|---|---|
| `email` | string | Yes |
| `password` | string | Yes |

**Response `200 OK`**
```json
{
  "token": "eyJhbGciOi...",
  "refresh_token": "Z3f8K1m..."
}
```

**Errors:** `400` missing/invalid field · `401` invalid email or password · `429` too many requests (see [rate limiting](#rate-limiting))

> The same error is returned for "no such account" and "wrong password", by design — this prevents an attacker from using the endpoint to discover which emails have an account.

### Using the token

Send the access token on every other request:
```
Authorization: Bearer <token>
```
Access tokens are short-lived — 15 minutes by default (`JWT_ACCESS_TOKEN_TTL_MINUTES`). Use the `refresh_token` returned alongside it to get a new pair without logging in again (see below), instead of waiting for it to expire.

### `POST /auth/refresh`

Exchange a refresh token for a brand-new token pair.

**Body**

| Field | Type | Required |
|---|---|---|
| `refresh_token` | string | Yes |

**Response `200 OK`**
```json
{
  "token": "eyJhbGciOi...",
  "refresh_token": "N9pQ2x..."
}
```

**Errors:** `400` missing `refresh_token` · `401` invalid, expired, or already-used refresh token

> Refresh tokens are **single-use**: a successful call revokes the one you sent and returns a new one (token rotation). Submitting an already-used refresh token is treated as a possible theft — it revokes *every* refresh token belonging to that account, logging out all of its sessions.

Refresh tokens are valid for 30 days by default (`JWT_REFRESH_TOKEN_TTL_DAYS`) unless revoked sooner by a refresh, a logout, a password change, or reuse detection.

---

### `POST /auth/logout`

Revoke a refresh token, ending that session.

**Body**

| Field | Type | Required |
|---|---|---|
| `refresh_token` | string | Yes |

**Response `204 No Content`**

**Errors:** `400` missing `refresh_token`

> Idempotent — logging out with an unknown or already-revoked refresh token still returns `204`. This only revokes the refresh token: an access token already issued remains valid until it naturally expires (JWTs are stateless), which is exactly why the access token's lifetime is kept short.

---

### `POST /auth/password`

Change the authenticated account's password. **Requires `Authorization: Bearer <token>`.**

**Body**

| Field | Type | Required | Notes |
|---|---|---|---|
| `current_password` | string | Yes | Must match the account's current password. |
| `new_password` | string | Yes | Minimum 8 characters. |

**Response `204 No Content`**

**Errors:** `400` missing field / `new_password` too short · `401` missing/invalid token, or `current_password` is incorrect · `404` account no longer exists

> On success, **every** refresh token belonging to the account is revoked — all other sessions (and this one, once its current access token expires) must log in again.

### `POST /auth/forgot-password`

Request a password reset email.

**Body**

| Field | Type | Required |
|---|---|---|
| `email` | string | Yes |

**Response `204 No Content`**

**Errors:** `400` missing/invalid email · `429` too many requests (see [rate limiting](#rate-limiting))

> Always returns `204`, whether or not the email belongs to an account — this prevents using the endpoint to discover which emails are registered. If the account exists, a single-use reset token is emailed to it, valid for 30 minutes by default (`PASSWORD_RESET_TOKEN_TTL_MINUTES`). Without `SMTP_HOST` configured, the email is logged by the server instead of sent — handy for local development.

---

### `POST /auth/reset-password`

Set a new password using the token from the forgot-password email.

**Body**

| Field | Type | Required | Notes |
|---|---|---|---|
| `token` | string | Yes | From the forgot-password email. |
| `new_password` | string | Yes | Minimum 8 characters. |

**Response `204 No Content`**

**Errors:** `400` missing field / `new_password` too short · `401` invalid, expired, or already-used token

> On success, **every** refresh token belonging to the account is revoked, same as `/auth/password`.

---

### Rate limiting

`POST /auth/register`, `POST /auth/login`, and `POST /auth/forgot-password` share a per-client-IP limit: 5 requests per 60-second window by default (`AUTH_RATE_LIMIT_MAX` / `AUTH_RATE_LIMIT_WINDOW_SECONDS`). Exceeding it returns:

**Response `429 Too Many Requests`**
```json
{ "error": "too many requests" }
```
with a `Retry-After` header giving the number of seconds until the window resets.

> The limiter is in-memory and per-instance. Running several replicas behind a load balancer means each instance tracks its own counter — there's no shared global limit without an external store (e.g. Redis).

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

List cards, with optional filtering, sorting, and pagination.

**Query parameters**

| Param | Type | Required | Description |
|---|---|---|---|
| `storage_id` | int | No | Only return cards belonging to this storage. |
| `name` | string | No | Case-insensitive partial match on the card name. |
| `page` | int | No | 1-based page number. Defaults to `1`. |
| `limit` | int | No | Cards per page, max `100`. Defaults to `25`. |
| `sort` | string | No | One of `name`, `-name`, `added`, `-added`, `updated`, `-updated`. Defaults to `-updated`. A `-` prefix means descending. `id` is always used as a stable secondary tie-breaker. |

**Example**
```
GET /cards?storage_id=1&sort=-added&page=1&limit=25
```

**Response `200 OK`**
```json
{
  "data": [
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
  ],
  "page": 1,
  "limit": 25,
  "total": 1,
  "total_pages": 1
}
```

**Errors:** `400` if `storage_id`, `page`, or `limit` is not a valid integer, or `sort` is not one of the allowed values.

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

List all storages, each annotated with its current card count. Supports optional filtering and pagination.

**Query parameters**

| Param | Type | Required | Description |
|---|---|---|---|
| `type` | string | No | Only return storages of this type. Exact match, case-insensitive. |
| `page` | int | No | 1-based page number. Defaults to `1`. |
| `limit` | int | No | Storages per page, max `100`. Defaults to `25`. |
| `sort` | string | No | One of `name`, `-name`, `added`, `-added`, `updated`, `-updated`. Defaults to `-updated`. A `-` prefix means descending. `id` is always used as a stable secondary tie-breaker. |

**Example**
```
GET /storage?type=binder&sort=-added&page=1&limit=25
```

**Response `200 OK`**
```json
{
  "data": [
    {
      "id": 1,
      "name": "Vintage Collection",
      "type": "binder",
      "card_count": 3,
      "added": "2026-01-15 10:30:00",
      "updated": "2026-01-15 10:30:00"
    }
  ],
  "page": 1,
  "limit": 25,
  "total": 1,
  "total_pages": 1
}
```

**Errors:** `400` if `page` or `limit` is not a valid integer, `limit` is outside `1..100`, or `sort` is not one of the allowed values.

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

List all decks, each annotated with its current card count. Supports optional filtering and pagination.

**Query parameters**

| Param | Type | Required | Description |
|---|---|---|---|
| `format` | string | No | Only return decks of this format. Exact match, case-insensitive. |
| `page` | int | No | 1-based page number. Defaults to `1`. |
| `limit` | int | No | Decks per page, max `100`. Defaults to `25`. |
| `sort` | string | No | One of `name`, `-name`, `added`, `-added`, `updated`, `-updated`. Defaults to `-updated`. A `-` prefix means descending. `id` is always used as a stable secondary tie-breaker. |

**Example**
```
GET /deck?format=commander&sort=-added&page=1&limit=25
```

**Response `200 OK`**
```json
{
  "data": [
    {
      "id": 1,
      "name": "Kess Commander",
      "format": "commander",
      "commander_id": 12,
      "card_count": 4,
      "added": "2026-01-15 10:30:00",
      "updated": "2026-01-15 10:30:00"
    }
  ],
  "page": 1,
  "limit": 25,
  "total": 1,
  "total_pages": 1
}
```

**Errors:** `400` if `page` or `limit` is not a valid integer, `limit` is outside `1..100`, or `sort` is not one of the allowed values.

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
| `clear_commander_id` | bool | Set to `true` to explicitly remove the current commander (set `commander_id` to `null`). |

**Example — clear the commander**
```json
{ "clear_commander_id": true }
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
| `401 Unauthorized` | Missing/malformed `Authorization` header, invalid or expired access token, invalid/expired/reused refresh token, invalid/expired/used password-reset token, incorrect `current_password`, or (on `/auth/login`) wrong email/password. |
| `404 Not Found` | The resource identified by the URL doesn't exist for the authenticated account. A resource that exists but belongs to another account also returns `404`, not `403` — this avoids confirming that an ID exists at all. |
| `409 Conflict` | Email already registered (`/auth/register`). |
| `429 Too Many Requests` | Rate limit exceeded on `/auth/register` or `/auth/login` (see [rate limiting](#rate-limiting)). |
| `500 Internal Server Error` | Unexpected failure (database unreachable, etc). |

### Validation rules summary

| Field | Rule |
|---|---|
| `scryfall_id` | Must be a valid UUID |
| `collector_number` | Must be an integer > 0 |
| `storage_id` (on cards) | Must reference an existing storage row, if provided |
| `commander_id` (on decks) | Must reference an existing card row, if provided |
| `password` / `new_password` | Minimum 8 characters |
| `refresh_token` | Single-use; reuse after rotation revokes all of the account's refresh tokens |
