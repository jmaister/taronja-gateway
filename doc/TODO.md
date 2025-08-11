
# FIX!!!

* Token list show for every user. It must be filtered by user ID.

# TODO tasks for the project

* Return uptime on the /health endpoint
* Add a health check for the database connection
    * Open DB connections

## Stats

* CPU usage
* Memory usage

## Password

* Add password strength validation with rules
    * Minimum length
    * Special characters
    * Uppercase letters
* Implement password reset functionality
* Maximum password attempts before lockout
    * Unlock account button
* Implement password expiration policy
* 2FA (Two-Factor Authentication)
    * Email-based 2FA?
    * SMS-based 2FA?
    * Authenticator app-based 2FA
# Logs

Show logs in the dashboard
* Filter logs by date range
* Filter logs by severity level
* Search logs by keyword

# Token authentication

* Implement token-based authentication
* Token expiration and renewal
* Secure token storage
* Revoke tokens
* One user can have multiple tokens with different expiration times
* The tokens table do not get deleted, just set as inactive, also gets a count of how many times it has been used. The token is associated to a User.
* The users get authenticated by using the "Authorization: Bearer <token>" header.
* Middleware name is "TokenAuthMiddleware" and it should be applied to all routes except the login and registration routes.

