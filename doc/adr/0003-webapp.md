# 1. Webapp route configuration

Date: 2025-05-29

## Status

Done

## Context and Problem Statement

The gateway needs a webapp for managing its live configuration, like user management, and other settings.

## Decision Outcome

Technologies:

- React
- React Router
- TypeScript
- Tailwind CSS
- Vite

The webapp is in the `webapp` directory of the repository, and the URL is "/_/admin".

As it is an SPA, the gateway has to redirect all requests to the webapp, except for the API routes to the index page on /_/admin/index.html.

Configuration in the webapp, based on React Router, is as follows:

**vite.config.ts**: has to define a `base` URL for the webapp, which is "/_/admin/".

```typescript
// https://vite.dev/config/
export default defineConfig({
    base: '/_/admin/',
    plugins: [...],
    ...
});
```

**App.tsx**: the main entry point of the webapp, which uses React Router to define the routes.

```tsx
import { BrowserRouter as Router, Route, Switch } from 'react-router-dom';

...
<BrowserRouter basename="/_/admin">
```

**gateway.go**: has to redirect all the requests under /_/admin/** to /_/admin/index.html, except for the API routes.

