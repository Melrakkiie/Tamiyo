# The browser's refresh token lives only in a same-origin `httpOnly` cookie

`register`, `login` and `refresh` set `tamiyo_refresh_token` (`HttpOnly`, `SameSite=Strict`, `Secure`, `Path=<API_BASE_PATH>/auth`), and the front keeps only the 15-minute access token in memory, so an XSS cannot steal the 30-day token as it could from `localStorage`. The front sends `X-Refresh-Token-Transport: cookie` so responses omit `refresh_token` from the JSON too; without that header curl and scripts keep getting it. `SameSite=Strict` is what protects against CSRF, and it only works because the front and the API share one origin (`tamiyo.alangelier.com`, API under `/api`): splitting them onto different origins means redesigning this.

## Consequences

Reusing a rotated refresh token closes every session of the account (theft detection). A double reload can resend the old cookie, so a token replaced less than 10 s ago (`reuseGrace` in `internal/token/service.go`) is accepted once more by following `refresh_tokens.replaced_by` to the live token. A token revoked by logout or a password change is never in that grace.
