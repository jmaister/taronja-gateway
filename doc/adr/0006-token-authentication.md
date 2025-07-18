# 6. token-authentication

Date: 2025-07-18

## Status

Accepted

## Context

The gateway needs to provide token-based authentication for API access, allowing applications to authenticate users without requiring session cookies. This enables stateless authentication for API clients and mobile applications.

## Decision

### Token Authentication System

- **Immutable Tokens**: Tokens cannot be updated or deleted after creation
- **Lazy Expiration**: Tokens only expire when accessed after their expiration date
- **Permanent Retention**: Expired tokens are kept in the database permanently for audit purposes
- **Usage Tracking**: Each token access increments a usage counter
- **Multiple Tokens**: Users can have multiple active tokens with different expiration times
- **Bearer Authentication**: Tokens are provided via `Authorization: Bearer <token>` header
- **Unified Middleware**: Token authentication is integrated into `SessionMiddleware` as a fallback when cookie authentication fails

### Token Lifecycle

1. **Creation**: Tokens are created with a hash, expiration date, and associated user
2. **Usage**: Each access increments usage count and validates expiration
3. **Expiration**: Tokens are marked as expired only when accessed past expiration date
4. **Retention**: All tokens remain in database for audit and security analysis

### Authentication Flow

1. **Primary**: Cookie-based session authentication is attempted first
2. **Fallback**: If cookie auth fails, Bearer token authentication is tried
3. **Unified**: Both methods use the same `SessionMiddleware` for consistent handling
4. **Context**: Successful authentication enriches request context with session data

## Consequences

- Simplified token management with no update/delete operations
- Enhanced security through permanent audit trail
- Stateless authentication suitable for API clients
- Reduced database maintenance overhead (no token cleanup required)
- **Unified authentication flow** providing flexible client support (cookies or tokens)
- **Single point of authentication** through SessionMiddleware eliminates duplicate logic
