# Taronja Gateway admin dashboard

React + Vite app served by the gateway under `/_/admin/`.

```bash
npm install
npm run dev      # Vite dev server (run the gateway separately)
npm run build    # production build in dist/, embedded in the tg binary
npx vitest run   # tests
npm run lint     # oxlint (.oxlintrc.json)
npm run gen:api  # regenerate src/apiclient from the OpenAPI spec
```
