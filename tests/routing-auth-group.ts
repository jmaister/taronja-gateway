import http from 'k6/http';
import { check } from 'k6';

export let options = {
    vus: 1,
    iterations: 1,
};

export default function () {
    let res = http.get('http://127.0.0.1:8080/_____/asdfsfasdf/sf/asdfsd');
    check(res, {
        'is status 404': (r) => r.status === 404,
    });

    res = http.get('http://127.0.0.1:8080/api/v1/posts/1');
    check(res, {
        'is status 200': (r) => r.status === 200,
    });

    res = http.get('http://127.0.0.1:8080/api/v2/posts/1');
    check(res, {
        'is status 401': (r) => r.status === 401,
    });

}