
# FIX!!!

All fixed.

# TODO tasks for the project

* use "https://ip.guide/" to get IP info, it's free and no API key needed.

# Print OAuth callback URL in the console when starting the server.

That way, when setting up OAuth providers, the developer can easily copy the callback URL.

# Stats

Button to refresh stats.

# Request identifier and tracing

OpenTelemetry + Open Telemetry server: https://opentelemetry.io/

Should we add X-Request-ID to all requests and responses for tracing?
Are there any other ways to trace requests?
Are there libraries that already handle tracing?
Do libraries stick to an specific tracing product or standard?

## Logs

Show logs in the dashboard
* Filter logs by date range
* Filter logs by severity level
* Search logs by keyword

## Stats

* CPU usage
* Memory usage

# Health check 

GET /_/health - Returns 200 OK if the service is running

Health check configuration for the routes configured in the gateway:
- Add a URL to check
- Check interval
- Timeout
- Expected response code
- Show the results in /_/health



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


## Storage

* Add a storage system for user-uploaded files
* Implement file versioning
* Allow users to manage their files (upload, delete, rename)
* Integrate with cloud storage providers (e.g., AWS S3, Google Cloud Storage)


