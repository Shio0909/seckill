import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const seckillSuccessRate = new Rate('seckill_success_rate');
const seckillDuration = new Trend('seckill_duration', true);
const productListDuration = new Trend('product_list_duration', true);
const seckillSuccessCount = new Counter('seckill_success_count');
const seckillFailCount = new Counter('seckill_fail_count');

const BASE_URL = __ENV.BASE_URL || 'http://localhost:18080';

export const options = {
    stages: [
        { duration: '10s', target: 50 },
        { duration: '20s', target: 50 },
        { duration: '10s', target: 200 },
        { duration: '30s', target: 200 },
        { duration: '10s', target: 500 },
        { duration: '30s', target: 500 },
        { duration: '10s', target: 0 },
    ],
    thresholds: {
        http_req_failed: ['rate<0.10'],
        http_req_duration: ['p(95)<2000'],
        product_list_duration: ['p(95)<500'],
        seckill_duration: ['p(99)<3000'],
    },
    summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

export function setup() {
    console.log(`>>> Target: ${BASE_URL}`);

    const tokens = [];
    const userCount = 100;

    const runId = Date.now();
    for (let i = 0; i < userCount; i++) {
        const username = `k6_${runId}_${i}`;
        const phone = `13${String((runId + i) % 1000000000).padStart(9, '0')}`;

        http.post(
            `${BASE_URL}/api/v1/register`,
            JSON.stringify({ username, password: 'test123456', phone }),
            { headers: { 'Content-Type': 'application/json' } }
        );

        const loginRes = http.post(
            `${BASE_URL}/api/v1/login`,
            JSON.stringify({ username, password: 'test123456' }),
            { headers: { 'Content-Type': 'application/json' } }
        );

        if (loginRes.status === 200) {
            try {
                const body = JSON.parse(loginRes.body);
                if (body?.data?.token) {
                    tokens.push(body.data.token);
                }
            } catch (_) {}
        }
    }

    console.log(`>>> Got ${tokens.length} JWT tokens`);

    if (tokens.length > 0) {
        const warmupRes = http.post(`${BASE_URL}/api/v1/admin/products/1/warmup`, null, {
            headers: {
                Authorization: `Bearer ${tokens[0]}`,
            },
        });
        console.log(`>>> Warmup product 1: status=${warmupRes.status}`);
    }

    const prodRes = http.get(`${BASE_URL}/api/v1/products?page=1&page_size=10`);
    if (prodRes.status === 200) {
        try {
            const data = JSON.parse(prodRes.body);
            console.log(`>>> Product API OK, code: ${data.code}`);
        } catch (_) {}
    }

    return { tokens };
}

export default function (data) {
    const tokens = data.tokens;
    if (!tokens || tokens.length === 0) {
        return;
    }

    const token = tokens[Math.floor(Math.random() * tokens.length)];
    const authHeaders = {
        Authorization: `Bearer ${token}`,
    };

    group('product_list', function () {
        const page = Math.floor(Math.random() * 3) + 1;
        const res = http.get(`${BASE_URL}/api/v1/products?page=${page}&page_size=10`, {
            tags: { name: 'GET /products' },
        });
        productListDuration.add(res.timings.duration);
        check(res, { 'product list 200': (r) => r.status === 200 });
    });

    group('product_detail', function () {
        const pid = Math.floor(Math.random() * 10) + 1;
        const res = http.get(`${BASE_URL}/api/v1/products/${pid}`, {
            tags: { name: 'GET /products/:id' },
        });
        check(res, { 'product detail 200': (r) => r.status === 200 });
    });

    group('seckill', function () {
        const tokenRes = http.get(`${BASE_URL}/api/v1/idempotent/token`, {
            tags: { name: 'GET /idempotent/token' },
        });

        let idempotentToken = '';
        if (tokenRes.status === 200) {
            try {
                const body = JSON.parse(tokenRes.body);
                if (body?.data?.token) {
                    idempotentToken = body.data.token;
                }
            } catch (_) {}
        }

        const seckillHeaders = Object.assign({}, authHeaders);
        if (idempotentToken) {
            seckillHeaders['X-Idempotent-Token'] = idempotentToken;
        }

        const res = http.post(
            `${BASE_URL}/api/v1/seckill/buy`,
            { product_id: '1' },
            {
                headers: seckillHeaders,
                tags: { name: 'POST /seckill/buy' },
            }
        );

        seckillDuration.add(res.timings.duration);

        if (res.status === 200) {
            try {
                const body = JSON.parse(res.body);
                if (body.success === true || body.code === 0) {
                    seckillSuccessRate.add(1);
                    seckillSuccessCount.add(1);
                } else {
                    seckillSuccessRate.add(0);
                    seckillFailCount.add(1);
                }
            } catch (_) {
                seckillSuccessRate.add(0);
                seckillFailCount.add(1);
            }
        } else if (res.status === 429) {
            seckillSuccessRate.add(0);
        } else {
            seckillSuccessRate.add(0);
            seckillFailCount.add(1);
        }
    });

    sleep(Math.random() * 0.3);
}

export function teardown() {
    console.log('>>> Benchmark finished');
}
