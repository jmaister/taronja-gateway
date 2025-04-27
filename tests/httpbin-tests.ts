import http from 'k6/http';
import { check } from 'k6';

// Configuration for all test scenarios
export const options = {
  vus: 1, // Simulate 1 virtual user
  duration: '30s', // Run for 30 seconds (adjust as needed)
  iterations: 1, // Run each scenario once (as per your Loop Controllers)
};

// Base URL configuration (can be overridden in individual requests if needed)
const baseURL = 'http://127.0.0.1:8080';

// --- Routing / Auth group ---
export function routingAuthGroup() {

    // httpbin DELETE - Expecting a 200
    const resDelete = http.del(`${baseURL}/api/httpbin/delete`);
    check(resDelete, { 'status was 200': (r) => r.status === 200, 'delete_200': (r) => r.status === 200 });

    // httpbin GET - Expecting a 200
    const resGet = http.get(`${baseURL}/api/httpbin/get`);
    check(resGet, { 'status was 200': (r) => r.status === 200, 'get_200': (r) => r.status === 200 });

    // httpbin PATCH - Expecting a 200
    const resPatch = http.patch(`${baseURL}/api/httpbin/patch`);
    check(resPatch, { 'status was 200': (r) => r.status === 200, 'patch_200': (r) => r.status === 200 });

    // httpbin POST - Expecting a 200
    const resPost = http.post(`${baseURL}/api/httpbin/post`);
    check(resPost, { 'status was 200': (r) => r.status === 200, 'post_200': (r) => r.status === 200 });

    // httpbin PUT - Expecting a 200
    const resPut = http.put(`${baseURL}/api/httpbin/put`);
    check(resPut, { 'status was 200': (r) => r.status === 200, 'put_200': (r) => r.status === 200 });

    // httpbin GET image - Expecting a 200
    const resGetImage = http.get(`${baseURL}/api/httpbin/image`);
    check(resGetImage, { 'status was 200': (r) => r.status === 200, 'get_image_200': (r) => r.status === 200 });

    // httpbin GET redirect - Expecting a 302
    const resRedirect = http.get(`${baseURL}/api/httpbin/redirect/1`);
    check(resRedirect, { 'status was 302': (r) => r.status === 302, 'assert_redirect_302': (r) => r.status === 302 });
}




// Define the order in which the test groups will run
export default function () {
  routingAuthGroup();
}
