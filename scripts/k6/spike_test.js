// ========================================================================
// 【核心】EventHub 抢票峰值压测脚本
// ========================================================================
// 模拟真实的抢票场景：
// - 开票前预热：用户提前涌入
// - 开票瞬间：QPS 瞬时飙升到 10000+
// - 持续抢购：高并发持续
// - 售罄下降：活动结束后流量骤降
// ========================================================================

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter, Gauge } from 'k6/metrics';
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

// ========================================================================
// 自定义指标
// ========================================================================

// 抢票成功率
const ticketSuccessRate = new Rate('ticket_success_rate');
// 库存不足（正常失败）
const stockEmptyRate = new Rate('stock_empty_rate');
// 限流率
const rateLimitedRate = new Rate('rate_limited_rate');
// 抢票延迟
const ticketDuration = new Trend('ticket_duration_ms');
// 成功抢票数
const ticketSuccessCount = new Counter('ticket_success_count');
// 当前并发
const currentVUs = new Gauge('current_vus');

// ========================================================================
// 压测配置 - 峰值场景
// ========================================================================

export const options = {
    // 峰值压测场景
    scenarios: {
        // 场景1: 开票前预热（用户提前登录、查看详情）
        warmup: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '30s', target: 200 },  // 30s 加到 200 用户
            ],
            exec: 'warmupPhase',
            startTime: '0s',
            gracefulStop: '5s',
        },
        
        // 场景2: 开票瞬间峰值（模拟整点开票）
        spike: {
            executor: 'ramping-arrival-rate',
            startRate: 0,
            timeUnit: '1s',
            preAllocatedVUs: 2000,
            maxVUs: 5000,
            stages: [
                { duration: '5s', target: 1000 },   // 5s 内到 1000 RPS
                { duration: '10s', target: 5000 },  // 10s 内到 5000 RPS (峰值)
                { duration: '30s', target: 5000 },  // 保持峰值 30s
                { duration: '15s', target: 2000 },  // 下降到 2000 RPS
                { duration: '30s', target: 2000 },  // 保持
                { duration: '10s', target: 500 },   // 售罄后下降
                { duration: '20s', target: 100 },   // 长尾流量
            ],
            exec: 'spikePhase',
            startTime: '30s',  // 在预热后开始
            gracefulStop: '30s',
        },
        
        // 场景3: 订单查询（后台压力）
        order_check: {
            executor: 'constant-arrival-rate',
            rate: 100,
            timeUnit: '1s',
            duration: '2m30s',
            preAllocatedVUs: 50,
            maxVUs: 100,
            exec: 'orderCheck',
            startTime: '1m',  // 抢票开始后再查询
        },
    },
    
    // 阈值设置
    thresholds: {
        // 抢票接口 P99 < 2s (允许峰值时略慢)
        'http_req_duration{name:seckill}': ['p(99)<2000'],
        // 预热接口 P95 < 200ms
        'http_req_duration{name:warmup}': ['p(95)<200'],
        // 错误率 < 5% (不含限流和库存不足)
        http_req_failed: ['rate<0.05'],
        // 限流率允许高一些（保护系统）
        rate_limited_rate: ['rate<0.50'],
    },
    
    // 汇总统计
    summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

// ========================================================================
// 配置
// ========================================================================

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const EVENT_ID = __ENV.EVENT_ID || 1;

// 预生成的 Token 池（实际测试需要在 setup 中获取）
let tokenPool = [];

// ========================================================================
// Setup：测试前准备
// ========================================================================

export function setup() {
    console.log('╔══════════════════════════════════════════════════════════╗');
    console.log('║         EventHub 抢票系统 - 峰值压力测试                  ║');
    console.log('╚══════════════════════════════════════════════════════════╝');
    console.log(`🎯 目标地址: ${BASE_URL}`);
    console.log(`🎫 测试活动ID: ${EVENT_ID}`);
    console.log('');
    
    // 批量注册用户并获取 Token
    const tokens = [];
    const batchSize = 200;
    
    console.log(`📝 正在注册 ${batchSize} 个测试用户...`);
    
    for (let i = 0; i < batchSize; i++) {
        const username = `spike_user_${Date.now()}_${i}`;
        const payload = JSON.stringify({
            username: username,
            password: '123456',
            phone: `139${String(i).padStart(8, '0')}`,
        });
        
        // 注册
        http.post(`${BASE_URL}/api/user/register`, payload, {
            headers: { 'Content-Type': 'application/json' },
            timeout: '5s',
        });
        
        // 登录获取 Token
        const loginRes = http.post(`${BASE_URL}/api/user/login`, JSON.stringify({
            username: username,
            password: '123456',
        }), {
            headers: { 'Content-Type': 'application/json' },
            timeout: '5s',
        });
        
        if (loginRes.status === 200) {
            try {
                const body = JSON.parse(loginRes.body);
                if (body.data && body.data.token) {
                    tokens.push(body.data.token);
                }
            } catch (e) {}
        }
    }
    
    console.log(`✅ 成功获取 ${tokens.length} 个用户 Token`);
    console.log('');
    
    return { tokens, eventId: EVENT_ID };
}

