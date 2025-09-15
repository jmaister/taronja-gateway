# 1. purpose

Date: 2025-04-26

## Status

Done

## Context

The main purpose of the gateway is to reduce the implementation time for an API and a frontend application by providing common features that are needed in most applications.

It provides common features, so that the application code can focus on business logic.

The application under the gateway will not need to implement authentication, authorization, routing, sessions, and other features.

## Decision

Create it and make it available as an open source project.

For the moment, the monetization strategy is not clear yet, but I make it with the idea of makint it profitable in the future.

The license will be MIT.

Using Go language with the standard library for HTTP server and router.

## Consequences

The gateway will be available as an open source project.
