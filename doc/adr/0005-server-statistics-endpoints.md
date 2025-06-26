# 4. Server Statistics

Date: 2025-06-26

## Status

2025-06-26 Started

## Context

The gateway collects statistics about the endpoints served, now we have to create endpoints to retrieve this information and show it in the webapp. This information is crucial for monitoring the performance and reliability of the gateway.

## Decision

Create new endpoints, all of them have parameters to filter the results (optionally) by start and end dates.

- `GET /_/api/statistics/requests`: Retrieve statistics about requests made to the gateway.
  - Parameters:
    - `start_date`: Optional start date for filtering results.
    - `end_date`: Optional end date for filtering results.
  - Response:
    - Number of requests
    - Number of requests by status code
    - Average response time
    - Average response size
    - Number of requests by country
    - Number of requests by device type (e.g., mobile, desktop)
    - Number of requests by platform (e.g., Android, iOS, Windows)
    - Number of requests by browser (e.g., Chrome, Firefox, Safari)

## Consequences

Be able to see the statistics in the webapp.