// ========================================================================
// 阶段1：预热（查看活动详情、库存）
// ========================================================================

export function warmupPhase(data) {
    const eventId = data.eventId;
    
    group('预热_查看活动', function() {
        // 获取活动详情
        const detailRes = http.get(`${BASE_URL}/api/events/${eventId}`, {
            tags: { name: 'warmup' },
            timeout: '5s',
        });
        
        check(detailRes, {
            '活动详情返回200': (r) => r.status === 200,
        });
        
        // 查询库存
        const stockRes = http.get(`${BASE_URL}/api/events/${eventId}/stock`, {
            tags: { name: 'warmup' },
            timeout: '5s',
        });
        
        check(stockRes, {
            '库存查询返回200': (r) => r.status === 200,
        });
    });
    
    sleep(randomIntBetween(1, 3));
}

// ========================================================================
// 阶段2：抢票峰值
// ========================================================================

export function spikePhase(data) {
    const tokens = data.tokens;
    const eventId = data.eventId;
    
    if (!tokens || tokens.length === 0) {
        console.error('没有可用的Token');
        return;
    }
    
    // 随机选择用户
    const token = tokens[randomIntBetween(0, tokens.length - 1)];
    
    // 生成幂等性 Key
    const idempotencyKey = `spike_${Date.now()}_${randomIntBetween(1, 999999)}`;
    
    const startTime = Date.now();
    
    const res = http.post(`${BASE_URL}/api/seckill/${eventId}`, JSON.stringify({
        ticket_type: randomIntBetween(1, 3),  // 随机票档
        quantity: 1,
    }), {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
            'X-Idempotency-Key': idempotencyKey,
        },
        tags: { name: 'seckill' },
        timeout: '10s',
    });
    
    const duration = Date.now() - startTime;
    ticketDuration.add(duration);
    currentVUs.add(__VU);
    
    // 分析响应
    let success = false;
    let rateLimited = false;
    let stockEmpty = false;
    
    if (res.status === 200) {
        try {
            const body = JSON.parse(res.body);
            if (body.code === 0 || body.code === 200) {
                success = true;
                ticketSuccessCount.add(1);
            } else if (body.code === 4001 || body.message?.includes('库存')) {
                stockEmpty = true;
            }
        } catch (e) {}
    } else if (res.status === 429) {
        rateLimited = true;
    }
    
    ticketSuccessRate.add(success);
    stockEmptyRate.add(stockEmpty);
    rateLimitedRate.add(rateLimited);
    
    check(res, {
        '抢票请求有效': (r) => r.status === 200 || r.status === 429 || r.status === 400,
        '响应时间<2s': (r) => r.timings.duration < 2000,
    });
}

// ========================================================================
// 阶段3：订单查询
// ========================================================================

export function orderCheck(data) {
    const tokens = data.tokens;
    
    if (!tokens || tokens.length === 0) {
        return;
    }
    
    const token = tokens[randomIntBetween(0, tokens.length - 1)];
    
    group('订单查询', function() {
        const res = http.get(`${BASE_URL}/api/orders`, {
            headers: {
                'Authorization': `Bearer ${token}`,
            },
            tags: { name: 'order_query' },
            timeout: '5s',
        });
        
        check(res, {
            '订单查询成功': (r) => r.status === 200,
        });
    });
    
    sleep(0.5);
}

// ========================================================================
// Teardown：测试结束汇总
// ========================================================================

export function teardown(data) {
    console.log('');
    console.log('╔══════════════════════════════════════════════════════════╗');
    console.log('║                    压测完成                              ║');
    console.log('╚══════════════════════════════════════════════════════════╝');
}

// ========================================================================
// 默认函数（供单独测试）
// ========================================================================

export default function(data) {
    spikePhase(data);
}
