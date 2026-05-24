import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 5,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<750'],
  },
};

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:8080';

function jsonPost(path, body) {
  return http.post(`${BASE}${path}`, JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
  });
}

export default function () {
  const id = `${__VU}-${__ITER}`;
  const path = `/tmp/k6-${id}.txt`;
  const content = encoding.b64encode(`payload-${id}`);

  let res = jsonPost('/api/fs/write', { path, offset: 0, content });
  check(res, { 'write ok': (r) => r.status === 200 });

  res = http.get(`${BASE}/api/fs/read?path=${encodeURIComponent(path)}`);
  check(res, { 'read ok': (r) => r.status === 200 });

  res = http.get(`${BASE}/api/fs/stat?path=${encodeURIComponent(path)}`);
  check(res, { 'stat ok': (r) => r.status === 200 });

  res = http.get(`${BASE}/api/fs/ls?path=${encodeURIComponent('/tmp')}`);
  check(res, { 'ls ok': (r) => r.status === 200 });

  sleep(1);
}
