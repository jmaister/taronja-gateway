
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

# Constants data

## Countries

* Add a list of countries with their ISO codes
* Flag images for each country
* Country names in multiple languages
* Country calling codes
* Country time zones
* Country languages
* Country currencies

/countries -> [{...}, {...}, ...]
/countries/{countryCode} -> {
  "name": "Spain",
  "flag": "/_/countries/ES/flag",
  "isoCode": "ES",
  "iso3Code": "ESP",
  "callingCode": "+34",
  "timeZone": "Europe/Madrid",
  "languages": ["Spanish", "Catalan", "Galician", "Basque", "Valencian"],
  "currency": "EUR"
}
/countries/{countryCode}/flag -> image of the flag

## Languages

* Add a list of languages with their ISO codes
* Language names in multiple languages
* Language codes (ISO 639-1, ISO 639-2, ISO 639-3)

/languages -> [{...}, {...}, ...]
/languages/{languageCode} -> {
    "name": "Spanish",
    "isoCode": "es",
    "iso2Code": "es",
    "iso3Code": "spa",
    "nativeName": "Español",
    "rtl": false,
    "flag": "/_/languages/es/flag"
}
/languages/{languageCode}/flag -> image of the flag

## Timezones

* Add a list of time zones with their names and offsets
* Time zone names in multiple languages
* Time zone offsets in seconds
* Time zoned data: https://github.com/dmfilipenko/timezones.json/blob/master/timezones.json


