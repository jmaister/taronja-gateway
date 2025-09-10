
# FIX!!!

All fixed.

# TODO tasks for the project

* Multi-login users
    - Handle "User account already exists with a different login method."

# Likes

Create a like feature for any content based on their ID. Each like is associated with the logged user.
Likes can't be repeated.

POST /api/likes/<content_name>/<content_id> - Creates a like
GET /api/likes/<content_name>/<content_id> - Returns the count
DELETE /api/likes/<content_name>/<content_id> - Removes this like
...
GET /api/users/likes - Returns the list of likes for the logged user

DB table:
likes
- id (PK)
- user_id (FK)
- content_name
- content_id
- created_at
- updated_at
- deleted_at

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
