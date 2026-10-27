# Browser end-to-end test

Drives the real built web app in headless Chromium against a real server, with
three users in separate browser profiles. It checks: registration, 1:1 chat live
over WebSocket, image + file transfer, logout and sign-in again, a brand-new
device using only the account key, wrong-key rejection, groups, key rotation when
a member leaves, a simulated server-side public-key swap, and finally that no
secret appears anywhere in the server's storage.

```bash
mkdir -p /tmp/tc-e2e && cd /tmp/tc-e2e && npm i playwright-core
(cd web && npm i && npx vite build)
(cd server && CGO_ENABLED=0 go build -o /tmp/tc-e2e/tc .)
e2e/run.sh
```

Uses `/usr/bin/chromium` (edit `executablePath` in `e2e.mjs` if yours differs).
