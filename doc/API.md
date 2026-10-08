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
- [Profiles](#profiles)
- [Deck ↔ Card relationship](#deck--card-relationship)
- [Deck Insights](#deck-insights)
- [Shared decks](#shared-decks)
- [Bulk Import](#bulk-import)
- [Bulk Export](#bulk-export)
- [Error reference](#error-reference)

---

## Authentication

Tamiyo is multi-tenant. `/health`, `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout` and the [shared deck](#shared-decks) routes are public; every other endpoint — including `/auth/password` — requires a Bearer token and is scoped to the authenticated account. You only ever see or modify your own cards, storages, and decks.

1. **Register** an account:
```bash
   curl -X POST localhost:8080/auth/register \
     -H "Content-Type: application/json" \
     -d '{"email": "you@example.com", "password": "at-least-8-chars", "display_name": "Tamiyo"}'
```
   `display_name` is optional (trimmed, at most 32 characters, see [`PATCH /auth/me`](#patch-authme)); a longer one is rejected with `400`.
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

`/health`, `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, `/auth/forgot-password`, `/auth/reset-password`, and `/auth/confirm-email` are the only public routes — everything else (`/cards`, `/storage`, `/deck`, `/auth/password`, `/auth/email`, `/auth/me`) requires a valid Bearer token and only ever returns or modifies that account's own data.

`/auth/register` and `/auth/login` are rate-limited per client IP (5 requests/minute by default) to blunt brute-force attempts.

### Forgotten password

```bash
curl -X POST localhost:8080/auth/forgot-password \
  -H "Content-Type: application/json" \
  -d '{"email": "you@example.com"}'
```

Without an `SMTP_HOST` configured, the reset email is logged to stdout instead of sent — the token is right there in the container logs, no real mail server needed for local development. See [`openapi.yaml`](./doc/openapi.yaml) for every auth-related environment variable (`JWT_*`, `AUTH_RATE_LIMIT_*`, `PASSWORD_RESET_*`, `EMAIL_CHANGE_*`, `SMTP_*`).

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

### Browser clients: the refresh cookie

`/auth/register`, `/auth/login` and `/auth/refresh` also set the refresh token in a cookie named `tamiyo_refresh_token`: `HttpOnly`, `SameSite=Strict`, `Secure` (unless `REFRESH_COOKIE_SECURE=false`), scoped to `<API_BASE_PATH>/auth` so it is only ever sent to the auth routes. A browser frontend served from the same origin as the API should:

- keep the access token in memory only (never `localStorage`),
- call `/auth/refresh` and `/auth/logout` with no body: the browser sends the cookie on its own, and the token is read from it,
- send `X-Refresh-Token-Transport: cookie` on `/auth/register`, `/auth/login` and `/auth/refresh`: `refresh_token` is then left out of the JSON responses.

JavaScript can't read an `HttpOnly` cookie, and with that header the token never appears in a response body either, so an XSS bug can't steal the long-lived refresh token. API clients (curl, scripts) are unaffected: they keep sending `refresh_token` in the body, which takes precedence over the cookie.

### `POST /auth/refresh`

Exchange a refresh token for a brand-new token pair.

**Body**

| Field | Type | Required |
|---|---|---|
| `refresh_token` | string | No — falls back to the `tamiyo_refresh_token` cookie |

The new refresh token is also set in the cookie (see [the refresh cookie](#browser-clients-the-refresh-cookie)).

**Response `200 OK`**
```json
{
  "token": "eyJhbGciOi...",
  "refresh_token": "N9pQ2x..."
}
```

**Errors:** `400` refresh token in neither the body nor the cookie, or malformed body · `401` invalid, expired, or already-used refresh token (the cookie is cleared)

> Refresh tokens are **single-use**: a successful call revokes the one you sent and returns a new one (token rotation). Submitting an already-used refresh token is treated as a possible theft — it revokes *every* refresh token belonging to that account, logging out all of its sessions. One exception: a token rotated out less than 10 seconds ago is accepted once more (the client most likely never received the response, e.g. a page reloaded mid-refresh), as long as the token that replaced it is still valid.

Refresh tokens are valid for 30 days by default (`JWT_REFRESH_TOKEN_TTL_DAYS`) unless revoked sooner by a refresh, a logout, a password change, or reuse detection.

---

### `POST /auth/logout`

Revoke a refresh token, ending that session. The refresh cookie is always cleared.

**Body**

| Field | Type | Required |
|---|---|---|
| `refresh_token` | string | No — falls back to the `tamiyo_refresh_token` cookie |

**Response `204 No Content`**

**Errors:** `400` refresh token in neither the body nor the cookie, or malformed body

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

### `GET /auth/me`

The authenticated account. **Requires `Authorization: Bearer <token>`.**

**Response `200 OK`**
```json
{ "id": "4b8a0a9e-2f1c-4c8e-9d3a-1e2f3a4b5c6d", "email": "you@example.com", "display_name": "Tamiyo", "avatar_scryfall_id": "0000579f-7b35-4ed3-b44c-db2a538066fe" }
```
`display_name` and `avatar_scryfall_id` are `null` until the user picks them. The avatar is the art crop of that Scryfall card: clients fetch the image from Scryfall and credit its artist, Tamiyo only stores the id.

**Errors:** `401` missing/invalid token · `404` account no longer exists

---

### `PATCH /auth/me`

Set or clear the account's profile: its display name, a purely cosmetic name the app shows instead of the email, and its avatar. **Requires `Authorization: Bearer <token>`.** Display names aren't unique and never identify an account. Only the fields present in the body change; at least one is required.

**Body**

| Field | Type | Required | Notes |
|---|---|---|---|
| `display_name` | string \| null | No | Trimmed; at most 32 characters. `null` or a blank string clears it. |
| `avatar_scryfall_id` | string \| null | No | A Scryfall card id (UUID); its art crop becomes the avatar. Not checked against Scryfall. `null` or a blank string clears it. |

**Response `200 OK`** — same body as `GET /auth/me`.

**Errors:** `400` no field given, a field that isn't a string, `display_name` longer than 32 characters, or `avatar_scryfall_id` not a UUID (nothing is changed then) · `401` missing/invalid token · `404` account no longer exists

---

### `POST /auth/email`

Ask to change the authenticated account's email. **Requires `Authorization: Bearer <token>`.** The email doesn't change yet: a single-use confirmation token is emailed to the **current** address, so that only the account's owner can approve the change even with a stolen session or password, valid for a day by default (`EMAIL_CHANGE_TOKEN_TTL_MINUTES`), as a link when `EMAIL_CHANGE_URL_TEMPLATE` is set. The account keeps signing in with its current email until the token is sent to `POST /auth/confirm-email`.

**Body**

| Field | Type | Required | Notes |
|---|---|---|---|
| `current_password` | string | Yes | Must match the account's current password. |
| `new_email` | string | Yes | A valid email, different from the current one. |

**Response `202 Accepted`**

**Errors:** `400` missing field, invalid email, or `new_email` is the current email · `401` missing/invalid token, or `current_password` is incorrect · `404` account no longer exists · `409` `new_email` already belongs to an account · `500` the confirmation email could not be sent

> Asking again sends a new token; earlier ones stay valid until they expire or one of them is used.

---

### `POST /auth/confirm-email`

Confirm an email change with the token emailed to the current address. Public, so the link works from any device; rate-limited like the other public auth routes.

**Body**

| Field | Type | Required |
|---|---|---|
| `token` | string | Yes |

**Response `204 No Content`**

**Errors:** `400` missing token · `401` invalid, expired, or already-used token · `409` the new email was registered by another account in the meantime · `429` too many requests

> On success the account signs in with the new email, and a notice is sent to the new address. Sessions stay open.

---

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

`POST /auth/register`, `POST /auth/login`, `POST /auth/forgot-password`, `POST /auth/reset-password`, and `POST /auth/confirm-email` share a per-client-IP limit: 5 requests per 60-second window by default (`AUTH_RATE_LIMIT_MAX` / `AUTH_RATE_LIMIT_WINDOW_SECONDS`). Exceeding it returns:

**Response `429 Too Many Requests`**
```json
{ "error": "too many requests" }
```
with a `Retry-After` header giving the number of seconds until the window resets.

The public [shared deck](#shared-decks) routes have their own limit, counted separately: 60 requests per 60-second window by default (`SHARE_RATE_LIMIT_MAX` / `SHARE_RATE_LIMIT_WINDOW_SECONDS`).

> The limiter is in-memory and per-instance. Running several replicas behind a load balancer means each instance tracks its own counter — there's no shared global limit without an external store (e.g. Redis).

---

## Conventions

### Timestamps
Every resource carries `added` and `updated` timestamps, generated and maintained by the database — they can never be set or modified through the API. A storage's `updated` also moves when a card is put in it, moved out of it or deleted, and a deck's when a card is added to it or removed from it, so `sort=-updated` lists the most recently active ones first.

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
| `color_identity` | string | No | Only cards whose color identity fits inside these WUBRG letters (any order), e.g. a commander's identity. Empty (`color_identity=`) keeps colorless cards only. Cards whose identity isn't known yet are left out. |
| `page` | int | No | 1-based page number. Defaults to `1`. |
| `limit` | int | No | Cards per page, max `100`. Defaults to `25`. |
| `stack` | bool | No | `true` returns one entry per stack of identical copies (same printing, foil, proxy and storage) instead of one per card: the stack's lowest-id copy, plus `quantity` and `copy_ids` (lowest first). `total`, `page` and `limit` then count stacks; `added` is the oldest copy's and `updated` the most recent one's. Defaults to `false`. |
| `sort` | string | No | One of `name`, `-name`, `added`, `-added`, `updated`, `-updated`, `mana_value`, `-mana_value`, `color`, `-color`, `type`, `-type`. Defaults to `-updated`. A `-` prefix means descending. `color` groups white, blue, black, red, green, multicolor, colorless, lands, then unknown; `type` groups by primary type (creature, planeswalker, battle, instant, sorcery, artifact, enchantment, land, other, unknown); both sort by name within a group. `id` is always used as a stable secondary tie-breaker. |

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
      "proxy": false,
      "storage_id": 1,
      "mana_value": 0,
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

**Errors:** `400` if `storage_id`, `page`, or `limit` is not a valid integer, `stack` is not a boolean, or `sort` is not one of the allowed values.

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
  "mana_value": 0,
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
| `proxy` | bool | No | `true` for a proxy (a printed stand-in rather than a real copy). Defaults to `false`. |
| `storage_id` | int | No | Must reference an existing storage if provided. |
| `mana_value` | number | No | Converted mana cost (CMC). Must be >= 0 if provided. Defaults to `0`. Not fetched automatically from Scryfall — the client supplies it, same as `name` or `set_code`. |
| `colors` | string | No | The card's colors as WUBRG letters (any case or order, stored in WUBRG order, e.g. `WR`), empty string for colorless. Left unknown (`null`) when omitted. |
| `card_type` | string | No | Primary type: `Creature`, `Planeswalker`, `Battle`, `Instant`, `Sorcery`, `Artifact`, `Enchantment`, `Land` or `Other`. Left unknown (`null`) when omitted. |
| `color_identity` | string | No | Color identity as WUBRG letters, normalized like `colors`. Left unknown (`null`) when omitted. |

**Example**
```json
{
  "name": "Counterspell",
  "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
  "set_code": "mh2",
  "collector_number": 267,
  "foil": false,
  "storage_id": 1,
  "mana_value": 2
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
| `proxy` | bool | |
| `storage_id` | int or `null` | Must reference an existing storage if provided. `null` removes the card from its storage; leaving the field out keeps the current one. |
| `mana_value` | number | Must be >= 0 if provided. |

> `scryfall_id`, `added`, and `updated` can never be modified after creation.

**Example — move a card to a different storage**
```json
{ "storage_id": 3 }
```

**Response `200 OK`** — the full, updated card.

**Errors:** `400` invalid id / invalid body / `storage_id` doesn't reference an existing storage · `404` card not found

---

### `DELETE /cards/:id`

Delete a card from the collection. The decks it was in keep it as a card to get back:
- Each of those decks gets it in its [pending list](#pending-cards-deckidpending) (one copy, merged with an existing pending entry for the same printing and foil), so it still shows in the deck until it's added to the collection again.
- Its `card_deck` links are removed. A deck whose commander it was keeps it as its pending commander (`commander_pending_id`).

**Response `204 No Content`**

**Errors:** `400` invalid id · `404` card not found

### `POST /cards/refresh-details`

Look up on Scryfall the cards whose `colors`, `card_type` or `color_identity` is still unknown (cards created before those fields existed, or imported while Scryfall was unreachable), and store their colors, primary type, color identity and mana value. Imports and the web frontend fill these in on their own; this is for the backlog.

Scryfall allows one `/cards/collection` call every 500 ms, so a large backlog takes a while. To keep each request short, a call handles at most 750 cards, in id order. While `next_after_id` is not `null`, call again with `?after_id=<next_after_id>`.

| Query param | Description |
|---|---|
| `after_id` | Optional, default `0`: only look at cards with a greater id |

**Response `200 OK`**
```json
{ "updated": 748, "not_found": 2, "remaining": 1630, "next_after_id": 2214 }
```

**Errors:** `400` invalid `after_id`, `502` Scryfall unreachable

### `DELETE /cards?confirm=true`

Delete **every** card of the account at once, with the same clean-up as above (each deck keeps its cards, commander included, in its pending list). Storages and decks themselves are kept, now empty. `confirm=true` is required so the collection can't be wiped by accident.

**Response `200 OK`**
```json
{ "deleted": 128 }
```

**Errors:** `400` `confirm=true` is missing

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
      "id": "6f0d3c5e-8a51-4c0b-9b1e-2d7c4a3f9e10",
      "name": "Kess Commander",
      "format": "commander",
      "commander_id": 12,
      "background_scryfall_id": "436d6a84-4cea-4ca7-94aa-9d08280652af",
      "commander_scryfall_id": "a0b4c5ad-14f7-4bcb-9a59-6c0ac4f1a5e0",
      "visibility": "unlisted",
      "card_count": 4,
      "pending_count": 2,
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

`pending_count` is the number of copies in the deck's pending list (see [Pending cards](#pending-cards-deckidpending)), not counted in `card_count`. `commander_scryfall_id` is read-only: the Scryfall id of the commander card, so a client can show its art without another call. `background_scryfall_id` is the art the user picked for the deck (`null` when none was chosen). `visibility` says who may see the deck: `private` (only its owner), `unlisted` (anyone with its link, the default) or `public` (anyone, and listed on its owner's profile). `id` is a random UUID generated when the deck is created, so deck ids can't be guessed from one another. It is also how anyone reaches the deck's read-only page: see [Shared decks](#shared-decks). The routes in this section still only serve the owner's own decks.

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
| `background_scryfall_id` | uuid | No | Scryfall id of the printing whose art (`art_crop`) is shown behind the deck. Any printing works, it doesn't have to be in the deck. |
| `visibility` | string | No | `private`, `unlisted` (default) or `public`. |

**Example**
```json
{ "name": "Izzet Control", "format": "legacy" }
```

**Response `201 Created`**

**Errors:** `400` missing required field, `visibility` not one of `private`, `unlisted`, `public`, or `commander_id` doesn't reference an existing card (`"commander_id does not reference an existing card"`)

---

### `PATCH /deck/:id`

Partially update a deck.

**Body** *(all optional)*

| Field | Type | Notes |
|---|---|---|
| `name` | string | |
| `format` | string | |
| `commander_id` | int | Must reference an existing card if provided. |
| `commander_pending_id` | int | Make one of the deck's [pending cards](#pending-cards-deckidpending) the commander, for a commander not in the collection yet. Clears `commander_id`; setting `commander_id` clears it in turn. When that pending card is added to the collection (`POST /deck/:id/pending/commit`), its first new copy becomes `commander_id`. |
| `clear_commander_id` | bool | Set to `true` to explicitly remove the current commander (sets both `commander_id` and `commander_pending_id` to `null`). |
| `background_scryfall_id` | uuid | Scryfall id of the printing whose art is shown behind the deck. |
| `clear_background_scryfall_id` | bool | Set to `true` to remove the chosen art (set `background_scryfall_id` to `null`). |
| `visibility` | string | `private`, `unlisted` or `public`. |

**Example — clear the commander**
```json
{ "clear_commander_id": true }
```

**Response `200 OK`** — the full, updated deck.

**Errors:** `400` invalid id / invalid body / invalid `commander_id` / `background_scryfall_id` not a UUID / unknown `visibility` · `404` deck not found

---

### `DELETE /deck/:id`

Delete a deck and all of its card associations (`card_deck` rows are removed via cascade). Cards themselves are never deleted.

**Response `204 No Content`**

**Errors:** `400` invalid id · `404` deck not found

---

## Profiles

What any signed-in user can see of another user. **Requires `Authorization: Bearer <token>`.** A user's email is never exposed here; `id` is the one from `GET /auth/me`.

### `GET /users/:id`

**Response `200 OK`**
```json
{ "id": "4b8a0a9e-2f1c-4c8e-9d3a-1e2f3a4b5c6d", "display_name": "Tamiyo", "avatar_scryfall_id": "0000579f-7b35-4ed3-b44c-db2a538066fe" }
```
`display_name` and `avatar_scryfall_id` are `null` when the user hasn't picked them.

**Errors:** `400` `:id` is not a UUID · `401` missing/invalid token · `404` no such user

### `GET /users/:id/decks`

That user's **public** decks (`visibility` = `public`), with the same pagination, `sort` and response shape as `GET /deck`. Unlisted and private decks are never listed. An unknown user simply has no decks.

**Errors:** `400` `:id` is not a UUID, or invalid `page` / `limit` / `sort` · `401` missing/invalid token

---

## Deck ↔ Card relationship

### `GET /deck/:id/cards`

List every card currently in a deck.

**Query parameters**

| Param | Type | Required | Description |
|---|---|---|---|
| `sort` | string | No | One of `name`, `-name`, `added`, `-added`, `updated`, `-updated`, `mana_value`, `-mana_value`. Defaults to `-updated`. A `-` prefix means descending. `id` is always used as a stable secondary tie-breaker. |

**Example**
```
GET /deck/6f0d3c5e-8a51-4c0b-9b1e-2d7c4a3f9e10/cards?sort=mana_value
```

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
    "mana_value": 1,
    "added": "2026-01-15 10:30:00",
    "updated": "2026-01-15 10:30:00"
  }
]
```

Returns an empty array `[]` if the deck has no cards.

**Errors:** `400` invalid id, or `sort` is not one of the allowed values · `404` deck not found

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

### Pending cards (`/deck/:id/pending`)

Cards wanted in a deck but not in the collection yet (typically picked on Scryfall from the deck page). They are stored per deck until they're added to the collection all at once.

- `GET /deck/:id/pending` — the list, sorted by name.
- `POST /deck/:id/pending` — add one. Body: `name`, `scryfall_id`, `set_code`, `collector_number` (required), `foil`, `quantity` (1–100, default 1), `mana_value`, `colors`, `card_type`, `color_identity` (same rules as `POST /cards`). Returns `201` with the item. If the deck already has a pending card of the same printing and finish (other than its pending commander), its quantity is raised instead and that item is returned.
- `PATCH /deck/:id/pending/:pending_id` — change how many copies it stands for. Body: `{ "quantity": 12 }` (1–1000). Returns the item.
- `DELETE /deck/:id/pending/:pending_id` — remove one. `204`, or `404` if it doesn't exist.
- `POST /deck/:id/pending/commit` — create every pending card in the collection (one card per copy, in `storage_id` if given), put each in the deck, and clear the list. Body optional: `{ "storage_id": 4 }`, plus `"pending_id": 12` to only add that one pending card (all its copies), and `"quantity": 5` with it to only add that many copies (the pending card keeps the rest). Returns `{ "cards_created": 7 }`. Items are handled one by one: on a failure, those already handled stay done and the rest stay pending.
- `POST /deck/:id/import` — add a list of cards to this deck, as `multipart/form-data` with the list in a `file` field and an optional `commander_from_first_line` (default `false`). It never creates cards: owned copies go in the deck, missing ones become pending cards. Details, list format and response in [`POST /deck/:id/import`](#post-deckidimport) under Bulk Import.

**Errors:** `400` invalid id or body, unknown `storage_id` · `404` deck or pending card not found

---

## Deck Insights

Two read-only routes analyze a deck's cards — nothing here persists anything, every call is computed fresh from Scryfall data at request time. Both are purely about playing the game: there's no notion of card price or collection value anywhere in Tamiyo.

**Errors common to both**
- `400` — `:id` is not a UUID, or the deck's own `format` field isn't a format Scryfall recognizes (so legality/stats can't be computed against it)
- `401` — unauthenticated
- `404` — no deck with that id exists for this account
- `502` — Scryfall couldn't be reached
- `500` — unexpected failure

---

### `GET /deck/:id/legality`

Checks every card in the deck against the deck's own `format` (lowercased, matched against Scryfall's per-card `legalities`). A card that's `not_legal`, `restricted` or `banned` for that format is reported as an issue; a card whose Scryfall ID can't be resolved is reported too (legality can't be verified), rather than silently ignored.

For the `commander` format specifically, two deck-construction rules Scryfall's per-card legality can't express are checked as well:
- **Singleton** — at most one copy of each card by name, except basic lands.
- **Color identity** — every card's color identity must be contained in the commander's (the deck's `commander_id`, or its `commander_pending_id` when the commander isn't in the collection yet).

The deck size is checked too, counting every copy (the commander and pending cards included): exactly 100 cards for `commander`, `brawl`, `duel`, `paupercommander`, `predh` and `gladiator`, exactly 60 for `oathbreaker` and `standardbrawl`, at least 60 for the other constructed formats (`standard`, `pioneer`, `modern`, `legacy`, `vintage`, `pauper`, `premodern`, `explorer`, `historic`, `timeless`, `alchemy`, `oldschool`, `penny`, `future`). Other formats get no size check. A wrong size is reported first, as an issue with an empty `card_name` and a reason like `deck size: 87 cards, commander requires exactly 100`.

Pending cards (see `GET /deck/:id/pending-cards`) are checked like the deck's own cards, one entry per copy, so a pending copy counts toward the singleton rule too. An issue about a pending card has no `card_id`.

**Known limitations:** only `commander` itself gets these extra checks (not singleton siblings like `oathbreaker` or `brawl`); named singleton exceptions (e.g. Shadowborn Apostle, Relentless Rats) aren't recognized, only the basic-land exemption; partner/background commanders aren't handled.

**Response `200 OK`**
```json
{
  "format": "commander",
  "legal": false,
  "issues": [
    { "card_id": 42, "card_name": "Channel", "reason": "banned in commander" },
    { "card_name": "", "reason": "deck size: 99 cards, commander requires exactly 100" },
    { "card_name": "Mountain", "reason": "singleton violation: 2 copies in deck (commander allows only 1, except basic lands)" }
  ]
}
```
`issues` is omitted entirely when the deck is legal.

### `GET /deck/:id/stats`

Summarizes the deck's composition: mana curve, color breakdown, and primary card types — all computed from Scryfall data, nothing stored by Tamiyo itself. Lands are excluded from the mana curve, color breakdown, and average mana value (a land's mana value is always 0 and it has no casting colors, so including it would just dilute what the deck actually casts) but are still counted in `card_count`, `land_count`, and `type_breakdown`. Pending cards are included, once per copy, as if they were already in the deck.

**Response `200 OK`**
```json
{
  "card_count": 100,
  "land_count": 38,
  "nonland_count": 62,
  "average_mana_value": 2.74,
  "mana_curve": [
    { "mana_value": 0, "count": 3 },
    { "mana_value": 1, "count": 12 },
    { "mana_value": 2, "count": 20 }
  ],
  "color_breakdown": { "W": 10, "U": 8, "C": 5 },
  "type_breakdown": { "Land": 38, "Creature": 30, "Instant": 12, "Sorcery": 10, "Artifact": 10 }
}
```

A multicolor card counts once per color it has in `color_breakdown`; a dual-typed permanent (e.g. "Artifact Creature") counts once under a single primary type in `type_breakdown` — Creature takes precedence over Artifact/Enchantment, matching how most deckbuilding sites categorize it. A double-faced or split card counts under the type of its front face (a creature with a land on its back is a creature).

---

## Shared decks

Read-only access to a deck by its id. **No authentication required.** Deck ids are random UUIDs, so an unlisted deck can only be found by someone who was given its id or link. Only `public` and `unlisted` decks resolve: a private deck, an unknown id or anything that isn't a UUID all answer `404`, so the response never tells whether a private deck exists.

The three routes share a per-client-IP limit, separate from the auth one: 60 requests per 60-second window by default (`SHARE_RATE_LIMIT_MAX` / `SHARE_RATE_LIMIT_WINDOW_SECONDS`). Exceeding it returns `429` with a `Retry-After` header, as described in [Rate limiting](#rate-limiting).

### `GET /shared/decks/:id`

**Response `200 OK`**
```json
{
  "deck": {
    "id": "6f0d3c5e-8a51-4c0b-9b1e-2d7c4a3f9e10",
    "name": "Kess Commander",
    "format": "commander",
    "visibility": "unlisted",
    "background_scryfall_id": null,
    "commander_scryfall_id": "a0b4c5ad-14f7-4bcb-9a59-6c0ac4f1a5e0",
    "card_count": 100,
    "added": "2026-01-15 10:30:00",
    "updated": "2026-01-15 10:30:00"
  },
  "owner": { "id": "4b8a0a9e-2f1c-4c8e-9d3a-1e2f3a4b5c6d", "display_name": "Tamiyo", "avatar_scryfall_id": null },
  "cards": [
    {
      "name": "Island",
      "scryfall_id": "fc3f6a8f-0b5e-4b5f-9a1a-1d4b0e3c4c2f",
      "set_code": "neo",
      "collector_number": "294",
      "foil": false,
      "quantity": 12,
      "mana_value": 0,
      "colors": null,
      "card_type": "Land",
      "color_identity": "U",
      "commander": false
    }
  ]
}
```

The owner's copies and the deck's pending cards are merged into one list, one entry per printing and finish with its `quantity`, sorted by name. Nothing tells them apart, and storages, proxies and card ids are left out. The commander is always its own entry with `commander: true`. `card_count` counts every card, pending ones included. `owner` has the shape of [`GET /users/:id`](#get-usersid).

**Errors:** `404` unknown, private or malformed id · `429` rate limit exceeded

### `GET /shared/decks/:id/legality`

Same report as [`GET /deck/:id/legality`](#get-deckidlegality), without `card_id` on the issues.

**Errors:** `400` the deck's format isn't one Scryfall recognizes · `404` as above · `429` rate limit exceeded · `502` Scryfall unreachable

### `GET /shared/decks/:id/stats`

Same statistics as [`GET /deck/:id/stats`](#get-deckidstats).

**Errors:** `400` the deck's format isn't one Scryfall recognizes · `404` as above · `429` rate limit exceeded · `502` Scryfall unreachable

---

## Bulk Import

Four routes import cards in bulk, instead of one `POST /cards` call per card: a collection export from a third-party tool, a plain card list, or a decklist. The collection and list imports create cards (and, for ManaBox, storages/decks); the deck import fills an existing deck with cards already in the collection and never creates any. All four are `multipart/form-data` requests (not JSON) with the file itself in a field named `file`.

A bulk import never fails outright just because some rows couldn't be resolved: a request that parses successfully always returns `200 OK` with a summary of what happened, including a `warnings` list for any row that was skipped (card not found on Scryfall, a duplicate, a transient error). The import only fails as a whole (non-`200`) when the file itself can't be parsed, a referenced `storage_id` doesn't exist, or Scryfall couldn't be reached at all.

**Response shape (all four routes)**
```json
{
  "cards_created": 182,
  "cards_linked": 0,
  "cards_pending": 0,
  "cards_skipped": 3,
  "storages_created": 1,
  "decks_created": 0,
  "warnings": [
    "line 47: \"Mystery Card\" (xyz #999) not found on scryfall"
  ]
}
```
`warnings` is omitted entirely when empty. `cards_linked` and `cards_pending` are only ever non-zero for the deck import.

**Errors common to all four**
- `400` — no `file` field, or the file couldn't be parsed (wrong columns, malformed line) — message explains what's wrong
- `400` — a `storage_id` field doesn't reference an existing storage for this account
- `502` — Scryfall (used to resolve Moxfield rows — see below) couldn't be reached or returned an unexpected response after retrying; a rate-limited (`429`) response from Scryfall is retried automatically (honoring its `Retry-After` header when present) before this is returned
- `401` — unauthenticated, like every other route under this section

---

### `POST /import/manabox`

Import a [ManaBox](https://manabox.app/) collection export (`ManaBox_Collection.csv`). For each row: gets or creates the storage matching `Binder Name` (type = `Binder Type`); if `Binder Type` is `deck`, also gets or creates a deck with the same name (format defaults to `commander` — the export has no format column, so correct it afterwards with `PATCH /deck/:id` if needed); creates one card per physical copy (`Quantity`); links each card to the deck if applicable. ManaBox's export already carries the Scryfall ID directly, so cards are created even when Scryfall is unreachable: the route only looks them up to store their colors and primary type, and leaves those unknown (to fill in later with `POST /cards/refresh-details`) rather than failing — it never returns `502`. Scryfall allows one lookup of 75 cards every 500 ms, so a collection of several thousand cards takes about a minute.

**Form fields**

| Field | Required | Notes |
|---|---|---|
| `file` | Yes | The `ManaBox_Collection.csv` file. |
| `storage_id` | No | An existing storage for this account. When given, every card goes in it: `Binder Name` and `Binder Type` are ignored, and no storage or deck is created. Unknown storage → `400`. |

**Required CSV columns:** `Binder Name`, `Binder Type`, `Name`, `Set code`, `Scryfall ID`, `Collector number`, `Foil`, `Quantity` (column order doesn't matter).

---

### `POST /import/moxfield/collection`

Import a Moxfield "Export Collection" CSV. Moxfield's own export has no storage/binder concept, so every created card is assigned to an existing storage you choose — and no Scryfall ID either, so each row is resolved by (set, collector number) against the [Scryfall API](https://scryfall.com/docs/api/cards/collection).

**Form fields**

| Field | Required | Notes |
|---|---|---|
| `file` | Yes | The Moxfield collection CSV export. |
| `storage_id` | Yes | Must reference an existing storage for this account. Every imported card is assigned here. |

**Required CSV columns:** `Count`, `Name`, `Edition`, `Foil`, `Collector Number` (column order doesn't matter — Moxfield itself documents that only the names are checked).

---

### `POST /import/list`

Add a card list to the collection: one card is created per copy, in the storage given by `storage_id`, or in no storage without it. The list uses the same line format as [`POST /deck/:id/import`](#post-deckidimport) (see *Expected line format* there): `1 Sol Ring (SLD) 1011 *F*` creates that exact printing, foil; `4 Lightning Bolt` creates the printing Scryfall returns by default for that name. Section headers are skipped. A line that can't be resolved on Scryfall is skipped (counted in `cards_skipped`, noted in `warnings`); the rest is still imported.

**Form fields**

| Field | Required | Notes |
|---|---|---|
| `file` | Yes | The list, as plain text. |
| `storage_id` | No | An existing storage for this account. Unknown storage → `400`. |

---

### `POST /deck/:id/import`

Add a decklist to an **existing** deck — the front end creates the deck first (`POST /deck`), then calls this route. **It never creates cards in the collection**: each line is resolved against Scryfall, then:

- copies of that exact printing already in the collection are put in the deck (`cards_linked`). Copies that are in no deck yet and of the same finish (foil or not) are used first, but a copy already in another deck can be used too, since a card can belong to several decks. Copies already in this deck are never used twice, and each copy is used at most once per import;
- the copies the collection lacks are added to the deck's [pending cards](#pending-cards-deckidpending) (`cards_pending`), to add to the collection later with `POST /deck/:id/pending/commit`. A pending card of the same printing and finish already in the deck has its quantity raised instead.

`cards_created` and `decks_created` are always `0` for this route.

When `commander_from_first_line` is `true`, the first card line is treated as the deck's commander, but only if the deck has none yet. An owned commander becomes the deck's `commander_id`; one the collection lacks becomes its pending commander (`commander_pending_id`). A commander that can't be resolved on Scryfall is skipped (counted in `cards_skipped`, noted in `warnings`) and the rest of the list is still imported.

**Form fields**

| Field | Required | Notes |
|---|---|---|
| `file` | Yes | The decklist, as plain text. |
| `commander_from_first_line` | No | `true` or `false`. Defaults to `false`. |

**Errors** (besides the common ones): `400` invalid deck id · `404` deck not found.

**Expected line format:** `<quantity> <name> (<set code>) <collector number>[ *F*]`, e.g. `1 Sol Ring (SLD) 1011 *F*` — Moxfield's plain-text export (deck page → **More → Export → Plain Text**). `*E*`, etched, counts as foil, and collector numbers can contain dashes, like The List's `IMA-48`. Cards with two names (e.g. double-faced cards) keep both, separated by ` / `.

A plain list works too, one `<quantity> <name>` per line (`4 Lightning Bolt`, `1x Sol Ring`, `1 Fire // Ice`), and both formats can be mixed. A line without a printing is resolved by name on Scryfall: any printing of that card in the collection can go in the deck, and the copies the collection lacks are added as pending cards in the printing Scryfall returns by default. A split or double-faced card can be written with its full name (` / ` or ` // `) or its front face only. Section headers (`Commander`, `Companion`, `Deck`, `Mainboard`, `Sideboard`, `Maybeboard`, as in MTG Arena's format) are skipped, so every format from [`GET /deck/:id/export`](#get-deckidexport) can be imported back.

---

## Bulk Export

Two routes export your collection as a CSV file in the same format the matching [Bulk Import](#bulk-import) route reads — so round-tripping a collection out and back in is a no-op. Both are plain `GET` requests (no body): the response is the CSV file itself, not JSON, served with `Content-Type: text/csv; charset=utf-8` and a `Content-Disposition: attachment; filename="..."` header so a browser or HTTP client downloads it directly.

Both export the whole collection by default. With the optional `storage_id` query parameter (`GET /export/manabox?storage_id=4`), only the cards in that storage are exported; there's no filtering by deck (see [`GET /deck/:id/export`](#get-deckidexport) for that). Every physical copy of the same printing (same name, set, collector number and foil status) is collapsed into a single CSV row with a quantity/count column, the reverse of how importing that same row expands it back into that many individual cards.

**Errors common to both**
- `400` — `storage_id` isn't a positive integer
- `401` — unauthenticated, like every other route under this section
- `404` — `storage_id` doesn't reference an existing storage for this account
- `500` — unexpected failure reading the collection

---

### `GET /export/manabox`

Exports the account's collection (or one storage) as a ManaBox-compatible CSV (`ManaBox_Collection_export.csv`), matching the columns `POST /import/manabox` reads: `Binder Name, Binder Type, Name, Set code, Scryfall ID, Collector number, Foil, Quantity`.

Cards are grouped by storage, since storage (`Binder Name`/`Binder Type`) is ManaBox's only organizing concept. A card with no storage (`storage_id: null`) is grouped under a synthetic `Unsorted` / `binder` bucket rather than being dropped, sorted after every real storage. A card's deck membership is tracked independently of storage in Tamiyo (see [Deck ↔ Card relationship](#deck--card-relationship)) and isn't reflected here — only a storage whose own `type` is `deck` is exported as `Binder Type: deck`, mirroring exactly how `POST /import/manabox` derives deck membership on the way in.

### `GET /export/moxfield/collection`

Exports the account's collection (or one storage) as a Moxfield-compatible "Export Collection" CSV, matching the columns `POST /import/moxfield/collection` reads: `Count, Name, Edition, Foil, Collector Number`. Moxfield's own format has no storage concept at all, so unlike the ManaBox export, cards are grouped across every storage (and unsorted cards) with no distinction — the only way to see a card's storage is via `GET /cards`, not this export.

### `GET /deck/:id/export`

Exports **one deck** — not the whole collection — as plain text (`Content-Type: text/plain; charset=utf-8`), served as a download, in the format chosen with the `format` query parameter:

| `format` | Content | Download name |
|---|---|---|
| `moxfield` (default) | One line per printing, `1 Sol Ring (SLD) 1011 *F*`, the format Moxfield's deck import reads. | `Deck_moxfield.txt` |
| `plain` | One line per card name, every printing added up, `4 Lightning Bolt`, the commander first. | `Deck_list.txt` |
| `arena` | MTG Arena's format: a `Commander` section when the deck has one, then a `Deck` section, one line per card name. Split cards are written with ` // `. | `Deck_arena.txt` |

If the deck has a commander, it comes **first** in every format — in `moxfield`, that printing's line with its full quantity in the deck, not just the one physical card marked as commander — so importing the file back (which treats the first line as the commander when asked) reconstructs the same commander. Everything else is sorted alphabetically. Pending cards are included in every format, a pending commander first like an owned one. All three formats can be imported back with `POST /deck/:id/import`.

**Errors:** `400` `:id` is not a UUID or `format` is unknown · `401` unauthenticated · `404` deck not found

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
| `502 Bad Gateway` | A bulk import (see [Bulk Import](#bulk-import)) or deck insight (see [Deck Insights](#deck-insights)) couldn't reach or parse a response from the Scryfall API. |

### Validation rules summary

| Field | Rule |
|---|---|
| `scryfall_id` | Must be a valid UUID |
| `collector_number` | Must be an integer > 0 |
| `storage_id` (on cards) | Must reference an existing storage row, if provided |
| `commander_id` (on decks) | Must reference an existing card row, if provided |
| `password` / `new_password` | Minimum 8 characters |
| `refresh_token` | Single-use; reuse after rotation revokes all of the account's refresh tokens |
