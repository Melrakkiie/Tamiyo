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

### `GET /auth/me/preferences` · `PATCH /auth/me/preferences`

The account's display preferences, saved so they follow the user from one device to another. **Requires `Authorization: Bearer <token>`.**

```json
{ "show_collection_in_decks": true }
```

| Field | Type | Default | Notes |
|---|---|---|---|
| `show_collection_in_decks` | bool | `true` | Whether deck pages show what the collection holds: in the user's own decks, which cards are pending, where each copy is stored; in other people's decks, which cards the user owns (see [`GET /deck/:id/ownership`](#get-deckidownership)). |

`GET` returns them (the defaults until something is saved); `PATCH` changes the fields present in the body and returns them all.

**Errors:** `400` no known field in the body, or a field of the wrong type · `401` missing/invalid token

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
| `group` | string | No | One of `type`, `color`, `mana`. Orders the cards by that group first (primary type and color groups in the same order as `sort=type` / `sort=color`, mana value rounded down), then by `sort` within each group, so a grouped display keeps the chosen sort. |
| `colors` | string | No | WUBRG letters, read with `color_mode`. Cards whose colors aren't known yet are left out. |
| `color_mode` | string | No | How `colors` is matched: `exact` (default, exactly these colors; an empty `colors=` keeps colorless cards), `include` (at least these colors), `within` (only colors among these, colorless included). |
| `mana_value` | number | No | Compared with `mana_value_op`. |
| `mana_value_op` | string | No | `eq` (default), `lt`, `lte`, `gt`, `gte`. |
| `type` | string | No | A card type: `Creature`, `Planeswalker`, `Battle`, `Instant`, `Sorcery`, `Artifact`, `Enchantment` or `Land`. Matches the primary type, and any type of the card's type line (an artifact creature is also an `Artifact`). |
| `subtype` | string | No | Part of the subtypes, after the dash of the type line (`elf`, `vehicle`…), case-insensitive. At most 50 characters. |
| `legal_in` | string | No | A Scryfall format (`commander`, `modern`, `pioneer`, `standard`, `pauper`, `legacy`, `vintage`…): only cards legal or restricted in it. |
| `color_count` | int | No | Number of colors, `0` to `5`. |
| `foil` | bool | No | `true` for foil cards only, `false` for non-foil ones. |
| `storage_type` | string | No | Only cards in a storage of this type (`binder`, `box`, `deckbox`…), case-insensitive. |

`type` (beyond the primary type), `subtype` and `legal_in` rely on each printing's type line and legalities, which the API fetches from Scryfall in the background: a newly added printing becomes searchable by them within a few minutes, and legalities are refreshed every day so bans show up.

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

`card_count` and `pending_count` only count the deck itself (its `main` board, see [Boards](#boards)): the sideboard and the cards being considered are left out. `pending_count` is the number of copies in the deck's pending list (see [Pending cards](#pending-cards-deckidpending)), not counted in `card_count`. `commander_scryfall_id` is read-only: the Scryfall id of the commander card, so a client can show its art without another call. `background_scryfall_id` is the art the user picked for the deck (`null` when none was chosen). `visibility` says who may see the deck: `private` (only its owner), `unlisted` (anyone with its link, the default) or `public` (anyone, and listed on its owner's profile). `id` is a random UUID generated when the deck is created, so deck ids can't be guessed from one another. It is also how anyone reaches the deck's read-only page: see [Shared decks](#shared-decks). The routes in this section still only serve the owner's own decks.

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

### `POST /deck/:id/duplicate`

Copy a deck into your decks: one of yours, or someone else's `public` or `unlisted` deck. The copy is a new **private** deck named `<name> (copie)`, with the same `format` and `background_scryfall_id`. Its cards are added the way [`POST /deck/:id/import`](#post-deckidimport) adds a Tamiyo deck file, so the copy keeps boards, commander and tags: your owned copies of each printing go in it, missing ones become pending cards. No body.

**Response `201 Created`**
```json
{ "deck_id": "9b2f…", "summary": { "cards_created": 0, "cards_linked": 12, "cards_pending": 88, "cards_skipped": 0, "storages_created": 0, "decks_created": 1 } }
```

If adding the cards fails (Scryfall unreachable, say), the copy is removed and the error returned.

**Errors:** `400` invalid id · `404` unknown deck, or someone else's private deck · `502` Scryfall unavailable

---

### `POST /deck/:id/collect`

Add the cards of a deck to your collection, one card per copy, in the exact printing and finish listed, in `storage_id` when given. Owned and [pending](#pending-cards-deckidpending) cards both count.

```json
{ "mode": "missing", "boards": ["main", "sideboard"], "storage_id": 4 }
```

| `mode` | Deck | Effect |
|---|---|---|
| `pending` | yours | Adds the pending cards of those boards and puts them in the deck, like `POST /deck/:id/pending/commit`. |
| `all` | yours | The same, plus one more copy of every card already in the deck, kept out of any deck. |
| `all` | someone else's `public` / `unlisted` | Every copy. |
| `missing` | someone else's `public` / `unlisted` | Only the copies you lack, counted by name whatever the printing (as [`GET /deck/:id/ownership`](#get-deckidownership) does). |

`boards` lists at least one of `main`, `sideboard`, `considering`.

**Response `200 OK`** — the [import summary](#bulk-import): `cards_created`, and `cards_linked` for the pending cards also put in the deck. Cards are handled one by one: on a failure, those already added stay added, and a pending card that fails is reported in `warnings`.

**Errors:** `400` invalid id or body, unknown `mode` or board, `pending` on someone else's deck or `missing` on yours, unknown `storage_id` · `404` unknown deck, or someone else's private deck

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

### Boards

Every card of a deck, owned or pending, sits on one of three boards, given in its `board` field:

| `board` | Meaning |
|---|---|
| `main` | The deck itself. The default everywhere. |
| `sideboard` | The deck's sideboard. |
| `considering` | Cards being considered for the deck (Moxfield's maybeboard). |

Only `main` counts in the deck's `card_count` and `pending_count`, its [statistics](#get-deckidstats), its deck size and singleton checks, and [comparisons](#get-deckidcompareother_id). Card legality is checked on all three boards. The commander always stays on `main`: moving it elsewhere is refused with `409`.

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
    "board": "main",
    "added": "2026-01-15 10:30:00",
    "updated": "2026-01-15 10:30:00"
  }
]
```

Every board is returned; each card tells its own `board`. Returns an empty array `[]` if the deck has no cards.

**Errors:** `400` invalid id, or `sort` is not one of the allowed values · `404` deck not found

---

### `PUT /deck/:id/cards/:card_id`

Add a card to a deck, or move it to another board. **Idempotent** — calling this again with the same `id`/`card_id` and board succeeds silently.

**Body (optional)**
```json
{ "board": "sideboard" }
```

`board` is `main` (the default, also without a body), `sideboard` or `considering` (see [Boards](#boards)). A card already in the deck is moved to that board.

**Response `204 No Content`**

**Errors**
- `400` — invalid `id` or `card_id` (not an integer), or unknown `board`
- `404` — deck not found (`"deck not found"`) or card not found (`"card not found"`)
- `409` — the card is the deck's commander and `board` isn't `main`
- `500` — unexpected server/database error

---

### `DELETE /deck/:id/cards/:card_id`

Remove a card from a deck. **Idempotent** — always returns success, whether or not the card was actually in the deck (or whether the deck/card exist at all). There's no distinct "not linked" error: the end state — "this card is not in this deck" — is what's guaranteed.

**Response `204 No Content`**

**Errors:** `400` invalid `id` or `card_id` · `500` unexpected server/database error

---

### Pending cards (`/deck/:id/pending`)

Cards wanted in a deck but not in the collection yet (typically picked on Scryfall from the deck page). They are stored per deck until they're added to the collection all at once.

- `GET /deck/:id/pending` — the list, sorted by name. Each item also tells what the collection already holds, outside this deck: `owned_copies` counts the cards of the same name (any printing, ` / ` and ` // ` alike, including copies in other decks), and `owned_same_printing` those of this exact printing. A client can tell a card missing from the collection (`0`) from one owned in another printing. Both are `0` in the responses of the routes below.
- `POST /deck/:id/pending` — add one. Body: `name`, `scryfall_id`, `set_code`, `collector_number` (required), `foil`, `quantity` (1–100, default 1), `mana_value`, `colors`, `card_type`, `color_identity` (same rules as `POST /cards`), `board` (`main` by default, see [Boards](#boards)). Returns `201` with the item. If the deck already has a pending card of the same printing, finish and board (other than its pending commander), its quantity is raised instead and that item is returned.
- `PATCH /deck/:id/pending/:pending_id` — change how many copies it stands for, its board, or both. Body: `{ "quantity": 12 }` (1–1000) and/or `{ "board": "considering" }`. Returns the item. Moved onto a board that already has a pending card of the same printing and finish, the two are merged and the remaining item is returned. The pending commander can't leave `main` (`409`).
- `DELETE /deck/:id/pending/:pending_id` — remove one. `204`, or `404` if it doesn't exist.
- `POST /deck/:id/pending/commit` — create every pending card in the collection (one card per copy, in `storage_id` if given), put each in the deck on the board it was pending on, and clear the list. Pending cards on the `considering` board are left out, unless given as `pending_id`. Body optional: `{ "storage_id": 4 }`, plus `"pending_id": 12` to only add that one pending card (all its copies), and `"quantity": 5` with it to only add that many copies (the pending card keeps the rest). Returns `{ "cards_created": 7 }`. Items are handled one by one: on a failure, those already handled stay done and the rest stay pending.
- `POST /deck/:id/import` — add a list of cards to this deck, as `multipart/form-data` with the list in a `file` field and an optional `commander_from_first_line` (default `false`). It never creates cards: owned copies go in the deck, missing ones become pending cards. Details, list format and response in [`POST /deck/:id/import`](#post-deckidimport) under Bulk Import.

Each item has a `board` field, like the deck's own cards.

**Errors:** `400` invalid id or body, unknown `storage_id` · `404` deck or pending card not found · `409` moving the pending commander off `main`

### Card tags (`/deck/:id/tags`)

Tags the owner puts on a deck's cards to help deckbuilding (`Ramp`, `Pioche`, `Removal`…). A tag belongs to a **card name within a deck**, not to a physical card: every copy of that card in the deck shares it, pending ones included, whatever the printing, and it stays when a copy is swapped for another printing. A card can have several tags. Each deck has its own tags.

Tags are trimmed (inner spaces collapsed) and must have 1 to 40 characters; a card has at most 20. They're matched case-insensitively within the deck: tagging a card `ramp` when the deck already uses `Ramp` stores `Ramp`. Tags of a card that has left the deck are kept (they come back if the card does) but aren't returned.

- `GET /deck/:id/tags` — `{ "tags": ["Pioche", "Ramp"], "cards": [{ "name": "Sol Ring", "tags": ["Ramp"] }] }`: every tag used in the deck, and the tagged cards (untagged ones are left out). Sorted alphabetically, ignoring case.
- `PUT /deck/:id/tags/cards` — replace a card's tags. Body: `{ "name": "Sol Ring", "tags": ["Ramp", "Artefact"] }` (`[]` removes them all). `name` is matched like the deck comparison does (case, ` / ` or ` // `). Returns the card's tags in the same shape as an entry of `cards`.
- `PATCH /deck/:id/tags` — rename a tag on every card. Body: `{ "from": "Ramp", "to": "Accélération" }`. Renaming into a tag the deck already uses merges them. `204`.
- `DELETE /deck/:id/tags?tag=Ramp` — remove a tag from every card. `204`.

Tagging changes the deck's `updated` date. Shared decks and the deck comparison show tags too (see [Shared decks](#shared-decks)).

**Errors:** `400` invalid id, a missing field, a tag that's blank or over 40 characters, more than 20 tags · `404` deck not found, card not in the deck, unknown tag

### Display settings (`/deck/:id/view`)

How the owner last displayed the deck's cards, so the deck page opens the same way next time. It's the owner's own setting: shared decks don't use it. Changing it doesn't change the deck's `updated` date.

- `GET /deck/:id/view` — `{ "grouping": "type", "sort": "mana_value", "collapsed_boards": ["considering"] }`. Those are also the values before anything is saved.
- `PUT /deck/:id/view` — save them. Body: `grouping` (`type`, `color`, `mana`, `storage`, `tag`, or `null` for no grouping), `sort` (required, same values as the `sort` of `GET /deck/:id/cards`) and `collapsed_boards`, the [boards](#boards) folded on the deck page (`sideboard`, `considering`, or neither; `["considering"]` when left out). Returns the saved settings.

**Errors:** `400` invalid id, missing `sort`, unknown `grouping` or `sort` · `404` deck not found

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

Every board is checked card by card (see [Boards](#boards)), and each issue about a card tells its `board`, but the singleton rule and the deck size only look at `main`. The deck size is checked counting every copy of `main` (the commander and pending cards included): exactly 100 cards for `commander`, `brawl`, `duel`, `paupercommander`, `predh` and `gladiator`, exactly 60 for `oathbreaker` and `standardbrawl`, at least 60 for the other constructed formats (`standard`, `pioneer`, `modern`, `legacy`, `vintage`, `pauper`, `premodern`, `explorer`, `historic`, `timeless`, `alchemy`, `oldschool`, `penny`, `future`). Other formats get no size check. A wrong size is reported first, as an issue with an empty `card_name` and a reason like `deck size: 87 cards, commander requires exactly 100`.

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

Read-only access to public decks, and to a deck by its id. **No authentication required** for the six `/shared/decks` routes. Deck ids are random UUIDs, so an unlisted deck can only be found by someone who was given its id or link. Only `public` and `unlisted` decks resolve: a private deck, an unknown id or anything that isn't a UUID all answer `404`, so the response never tells whether a private deck exists.

The six `/shared/decks` routes share a per-client-IP limit, separate from the auth one: 60 requests per 60-second window by default (`SHARE_RATE_LIMIT_MAX` / `SHARE_RATE_LIMIT_WINDOW_SECONDS`). Exceeding it returns `429` with a `Retry-After` header, as described in [Rate limiting](#rate-limiting).

### `GET /shared/decks`

Browse every `public` deck, of every user. Unlisted and private decks are never listed.

**Query parameters** (all optional)

| Param | Description |
|---|---|
| `q` | Part of the deck's name, case-insensitive. |
| `format` | The deck's format, case-insensitive (`commander`, `modern`…). |
| `commander` | Part of the commander's name. |
| `card` | Part of the name of a card on the deck's `main` [board](#boards), owned or pending. |
| `owner` | Part of the owner's display name. |
| `colors` | The deck's color identity: letters among `W`, `U`, `B`, `R`, `G` (`UG`), or `C` for colorless decks. |
| `color_mode` | How `colors` is matched: `exact` (default), `include` (at least these colors) or `within` (at most these colors). Ignored with `colors=C`. |
| `color_count` | Number of colors in the identity, `0` to `5`. |
| `sort` | `updated`, `name`, `added` or `card_count`, `-` prefix for descending. Defaults to `-updated`. |
| `page`, `limit` | Pagination: `limit` defaults to 24, at most 100. |

A deck's color identity is its commander's, or, without a commander (or when the commander's is unknown), the union of the identities of the cards on its `main` board. Text filters can't exceed 100 characters.

**Response `200 OK`**
```json
{
  "data": [
    {
      "id": "6f0d3c5e-8a51-4c0b-9b1e-2d7c4a3f9e10",
      "name": "Kess Commander",
      "format": "commander",
      "background_scryfall_id": null,
      "commander_scryfall_id": "a0b4c5ad-14f7-4bcb-9a59-6c0ac4f1a5e0",
      "commander_name": "Kess, Dissident Mage",
      "color_identity": "UBR",
      "card_count": 100,
      "owner": { "id": "4b8a0a9e-2f1c-4c8e-9d3a-1e2f3a4b5c6d", "display_name": "Tamiyo", "avatar_scryfall_id": null },
      "added": "2026-01-15 10:30:00",
      "updated": "2026-01-15 10:30:00"
    }
  ],
  "page": 1,
  "limit": 24,
  "total": 1,
  "total_pages": 1
}
```

`card_count` counts the `main` board, pending cards included, like [`GET /shared/decks/:id`](#get-shareddecksid). `color_identity` is in `WUBRG` order, empty for a colorless deck.

**Errors:** `400` invalid parameter · `429` rate limit exceeded

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
      "commander": false,
      "tags": ["Terrain"]
    }
  ]
}
```

The owner's copies and the deck's pending cards are merged into one list, one entry per printing, finish and [board](#boards) with its `quantity`, sorted by name. Each entry has its `board` (`main`, `sideboard` or `considering`). Nothing tells owned and pending cards apart, and storages, proxies and card ids are left out. The commander is always its own entry with `commander: true`. `tags` are the owner's [card tags](#card-tags-deckidtags), shared by every printing of the card. `card_count` counts every card of `main`, pending ones included. `owner` has the shape of [`GET /users/:id`](#get-usersid).

**Errors:** `404` unknown, private or malformed id · `429` rate limit exceeded

### `GET /shared/decks/:id/legality`

Same report as [`GET /deck/:id/legality`](#get-deckidlegality), without `card_id` on the issues.

**Errors:** `400` the deck's format isn't one Scryfall recognizes · `404` as above · `429` rate limit exceeded · `502` Scryfall unreachable

### `GET /shared/decks/:id/stats`

Same statistics as [`GET /deck/:id/stats`](#get-deckidstats).

**Errors:** `400` the deck's format isn't one Scryfall recognizes · `404` as above · `429` rate limit exceeded · `502` Scryfall unreachable

### `GET /shared/decks/:id/export`

The deck exported as [`GET /deck/:id/export`](#get-deckidexport) does, with the same `format` and `tags` parameters, response and download names.

**Errors:** `400` unknown `format`, or `tags` isn't a boolean · `404` as above · `429` rate limit exceeded

### `GET /shared/decks/:id/compare/:other_id`

Compares two `public` or `unlisted` decks, as [`GET /deck/:id/compare/:other_id`](#get-deckidcompareother_id) does, for anyone. `mine` is always `false`.

**Errors:** `404` either deck is unknown, malformed or private · `429` rate limit exceeded

### `GET /deck/:id/compare/:other_id`

Compare two decks card by card. **Requires authentication** (it is not rate-limited like the routes above). Each deck can be one of yours, whatever its visibility, or someone else's `public` or `unlisted` deck; the same deck can be given twice.

Cards are matched **by name only**: printings and finishes are ignored, and a split or double-faced card matches whether it's written with ` / ` or ` // `. Owned copies and pending cards both count, on the `main` [board](#boards) only.

**Response `200 OK`**
```json
{
  "deck": {
    "id": "6f0d3c5e-8a51-4c0b-9b1e-2d7c4a3f9e10",
    "name": "Kess Commander",
    "format": "commander",
    "visibility": "private",
    "owner": { "id": "4b8a0a9e-2f1c-4c8e-9d3a-1e2f3a4b5c6d", "display_name": "Tamiyo", "avatar_scryfall_id": null },
    "mine": true,
    "card_count": 100
  },
  "other": { "id": "…", "name": "Kess Spellslinger", "format": "commander", "visibility": "unlisted", "owner": { "id": "…", "display_name": "Alice", "avatar_scryfall_id": null }, "mine": false, "card_count": 100 },
  "common": [
    { "name": "Island", "scryfall_id": "fc3f6a8f-0b5e-4b5f-9a1a-1d4b0e3c4c2f", "mana_value": 0, "colors": "", "card_type": "Land", "color_identity": "U", "quantity": 12, "other_quantity": 9, "commander": false, "other_commander": false, "tags": ["Terrain"], "other_tags": [] }
  ],
  "only_in_deck": [
    { "name": "Kess, Dissident Mage", "scryfall_id": "…", "mana_value": 4, "colors": "UBR", "card_type": "Creature", "color_identity": "UBR", "quantity": 1, "other_quantity": 0, "commander": true, "other_commander": false, "tags": ["Commandant"], "other_tags": [] }
  ],
  "only_in_other": []
}
```

- `common`: names in both decks, with how many copies each one has (`quantity` for `:id`, `other_quantity` for `:other_id`) — they can differ.
- `only_in_deck` / `only_in_other`: names in only one of them (the other quantity is `0`).
- Every list is sorted by name and always present, possibly empty. `scryfall_id`, `mana_value`, `colors`, `card_type` and `color_identity` come from one of the matching printings, preferring `:id`'s. `commander` / `other_commander` tell whether the card is that deck's commander, and `tags` / `other_tags` are its [tags](#card-tags-deckidtags) in each deck. `mine` tells whether the deck belongs to the caller.

**Errors:** `400` an id isn't a UUID · `401` unauthenticated · `404` a deck doesn't exist, or is someone else's private deck

### `GET /deck/:id/ownership`

How many copies of each card of a deck the **signed-in user** has in their collection. **Requires authentication.** The deck can be one of theirs or someone else's `public` or `unlisted` deck. Every card of the deck is listed once, every board included, and matched **by name only**, whatever the printing or finish (` / ` and ` // ` alike); every copy of the collection counts, including copies in other decks.

**Response `200 OK`**
```json
{ "cards": [{ "name": "Island", "owned": 0 }, { "name": "Sol Ring", "owned": 3 }] }
```

Sorted by name.

**Errors:** `401` unauthenticated · `404` unknown, malformed, or someone else's private deck

---

## Bulk Import

Five routes import cards in bulk, instead of one `POST /cards` call per card: a collection export from a third-party tool, a plain card list, a [Tamiyo file](#tamiyo-format), or a decklist. The collection, list and Tamiyo imports create cards (and, for ManaBox and Tamiyo, storages; for ManaBox, decks); the deck import fills an existing deck with cards already in the collection and never creates any. All five are `multipart/form-data` requests (not JSON) with the file itself in a field named `file`.

A bulk import never fails outright just because some rows couldn't be resolved: a request that parses successfully always returns `200 OK` with a summary of what happened, including a `warnings` list for any row that was skipped (card not found on Scryfall, a duplicate, a transient error). The import only fails as a whole (non-`200`) when the file itself can't be parsed, a referenced `storage_id` doesn't exist, or Scryfall couldn't be reached at all.

**Response shape (all five routes)**
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

**Errors common to all five**
- `400` — no `file` field, or the file couldn't be parsed (wrong columns, malformed line) — message explains what's wrong
- `400` — a `storage_id` field doesn't reference an existing storage for this account
- `502` — Scryfall (used to resolve Moxfield rows — see below) couldn't be reached or returned an unexpected response after retrying; a rate-limited (`429`) response from Scryfall is retried automatically (honoring its `Retry-After` header when present) before this is returned
- `401` — unauthenticated, like every other route under this section

---

### `POST /import/manabox`

Import a [ManaBox](https://manabox.app/) collection export (`ManaBox_Collection.csv`). For each row: gets or creates the storage matching `Binder Name` (type = `Binder Type`, except that a `deck` binder becomes a `deckbox` storage, so it isn't mistaken for a deck); if `Binder Type` is `deck`, also gets or creates a deck with the same name (format defaults to `commander` — the export has no format column, so correct it afterwards with `PATCH /deck/:id` if needed); creates one card per physical copy (`Quantity`); links each card to the deck if applicable. ManaBox's export already carries the Scryfall ID directly, so cards are created even when Scryfall is unreachable: the route only looks them up to store their colors and primary type, and leaves those unknown (to fill in later with `POST /cards/refresh-details`) rather than failing — it never returns `502`. Scryfall allows one lookup of 75 cards every 500 ms, so a collection of several thousand cards takes about a minute.

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

### `POST /import/tamiyo`

Import a [Tamiyo file](#tamiyo-format) of kind `collection` or `storage` into the collection. One card is created per copy, with its printing, finish and proxy status. Without `storage_id`, each card goes in the storage named in the file: an existing storage with exactly that name, or a new one of the file's type (`binder` when the file doesn't say). With `storage_id`, every card goes in that storage and the file's storages are ignored. Each printing is looked up on Scryfall by its id, to store its details; one Scryfall doesn't know is skipped (counted in `cards_skipped`, noted in `warnings`).

**Form fields**

| Field | Required | Notes |
|---|---|---|
| `file` | Yes | The Tamiyo file. |
| `storage_id` | No | An existing storage for this account. Unknown storage → `400`. |

**Errors** (besides the common ones): `400` the file is a Tamiyo `deck` file (it is imported with [`POST /deck/:id/import`](#post-deckidimport)).

---

### `POST /deck/:id/import`

Add a decklist to an **existing** deck — the front end creates the deck first (`POST /deck`), then calls this route. **It never creates cards in the collection**: each line is resolved against Scryfall, then:

- copies of that exact printing already in the collection are put in the deck (`cards_linked`). Copies that are in no deck yet and of the same finish (foil or not) are used first, but a copy already in another deck can be used too, since a card can belong to several decks. Copies already in this deck are never used twice, and each copy is used at most once per import;
- the copies the collection lacks are added to the deck's [pending cards](#pending-cards-deckidpending) (`cards_pending`), to add to the collection later with `POST /deck/:id/pending/commit`. A pending card of the same printing, finish and board already in the deck has its quantity raised instead.

Each card goes on the [board](#boards) of the section it's listed under: `main` by default, `sideboard` under a `Sideboard` header, `considering` under `Maybeboard` or `Considering` (see the list format below).

`cards_created` and `decks_created` are always `0` for this route.

When `commander_from_first_line` is `true`, the first card line of `main` is treated as the deck's commander, but only if the deck has none yet. An owned commander becomes the deck's `commander_id`; one the collection lacks becomes its pending commander (`commander_pending_id`). A commander that can't be resolved on Scryfall is skipped (counted in `cards_skipped`, noted in `warnings`) and the rest of the list is still imported.

**Form fields**

| Field | Required | Notes |
|---|---|---|
| `file` | Yes | The decklist, as plain text, or a [Tamiyo](#tamiyo-format) `deck` file. |
| `commander_from_first_line` | No | `true` or `false`. Defaults to `false`. Ignored for a Tamiyo file. |

A file starting with `{` is read as a [Tamiyo](#tamiyo-format) `deck` file: its cards go on their own boards, with their exact printing and finish, owned copies and missing ones handled as above. Its commander becomes the deck's commander if the deck has none yet. Its tags, when it has any, are added to the deck's [tags](#card-tags-deckidtags) (a card keeps the tags it already had); a tag that can't be set, for a card that couldn't be added, is noted in `warnings`. The file's `deck` name and format are not applied: the deck keeps its own.

**Errors** (besides the common ones): `400` invalid deck id, or a Tamiyo file of kind `collection` or `storage` · `404` deck not found.

**Expected line format:** `<quantity> <name> (<set code>) <collector number>[ *F*]`, e.g. `1 Sol Ring (SLD) 1011 *F*` — Moxfield's plain-text export (deck page → **More → Export → Plain Text**). `*E*`, etched, counts as foil, and collector numbers can contain dashes, like The List's `IMA-48`. Cards with two names (e.g. double-faced cards) keep both, separated by ` / `.

A plain list works too, one `<quantity> <name>` per line (`4 Lightning Bolt`, `1x Sol Ring`, `1 Fire // Ice`), and both formats can be mixed. A line without a printing is resolved by name on Scryfall: any printing of that card in the collection can go in the deck, and the copies the collection lacks are added as pending cards in the printing Scryfall returns by default. A split or double-faced card can be written with its full name (` / ` or ` // `) or its front face only. Section headers, with or without a trailing `:` and in any case, are not cards: they choose the [board](#boards) of the lines below them. `Commander`, `Companion`, `Deck` and `Mainboard` mean `main`, `Sideboard` means `sideboard`, `Maybeboard` and `Considering` mean `considering`. Lines before any header go on `main`. Every format from [`GET /deck/:id/export`](#get-deckidexport) can be imported back. `POST /import/list` reads the same format, ignoring the sections.

---

## Bulk Export

Three routes export your collection: two as a CSV file in the same format the matching [Bulk Import](#bulk-import) route reads — so round-tripping a collection out and back in is a no-op. Both are plain `GET` requests (no body): the response is the CSV file itself, not JSON, served with `Content-Type: text/csv; charset=utf-8` and a `Content-Disposition: attachment; filename="..."` header so a browser or HTTP client downloads it directly.

Both export the whole collection by default. With the optional `storage_id` query parameter (`GET /export/manabox?storage_id=4`), only the cards in that storage are exported; there's no filtering by deck (see [`GET /deck/:id/export`](#get-deckidexport) for that). Every physical copy of the same printing (same name, set, collector number and foil status) is collapsed into a single CSV row with a quantity/count column, the reverse of how importing that same row expands it back into that many individual cards.

**Errors common to both**
- `400` — `storage_id` isn't a positive integer
- `401` — unauthenticated, like every other route under this section
- `404` — `storage_id` doesn't reference an existing storage for this account
- `500` — unexpected failure reading the collection

---

### Tamiyo format

Tamiyo's own file format: a JSON document that keeps everything the other formats lose (storages and their type, proxies, the exact printing, a deck's boards, commander and tags), so a collection, a storage or a deck can be exported and imported back, into the same account or another one. Every file starts with the format version and what it holds:

```json
{
  "tamiyo": 1,
  "kind": "collection",
  "exported_at": "2026-10-09T09:30:00Z",
  "storages": [{ "name": "Classeur bleu", "type": "binder" }],
  "cards": [
    { "name": "Sol Ring", "scryfall_id": "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", "set_code": "sld", "collector_number": "1011",
      "foil": false, "proxy": false, "quantity": 2, "storage": "Classeur bleu" },
    { "name": "Duress", "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "m19", "collector_number": "94",
      "foil": false, "proxy": false, "quantity": 1, "storage": null }
  ]
}
```

- `kind` is `collection` (the whole collection), `storage` (one storage) or `deck`.
- In a `collection` or `storage` file, each card entry is one printing, finish, proxy status and storage with its `quantity`. `storage` names one of the `storages`, or is `null` for a card in no storage.
- A `deck` file has a `deck` object (`name`, `format`) instead of `storages`. Its card entries carry a `board` (`main`, `sideboard` or `considering`) instead of `storage` and `proxy`, and the commander's entry has `"commander": true`. Owned and pending cards are merged. It has a `tags` list (`[{ "name": "Sol Ring", "tags": ["Ramp"] }]`) when it was exported with its tags.
- `quantity` is between 1 and 1000. A file whose `tamiyo` version is newer than the API's is refused.

### `GET /export/manabox`

Exports the account's collection (or one storage) as a ManaBox-compatible CSV (`ManaBox_Collection_export.csv`), matching the columns `POST /import/manabox` reads: `Binder Name, Binder Type, Name, Set code, Scryfall ID, Collector number, Foil, Quantity`.

Cards are grouped by storage, since storage (`Binder Name`/`Binder Type`) is ManaBox's only organizing concept. A card with no storage (`storage_id: null`) is grouped under a synthetic `Unsorted` / `binder` bucket rather than being dropped, sorted after every real storage. A card's deck membership is tracked independently of storage in Tamiyo (see [Deck ↔ Card relationship](#deck--card-relationship)) and isn't reflected here — only a storage whose own `type` is `deckbox` is exported as `Binder Type: deck`, mirroring exactly how `POST /import/manabox` derives deck membership on the way in.

### `GET /export/moxfield/collection`

Exports the account's collection (or one storage) as a Moxfield-compatible "Export Collection" CSV, matching the columns `POST /import/moxfield/collection` reads: `Count, Name, Edition, Foil, Collector Number`. Moxfield's own format has no storage concept at all, so unlike the ManaBox export, cards are grouped across every storage (and unsorted cards) with no distinction — the only way to see a card's storage is via `GET /cards`, not this export.

### `GET /export/tamiyo`

Exports the account's collection, or one storage with `storage_id`, as a [Tamiyo file](#tamiyo-format) (`Tamiyo_Collection.json`, `Content-Type: application/json; charset=utf-8`): of kind `collection`, or `storage` with `storage_id`. Only the storages holding exported cards are listed (and the exported storage itself, even empty). `POST /import/tamiyo` reads it back.

### `GET /deck/:id/export`

Exports **one deck** — not the whole collection — as plain text (`Content-Type: text/plain; charset=utf-8`), served as a download, in the format chosen with the `format` query parameter:

| `format` | Content | Download name |
|---|---|---|
| `moxfield` (default) | One line per printing, `1 Sol Ring (SLD) 1011 *F*`, the format Moxfield's deck import reads. | `Deck_moxfield.txt` |
| `plain` | One line per card name, every printing added up, `4 Lightning Bolt`, the commander first. | `Deck_list.txt` |
| `arena` | MTG Arena's format: a `Commander` section when the deck has one, then a `Deck` section, one line per card name. Split cards are written with ` // `. | `Deck_arena.txt` |
| `tamiyo` | A [Tamiyo file](#tamiyo-format) of kind `deck`, served as `application/json`. Add `tags=true` to include the deck's tags. | `Deck_tamiyo.json` |

`tags` (`true` or `false`, default `false`) only applies to `tamiyo`. The rest of this section is about the three text formats.

The sideboard follows the deck in a section of its own in every format: `SIDEBOARD:` in `moxfield`, `Sideboard` in `plain` and `arena`. The cards being considered come last, under `MAYBEBOARD:` in `moxfield` and `Maybeboard` in `plain`; `arena` leaves them out. Each section is written only when it has cards, after a blank line.

If the deck has a commander, it comes **first** in every format — in `moxfield`, that printing's line with its full quantity in the deck, not just the one physical card marked as commander — so importing the file back (which treats the first line as the commander when asked) reconstructs the same commander. Everything else is sorted alphabetically. Pending cards are included in every format, a pending commander first like an owned one. All three formats can be imported back with `POST /deck/:id/import`.

**Errors:** `400` `:id` is not a UUID, `format` is unknown or `tags` isn't a boolean · `401` unauthenticated · `404` deck not found

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
