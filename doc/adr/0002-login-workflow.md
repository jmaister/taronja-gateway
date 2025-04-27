# 2. login-workflow

Date: 2025-04-26

## Status

Accepted

## Context

The authentication methods need to keep the session alive, so that the user can be authenticated in the frontend application.

## Decision

The login workflow for Basic authentication will be as follows:

```mermaid
sequenceDiagram
    participant User
    participant Frontend
    participant Gateway
    participant DB

    User->>Frontend: Login with credentials
    Frontend->>Gateway: Authenticate
    Gateway->>DB: Validate credentials
    DB-->>Gateway: User data
    Gateway-->>Frontend: Set Token in a cookie
```

Cookie name is `taronja-session`, and it will be set with the `HttpOnly` and `Secure` flags.

Session will be stored in the database, and the token will be a JWT token, stating the user id and the expiration date.
The expiration date will be set to 1 hour, and the session will be renewed every time the user makes a request to the gateway.

On each request, the gateway will check if the token is valid and not expired. The user will be checked in the database, and if the user is not found, deactivated, or expired, the user will be logged out.

The user will be logged out by deleting the session from the database and removing the cookie from the browser.

### User table

```sql
CREATE TABLE user (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    active BOOLEAN DEFAULT TRUE
);
```

Password will be hashed using bcrypt.

### Session table

```sql
CREATE TABLE session (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
```

## Consequences

Keep the user logged in for a period of time, so that the user can access the application without having to login again.
