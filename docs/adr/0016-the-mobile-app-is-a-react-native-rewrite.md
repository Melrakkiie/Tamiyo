# The mobile app is a React Native (Expo) rewrite, not the front wrapped

`Tamiyo-app` is an Expo (React Native, TypeScript) app with its own native UI, even though `Tamiyo-front` is already an installable PWA and could have been wrapped in a WebView with Capacitor. The goal is an app that feels native on a phone (stacks, bottom sheets, gestures) before being on the stores, and the front's desktop-first DOM UI (Mantine, hand-made drag and drop, hover previews) would only ever feel like a website in a shell. Flutter and Kotlin were set aside because they share no code with the front; Expo keeps React, TanStack Query and openapi-fetch, and EAS Build gives iOS later without a Mac. Android comes first.

## Consequences

- The front's pure TypeScript (Scryfall query syntax and client, filters, grouping, deck entry mutations, the API wrappers) is copied into the app as screens need it, and allowed to drift; a heavy piece of business logic is moved into the API rather than kept in two clients.
- The app reaches feature parity with the front screen by screen; until then, what it lacks (sign-up, password reset, email confirmation, playtest, bulk imports) stays on the web.
- The app keeps its refresh token in the device's secure storage and sends it in the JSON body, which the API already supports for clients without `X-Refresh-Token-Transport: cookie`; ADR-0007 still governs the browser.
