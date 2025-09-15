# Architecture Decision Record: Multi-Provider Login

## Status

Proposed

## Context

We need to support authentication via multiple external providers (e.g., Google, GitHub, etc.) in the Taronja Gateway. This will allow users to log to their account using several identity providers, improving flexibility and user experience.

## Decision

Implement multi-provider login by integrating with OAuth2 and OpenID Connect providers. The authentication flow will be handled in the gateway, mapping external identities to internal user accounts.

Users are identified by their email address.
- If a user logs in with an external provider for the first time, a new internal user account will be created and linked to the external identity.
- If a user logs in with an external provider that is already linked to an internal account, the existing account will be used.
- If a user logs in with an external provider using an email address that is already associated with a different internal account, the new login information will be linked to the previous user account.


## Consequences
- Users can authenticate using various external providers.
- Increased complexity in authentication logic and user mapping.
- Need to securely store provider credentials and handle callback endpoints.
- Requires updates to session management and user repository to support external identities.

## Related Decisions
- ADR-0001: User Management Strategy
- ADR-0005: Session Management

## References
- [OAuth2 RFC](https://datatracker.ietf.org/doc/html/rfc6749)
- [OpenID Connect](https://openid.net/connect/)
- [api/taronja-gateway-api.yaml](../../api/taronja-gateway-api.yaml)
