# taronja-gateway-react

Independent React and TypeScript SDK for Taronja Gateway.

This package is intended to move into the external clients repository later, but it lives in `/sdk` for now so the API can stabilize against the existing gateway and dashboard code.

## Included now

- A transport client for the gateway endpoints already used in the dashboard.
- A React auth provider with session polling, login, logout, and role helpers.
- Route guards and higher-order components for authenticated and admin-only UI.
- User profile helpers such as display name, avatar, and initials.

## Feature modules exposed by the client

- Session and user profile: `getCurrentUser`, `getLoginUrl`, `logout`
- Health and discovery: `getHealth`, `getOpenApiYaml`
- User administration: `listUsers`, `getUserById`, `createUser`
- Token management: `listTokens`, `getToken`, `createToken`, `deleteToken`
- Request analytics: `getRequestStatistics`, `getRequestDetails`
- Rate limiting: `getRateLimiterStats`, `getRateLimiterConfig`
- Counters: `getAvailableCounters`, `getAllUserCounters`, `getUserCounters`, `getUserCounterHistory`, `adjustUserCounters`

## Worth including next

- Optional TanStack Query adapters so consumers can use the same hooks as the dashboard without rebuilding query keys.
- React Router specific redirect helpers to remove direct `window.location` handling from consuming apps.
- OAuth provider login helpers if gateway login flows expose provider-specific routes or query parameters.
- Shared admin UI primitives for loading, unauthorized, and access-denied states when the package scope broadens beyond headless SDK behavior.

## Install

```bash
npm install taronja-gateway-react react
```

For local work inside this repository:

```bash
cd sdk
npm install
npm run typecheck
npm run build
```

Published releases are automated from the main repository release tag. The release workflow publishes this package to npm and mirrors the source package into `jmaister/taronja-gateway-clients`.

## Quick start

```tsx
import {
    TaronjaAuthProvider,
    RequireAdmin,
    createTaronjaClient,
    useTaronjaAuth,
} from 'taronja-gateway-react';

const client = createTaronjaClient({
    baseUrl: '/_',
});

function ProfileSummary() {
    const { currentUser, isAuthenticated, logout } = useTaronjaAuth();

    if (!isAuthenticated || !currentUser) {
        return <button onClick={() => window.location.assign(client.getLoginUrl())}>Login</button>;
    }

    return (
        <div>
            <p>{currentUser.username}</p>
            <button onClick={() => void logout()}>Logout</button>
        </div>
    );
}

export function App() {
    return (
        <TaronjaAuthProvider client={client}>
            <RequireAdmin fallback={<p>Admin access required.</p>}>
                <ProfileSummary />
            </RequireAdmin>
        </TaronjaAuthProvider>
    );
}
```
