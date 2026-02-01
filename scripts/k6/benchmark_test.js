// ========================================================================
// K6 真实压测脚本 — 秒杀系统性能基准测试
// ========================================================================
// 用法: k6 run scripts/k6/benchmark_test.js
// ========================================================================

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// 自定义指标
const seckillSuccessRate = new Rate('seckill_success_rate');
const seckillDuration = new Trend('seckill_duration', true);
const productListDuration = new Trend('product_list_duration', true);
const seckillSuccessCount = new Counter('seckill_success_count');
const seckillFailCount = new Counter('seckill_fail_count');

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// ========================================================================
// 压测场景：阶梯加压 (总计 ~2分钟)
// ========================================================================
export const options = {
    stages: [
        { duration: '10s', target: 50 },    // 预热
        { duration: '20s', target: 50 },    // 稳定
        { duration: '10s', target: 200 },   // 加压
        { duration: '30s', target: 200 },   // 高压
        { duration: '10s', target: 500 },   // 峰值
        { duration: '30s', target: 500 },   // 峰值持续
        { duration: '10s', target: 0 },     // 降压
    ],
    thresholds: {
        http_req_failed: ['rate<0.10'],
        http_req_duration: ['p(95)<2000'],
        'product_list_duration': ['p(95)<500'],
        'seckill_duration': ['p(99)<3000'],
    },
    summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

// ========================================================================
// Setup: 注册测试用户并获取 JWT Token
// ========================================================================
export function setup() {
    console.log(`>>> Target: ${BASE_URL}`);

    // 注册 + 登录，收集 Token
    const tokens = [];
    const userCount = 100;

    const runId = Date.now();
    for (let i = 0; i < userCount; i++) {
        const username = `k6_${runId}_${i}`;
        // 每次运行使用唯一手机号 (runId 后 7 位 + i)
        const phoneSuffix = String(runId % 10000000 * 100 + i).padStart(11, '1');
        const phone = phoneSuffix.substring(0, 11);

        // 注册 (JSON body)
        http.post(`${BASE_URL}/api/v1/register`, JSON.stringify({
            username: username,
            password: 'test123456',
            phone: phone,
        }), { headers: { 'Content-Type': 'application/json' } });

        // 登录 (JSON body) → 返回 {"token": "xxx", "message": "登录成功"}
        const loginRes = http.post(`${BASE_URL}/api/v1/login`, JSON.stringify({
            username: username,
            password: 'test123456',
        }), { headers: { 'Content-Type': 'application/json' } });

        if (loginRes.status === 200) {
            try {
                const body = JSON.parse(loginRes.body);
                if (body.token) {
                    tokens.push(body.token);
                }
            } catch (e) { }
        }
    }

    console.log(`>>> Got ${tokens.length} JWT tokens`);

    // 用第一个 token 预热商品库存
    if (tokens.length > 0) {
        const warmupRes = http.post(`${BASE_URL}/api/v1/admin/products/1/warmup`, null, {
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${tokens[0]}`,
            },
        });
        console.log(`>>> Warmup product 1: status=${warmupRes.status}`);
    }

    // 查看商品列表（验证连通性）
    const prodRes = http.get(`${BASE_URL}/api/v1/products?page=1&page_size=10`);
    if (prodRes.status === 200) {
        try {
            const data = JSON.parse(prodRes.body);
            console.log(`>>> Product API OK, code: ${data.code}`);
        } catch (e) { }
    }

    return { tokens: tokens };
}

// ========================================================================
// 主测试函数 — 每个 VU 循环执行
// ========================================================================
export default function (data) {
    const tokens = data.tokens;
    if (!tokens || tokens.length === 0) {
        return;
    }

    const token = tokens[Math.floor(Math.random() * tokens.length)];
    const authHeaders = {
        'Authorization': `Bearer ${token}`,
    };

    // ---------- 场景 1: 商品列表查询 (GET, 无需认证) ----------
    group('product_list', function () {
        const page = Math.floor(Math.random() * 3) + 1;
        const res = http.get(`${BASE_URL}/api/v1/products?page=${page}&page_size=10`, {
            tags: { name: 'GET /products' },
        });
        productListDuration.add(res.timings.duration);
        check(res, { 'product list 200': (r) => r.status === 200 });
    });

    // ---------- 场景 2: 商品详情 (GET, 无需认证) ----------
    group('product_detail', function () {
        const pid = Math.floor(Math.random() * 10) + 1;
        const res = http.get(`${BASE_URL}/api/v1/products/${pid}`, {
            tags: { name: 'GET /products/:id' },
        });
        check(res, { 'product detail 200': (r) => r.status === 200 });
    });

    // ---------- 场景 3: 秒杀抢购 (核心链路) ----------
    group('seckill', function () {
        // Step 1: 获取幂等 Token (GET, 无需认证)
        // 返回: {"code":200, "data":{"token":"uuid", ...}}
        const tokenRes = http.get(`${BASE_URL}/api/v1/idempotent/token`, {
            tags: { name: 'GET /idempotent/token' },
        });

        let idempotentToken = '';
        if (tokenRes.status === 200) {
            try {
                const body = JSON.parse(tokenRes.body);
                if (body.data && body.data.token) {
                    idempotentToken = body.data.token;
                }
            } catch (e) { }
        }

        // Step 2: 发起秒杀 (POST, form-urlencoded, 需 JWT + 幂等 Token)
        const seckillHeaders = Object.assign({}, authHeaders);
        if (idempotentToken) {
            seckillHeaders['X-Idempotent-Token'] = idempotentToken;
        }

        // 秒杀接口使用 c.PostForm() → application/x-www-form-urlencoded
        const res = http.post(
            `${BASE_URL}/api/v1/seckill/buy`,
            { product_id: '1' },  // k6: object → form-urlencoded
            {
                headers: seckillHeaders,
                tags: { name: 'POST /seckill/buy' },
            }
        );

        seckillDuration.add(res.timings.duration);

        if (res.status === 200) {
            try {
                const body = JSON.parse(res.body);
                if (body.success === true) {
                    seckillSuccessRate.add(1);
                    seckillSuccessCount.add(1);
                } else {
                    seckillSuccessRate.add(0);
                    seckillFailCount.add(1);
                }
            } catch (e) {
                seckillSuccessRate.add(0);
                seckillFailCount.add(1);
            }
        } else if (res.status === 429) {
            // 被限流 — 预期行为
            seckillSuccessRate.add(0);
        } else {
            seckillSuccessRate.add(0);
            seckillFailCount.add(1);
        }
    });

    // 模拟用户思考时间 0~300ms
    sleep(Math.random() * 0.3);
}

export function teardown(data) {
    console.log('>>> Benchmark finished');
}
