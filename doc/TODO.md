
# FIX!!!

- ...

# TODO tasks for the project

* Multi-login users
    - Handle "User account already exists with a different login method."
    - There is a user profile page that shows linked social login methods, user can add or remove linked accounts
    - Social login should be stored in a different table (e.g., UserLogins), not in the Users table
    - Link social login accounts to existing user profiles
    - The main user identifier is the email address
    - Users must edit their user profile name when they create a new account, coming from any social login method, stored in the Users table
    - Users can link multiple social login methods to a single account
    - On the user profile page, users can manage linked social login methods: link, unlink social accounts even if they do no match with the email address
    - Example:
      - User: user@example.com
        - Linked social logins:
          - Google: user@gmail.com
          - Facebook: user@example.com
          - Twitter: user123
      - User: anotheruser@example.com
        - Linked social logins:
          - Google: anotheruser@gmail.com

## User profile

- Gateway provides a page for user profile
- The configuration allows for a list of fields and their types


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


