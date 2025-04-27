import http from 'k6/http';
import { check } from 'k6';

export let options = {
    vus: 1,
    duration: '1s',
};

export default function () {
    let res = http.get('http://localhost:8080/_/health');
    check(res, {
        'is status 200': (r) => r.status === 200,
    });

}