import http, { RefinedResponse, ResponseType } from 'k6/http';
import { check } from 'k6';
import encoding from 'k6/encoding';

// Configuration
export const options: { vus: number; duration: string; thresholds: Record<string, string[]> } = {
  vus: 1, // 1 virtual user
  duration: '5s', // Short duration for this example
  thresholds: {
    'http_req_duration': ['p(95)<200'], // 95% of requests should be below 200ms
    'checks{name:login_success}': ['rate>0.99'], // 99% of login checks should pass
    'checks{name:profile_success}': ['rate>0.99'], // 99% of profile checks should pass
  },
};

// Base URL
const baseURL: string = 'http://localhost:8080'; // Replace with your application's base URL
const username : string = 'admin';
const password : string = 'password';


export default function (): void {
  // 1. Login with Basic Authentication
  const loginRes: RefinedResponse<ResponseType> = http.get(`${baseURL}/_/auth/basic/login?redirect=/api/httpbin-auth/anything`, {
    auth: 'basic',
    headers: {
      Authorization: `Basic ${encoding.b64encode(`${username}:${password}`)}`,
    },
  });

  // Check if the login was successful (status code 2xx)
  check(loginRes, { 'Login successful': (r) => r.status >= 200 && r.status < 300, 'login_success': (r) => r.status >= 200 && r.status < 300 });

  // cookie is set from the login response
  const profileRes: RefinedResponse<ResponseType> = http.get(`${baseURL}/_/me`);

  // Check if accessing the protected resource was successful (status code 200)
  check(profileRes, { 'Access profile successful': (r) => r.status === 200, 'profile_success': (r) => r.status === 200 });
}

function logResponse(res: RefinedResponse<ResponseType>): void {
  // Combine response status and body in one log line (and make it error prone for the body)
  try {
    const body = res.body === null 
      ? '' 
      : typeof res.body === 'string'
        ? res.body
        : new TextDecoder().decode(res.body as ArrayBuffer);
    console.log(`Response: ${res.status} - ${JSON.stringify(JSON.parse(body))}`);
  } catch (e) {
    console.log(`Response: ${res.status} - Error parsing body: ${e}`);
    console.log(`Full Response Body: ${res.body}`); // print the full body in case of error
  }
}
