import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 10, // Number of virtual users
  duration: '30s', // Test duration
};

const username = 'your-username'; // Replace with actual username
const password = 'your-password'; // Replace with actual password
const headers = {
  'Authorization': `Basic ${window.btoa(`${username}:${password}`)}`,
  'Content-Type': 'application/json',
  'Custom-Header': 'CustomValue', // Add any additional headers here
};

export default function () {
  const url = 'http://localhost:8080/_/health'; // Replace with your actual health endpoint
  const res = http.get(url, { headers });

  // Validate response
  check(res, {
    'status is 200': (r) => r.status === 200,
    'response time is < 200ms': (r) => r.timings.duration < 200,
  });

  sleep(1); // Wait for 1 second between requests
}
