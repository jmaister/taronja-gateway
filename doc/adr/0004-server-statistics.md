# 4. Server Statistics

Date: 2025-06-04

## Status

2025-06-04 Started

## Context

The gateway collects statistics about the endpoints served, such as the number of requests, response times, and error rates. This information is crucial for monitoring the performance and reliability of the gateway.

## Decision

Create a new middleware for collecting statistics about the endpoints served. Store the data in a table in the database, which can be queried to generate reports and dashboards.

Fields stored for each request-response:
- `method`: HTTP method (GET, POST, etc.)
- `path`: URL path of the request
- `status`: HTTP status code of the response
- `response_time`: Time taken to process the request in milliseconds
- `timestamp`: Time when the request was received
- `client_ip`: IP address of the client making the request
- `user_agent`: User agent string of the client
- `response_size`: Size of the response in bytes
- `error`: Any error message if the request failed
- `device`: Device type of the client (e.g., mobile, desktop)
- `user_id`: ID of the user making the request, if authenticated
- `session_id`: ID of the session, if applicable

Then, create a dashboard in the webapp to visualize this data, allowing administrators to see trends over time, identify bottlenecks, and monitor the health of the gateway.

## Consequences

Getting information about what is happening in the gateway in real-time and over time.

