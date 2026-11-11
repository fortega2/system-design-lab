// Constant arrival rate against the phase 0 app: 90% reads, 10% writes.
//
// Open model on purpose: requests keep arriving at RATE per second no matter
// how slow the app gets, so a slowdown piles up in-flight requests instead of
// quietly lowering the load (coordinated omission).
//
//   make load                          # 200 req/s for 5m
//   make load RATE=1000 DURATION=2m
//   make load RATE=30000 MAX_VUS=2000  # past the ceiling without capping k6
import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://app:8080';
const RATE = parseInt(__ENV.RATE || '200', 10);
const DURATION = __ENV.DURATION || '5m';
const MAX_VUS = parseInt(__ENV.MAX_VUS || '500', 10);
const SEED_ITEMS = 100;
const WRITE_RATIO = 0.1;

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };

export const options = {
  discardResponseBodies: true,
  scenarios: {
    constant: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: Math.min(50, MAX_VUS),
      // Every hung request holds a VU. Past maxVUs k6 reports
      // dropped_iterations: saturation seen from the client side.
      maxVUs: MAX_VUS,
    },
  },
  // Reference values for a healthy system; they never abort the run.
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<100'],
  },
};

export function setup() {
  const ids = [];
  for (let i = 0; i < SEED_ITEMS; i++) {
    const res = http.post(
      `${BASE_URL}/items`,
      JSON.stringify({ name: `seed-${i}`, value: 'k6' }),
      { ...JSON_HEADERS, responseType: 'text' },
    );
    if (res.status !== 201) {
      throw new Error(`seeding items: POST /items returned ${res.status}`);
    }
    ids.push(res.json('id'));
  }
  return { ids };
}

export default function (data) {
  if (Math.random() < WRITE_RATIO) {
    const res = http.post(
      `${BASE_URL}/items`,
      JSON.stringify({ name: 'k6', value: `${__VU}-${__ITER}` }),
      { ...JSON_HEADERS, tags: { name: 'POST /items' } },
    );
    check(res, { 'created 201': (r) => r.status === 201 });
    return;
  }

  const id = data.ids[Math.floor(Math.random() * data.ids.length)];
  // One metric series for every id instead of one per URL.
  const res = http.get(`${BASE_URL}/items/${id}`, { tags: { name: 'GET /items/{id}' } });
  check(res, { 'read 200': (r) => r.status === 200 });
}
