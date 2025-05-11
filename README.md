# Taronja Gateway

Taronja Gateway is an API and application gateway.

It serves as an entry point for your API server and your frontend application, handling routing, authentication, sessions, and many more features, leaving your application code clean and focused on business logic.

# Features

Features table, shows what is implemented and what is planned.

| Feature                      | Status   |
|------------------------------|----------|
| Gateway                      | ✅       |
| Authentication               | ✅       |
| Authentication: Basic        | ✅       |
| Authentication: OAuth2       | ✅       |
| - OAuth2: GitHub             | ✅       |
| - OAuth2: Google             | ✅       |
| Authentication: JWT          | 🚧       |
| Authorization using RBAC     | 🚧       |
| Sessions                     | ✅       |
| User management              | 🚧       |
| Rate Limiting                | 🚧       |
| Circuit breaker              | 🚧       |
| Caching                      | 🚧       |
| Logging                      | ✅       |
| Monitoring                   | 🚧       |
| Load Balancing               | 🚧       |
| more...                      | 🚧       |

## Commands

The Taronja Gateway CLI provides the following commands:

*   **Run the Gateway:**
    ```bash
    ./tg run --config ./sample/config.yaml
    ```
    This command starts the Taronja API Gateway using the configuration file specified by the `--config` flag.

*   **Add a new user:**
    ```bash
    ./tg adduser <username> <email> <password>
    ```
    This command creates a new user in the database with the provided username, email, and password.

