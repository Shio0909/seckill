// ========================================================================
// 【重点学习】K6 压测脚本 - API 全链路测试
// ========================================================================
// 此脚本测试完整的用户业务流程：
// 注册 -> 登录 -> 浏览商品 -> 秒杀下单 -> 查询订单
//
// 📝 简历亮点：
// - 设计全链路压测场景，模拟真实用户行为
// - 使用 K6 的 group 和 check 功能
// - 实现多场景并行测试
// ========================================================================

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

// ========================================================================
// 自定义指标
// ========================================================================

const registerSuccess = new Rate('register_success');
const loginSuccess = new Rate('login_success');
const productListSuccess = new Rate('product_list_success');
const seckillSuccess = new Rate('seckill_success');
const orderQuerySuccess = new Rate('order_query_success');

// ========================================================================
// 测试配置
// ========================================================================

export const options = {
    // 【重点】多场景配置
    scenarios: {
        // 场景1：用户注册登录
        user_flow: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '20s', target: 50 },
                { duration: '1m', target: 50 },
                { duration: '10s', target: 0 },
            ],
            exec: 'userFlow',
            tags: { scenario: 'user' },
        },
        // 场景2：商品浏览
        product_browse: {
            executor: 'constant-vus',
            vus: 100,
            duration: '2m',
            exec: 'productBrowse',
            tags: { scenario: 'product' },
        },
        // 场景3：秒杀场景
        seckill_spike: {
            executor: 'ramping-arrival-rate',
            startRate: 0,
            timeUnit: '1s',
            preAllocatedVUs: 500,
            maxVUs: 1000,
            stages: [
                { duration: '10s', target: 100 },  // 10 秒达到 100 RPS
                { duration: '30s', target: 1000 }, // 30 秒达到 1000 RPS（峰值）
                { duration: '1m', target: 500 },   // 保持 500 RPS
                { duration: '20s', target: 0 },
            ],
            exec: 'seckillSpike',
            tags: { scenario: 'seckill' },
        },
    },

    thresholds: {
        // 全局阈值
        http_req_failed: ['rate<0.05'],
        http_req_duration: ['p(95)<1000'],
        // 各场景阈值
        'http_req_duration{scenario:user}': ['p(95)<500'],
        'http_req_duration{scenario:product}': ['p(95)<200'],
        'http_req_duration{scenario:seckill}': ['p(99)<2000'],
    },
};

// ========================================================================
// 配置
// ========================================================================

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// ========================================================================
// 场景1：用户注册登录流程
// ========================================================================

export function userFlow() {
    const username = `user_${randomString(8)}`;
    const password = '123456';
    const phone = `138${randomIntBetween(10000000, 99999999)}`;

    group('用户注册', function () {
        const res = http.post(`${BASE_URL}/api/user/register`, JSON.stringify({
            username: username,
            password: password,
            phone: phone,
        }), {
            headers: { 'Content-Type': 'application/json' },
        });

        const success = check(res, {
            'register status 200': (r) => r.status === 200,
            'register success': (r) => {
                const body = JSON.parse(r.body);
                return body.code === 0;
            },
        });
        registerSuccess.add(success);
    });

    sleep(1);

    group('用户登录', function () {
        const res = http.post(`${BASE_URL}/api/user/login`, JSON.stringify({
            username: username,
            password: password,
        }), {
            headers: { 'Content-Type': 'application/json' },
        });

        const success = check(res, {
            'login status 200': (r) => r.status === 200,
            'login has token': (r) => {
                const body = JSON.parse(r.body);
                return body.data && body.data.token;
            },
        });
        loginSuccess.add(success);
    });

    sleep(randomIntBetween(2, 5));
}

// ========================================================================
// 场景2：商品浏览
// ========================================================================

export function productBrowse() {
    group('获取商品列表', function () {
        const res = http.get(`${BASE_URL}/api/products?page=1&page_size=10`, {
            headers: { 'Content-Type': 'application/json' },
        });

        const success = check(res, {
            'product list status 200': (r) => r.status === 200,
        });
        productListSuccess.add(success);
    });

    sleep(1);

    group('获取商品详情', function () {
        const productId = randomIntBetween(1, 10);
        const res = http.get(`${BASE_URL}/api/products/${productId}`, {
            headers: { 'Content-Type': 'application/json' },
        });

        check(res, {
            'product detail status 200': (r) => r.status === 200,
        });
    });

    sleep(randomIntBetween(1, 3));
}

// ========================================================================
// 场景3：秒杀峰值测试
// ========================================================================

// 预先准备好的用户 Token（在 setup 中获取）
let tokens = [];

export function setup() {
    // 预先注册并登录一批用户
    const preparedTokens = [];

    for (let i = 0; i < 100; i++) {
        const username = `seckill_user_${i}`;
        const password = '123456';

        // 注册
        http.post(`${BASE_URL}/api/user/register`, JSON.stringify({
            username: username,
            password: password,
            phone: `139${String(i).padStart(8, '0')}`,
        }), {
            headers: { 'Content-Type': 'application/json' },
        });

        // 登录
        const res = http.post(`${BASE_URL}/api/user/login`, JSON.stringify({
            username: username,
            password: password,
        }), {
            headers: { 'Content-Type': 'application/json' },
        });

        if (res.status === 200) {
            const body = JSON.parse(res.body);
            if (body.data && body.data.token) {
                preparedTokens.push(body.data.token);
            }
        }
    }

    console.log(`准备了 ${preparedTokens.length} 个用户 Token`);
    return { tokens: preparedTokens };
}

export function seckillSpike(data) {
    const tokens = data.tokens || [];
    if (tokens.length === 0) {
        return;
    }

    const token = tokens[randomIntBetween(0, tokens.length - 1)];
    const productId = __ENV.PRODUCT_ID || 1;

    group('秒杀抢购', function () {
        const res = http.post(`${BASE_URL}/api/seckill`, JSON.stringify({
            product_id: parseInt(productId),
            quantity: 1,
        }), {
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`,
            },
            tags: { name: 'seckill' },
        });

        const success = check(res, {
            'seckill status 200': (r) => r.status === 200,
        });

        if (res.status === 200) {
            const body = JSON.parse(res.body);
            seckillSuccess.add(body.code === 0);
        } else {
            seckillSuccess.add(false);
        }
    });
}

// ========================================================================
// 【重点】K6 高级功能
// ========================================================================
//
// 1. 场景（Scenarios）：
//    - ramping-vus：逐步增加虚拟用户
//    - constant-vus：固定虚拟用户数
//    - ramping-arrival-rate：逐步增加请求速率（推荐秒杀场景）
//    - constant-arrival-rate：固定请求速率
//
// 2. 检查（Checks）：
//    - 验证响应状态码
//    - 验证响应内容
//    - 支持自定义检查函数
//
// 3. 阈值（Thresholds）：
//    - 定义性能指标阈值
//    - 不满足阈值测试会失败
//    - 支持按标签过滤
//
// 4. 自定义指标：
//    - Counter：计数器
//    - Gauge：瞬时值
//    - Rate：比率
//    - Trend：趋势（统计值）
//
// 5. 数据共享：
//    - SharedArray：VU 间共享只读数据
//    - setup/teardown：测试前后钩子
// ========================================================================

// ========================================================================
// 【重点】运行命令
// ========================================================================
//
// 基础运行：
// k6 run scripts/k6/api_test.js
//
// 指定场景运行：
// k6 run --scenario seckill_spike scripts/k6/api_test.js
//
// 输出到 Prometheus：
// k6 run --out experimental-prometheus-rw scripts/k6/api_test.js
//
// ========================================================================
