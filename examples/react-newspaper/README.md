# React Newspaper example

A React SPA behind Taronja Gateway: `/` is public, `/premium/*` needs a login.
Needs `tg` on your PATH and Node.js. Run every command from this folder (the
user database is created here).

```bash
npm install
npm run build
tg adduser john john@example.com smith   # a user to log in with
```

## Gateway serves the build

```bash
tg run --config taronja-gateway.yaml
```

## Gateway proxies to the Vite preview server

Terminal 1:

```bash
npm run preview          # serves the build on http://localhost:4173
```

Terminal 2:

```bash
tg run --config taronja-gateway-http.yaml
```

Open <http://localhost:8080>, and log in as `john` / `smith` to see
`/premium/`.
