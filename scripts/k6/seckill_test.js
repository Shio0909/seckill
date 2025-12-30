// ========================================================================
// 【重点学习】K6 压测脚本 - 秒杀系统性能测试
// ========================================================================
// K6 是一个现代化的负载测试工具，使用 JavaScript 编写测试脚本
// 官网：https://k6.io/
//
// 📝 简历亮点：
// - 使用 K6 进行高并发压力测试
// - 设计不同负载场景（阶梯加压、峰值测试）
// - 分析性能指标（QPS、延迟、错误率）
//
// 🔥 面试高频：
// Q: 如何进行压力测试？需要关注哪些指标？
// A: 关键指标：
//    1. QPS/TPS：每秒请求/事务数
//    2. 延迟分布：P50、P95、P99
//    3. 错误率：4xx、5xx 比例
//    4. 资源使用：CPU、内存、网络
//    5. 业务指标：库存一致性、超卖率
//
// Q: 如何设计压测场景？
// A: 1. 基准测试：单用户单请求，测试系统基础性能
//    2. 阶梯加压：逐步增加并发，找到系统瓶颈
//    3. 峰值测试：模拟秒杀瞬时高并发
//    4. 稳定性测试：长时间中等负载，检测内存泄漏
// ========================================================================

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// ========================================================================
// 自定义指标
// ========================================================================

// 秒杀成功率
const seckillSuccessRate = new Rate('seckill_success_rate');
// 秒杀请求延迟
const seckillDuration = new Trend('seckill_duration');
// 秒杀成功数
const seckillSuccessCounter = new Counter('seckill_success_total');
// 秒杀失败数
const seckillFailCounter = new Counter('seckill_fail_total');

// ========================================================================
// 测试配置
// ========================================================================

// 【重点】压测场景配置
// 使用 stages 定义负载变化曲线
export const options = {
    // 场景一：阶梯加压测试
    stages: [
        { duration: '30s', target: 100 },   // 30s 内加到 100 用户
        { duration: '1m', target: 100 },    // 保持 100 用户 1 分钟
        { duration: '30s', target: 500 },   // 30s 内加到 500 用户
        { duration: '1m', target: 500 },    // 保持 500 用户 1 分钟
        { duration: '30s', target: 1000 },  // 30s 内加到 1000 用户
        { duration: '2m', target: 1000 },   // 保持 1000 用户 2 分钟
        { duration: '30s', target: 0 },     // 30s 内减少到 0
    ],

    // 【重点】阈值设置 - 不满足则测试失败
    thresholds: {
        // HTTP 请求失败率 < 1%
        http_req_failed: ['rate<0.01'],
        // P95 延迟 < 500ms
        http_req_duration: ['p(95)<500'],
        // 秒杀接口 P99 延迟 < 1000ms
        'http_req_duration{name:seckill}': ['p(99)<1000'],
        // 秒杀成功率 > 5%（库存有限时）
        seckill_success_rate: ['rate>0.05'],
    },

    // 输出设置
    summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

// ========================================================================
// 测试数据
// ========================================================================

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// 模拟用户数据
const users = [];
for (let i = 1; i <= 1000; i++) {
    users.push({
        username: `testuser${i}`,
        password: '123456',
        phone: `138${String(i).padStart(8, '0')}`,
    });
}

// 测试商品 ID
const PRODUCT_ID = __ENV.PRODUCT_ID || 1;

// ========================================================================
// Setup：测试前准备
// ========================================================================

export function setup() {
    console.log('========== 压测开始 ==========');
    console.log(`目标地址: ${BASE_URL}`);
    console.log(`测试商品ID: ${PRODUCT_ID}`);

    // 注册测试用户并获取 Token
    const tokens = [];

    for (let i = 0; i < Math.min(100, users.length); i++) {
        const user = users[i];

        // 注册用户
        let res = http.post(`${BASE_URL}/api/user/register`, JSON.stringify(user), {
            headers: { 'Content-Type': 'application/json' },
        });

        // 登录获取 Token
        res = http.post(`${BASE_URL}/api/user/login`, JSON.stringify({
            username: user.username,
            password: user.password,
        }), {
            headers: { 'Content-Type': 'application/json' },
        });

        if (res.status === 200) {
            const body = JSON.parse(res.body);
            if (body.data && body.data.token) {
                tokens.push(body.data.token);
            }
        }
    }

    console.log(`成功获取 ${tokens.length} 个用户 Token`);
    return { tokens };
}

// ========================================================================
// 主测试函数
// ========================================================================

export default function (data) {
    const tokens = data.tokens;
    if (tokens.length === 0) {
        console.error('没有可用的 Token');
        return;
    }

    // 随机选择一个 Token
    const token = tokens[Math.floor(Math.random() * tokens.length)];

    group('秒杀场景', function () {
        // 【重点】秒杀接口测试
        const seckillRes = http.post(
            `${BASE_URL}/api/seckill`,
            JSON.stringify({
                product_id: parseInt(PRODUCT_ID),
                quantity: 1,
            }),
            {
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`,
                },
                tags: { name: 'seckill' }, // 用于指标分组
            }
        );

        // 记录延迟
        seckillDuration.add(seckillRes.timings.duration);

        // 检查响应
        const success = check(seckillRes, {
            'status is 200': (r) => r.status === 200,
            'response has code': (r) => JSON.parse(r.body).code !== undefined,
        });

        // 解析响应判断秒杀结果
        if (seckillRes.status === 200) {
            const body = JSON.parse(seckillRes.body);
            if (body.code === 0) {
                // 秒杀成功
                seckillSuccessRate.add(1);
                seckillSuccessCounter.add(1);
            } else {
                // 秒杀失败（库存不足、已购买等）
                seckillSuccessRate.add(0);
                seckillFailCounter.add(1);
            }
        } else {
            seckillSuccessRate.add(0);
            seckillFailCounter.add(1);
        }
    });

    // 【重点】模拟用户思考时间
    // 真实场景中用户不会连续发起请求
    sleep(Math.random() * 0.5); // 0-0.5 秒随机等待
}

// ========================================================================
// Teardown：测试后清理
// ========================================================================

export function teardown(data) {
    console.log('========== 压测结束 ==========');
}

// ========================================================================
// 【重点】K6 使用指南
// ========================================================================
// 
// 1. 安装 K6:
//    Windows: choco install k6
//    Mac: brew install k6
//    Linux: https://k6.io/docs/getting-started/installation/
//
// 2. 运行测试:
//    k6 run scripts/k6/seckill_test.js
//
// 3. 带参数运行:
//    k6 run -e BASE_URL=http://localhost:8080 -e PRODUCT_ID=1 scripts/k6/seckill_test.js
//
// 4. 输出 JSON 报告:
//    k6 run --out json=results.json scripts/k6/seckill_test.js
//
// 5. 输出到 InfluxDB + Grafana:
//    k6 run --out influxdb=http://localhost:8086/k6 scripts/k6/seckill_test.js
//
// 6. 云端运行（K6 Cloud）:
//    k6 cloud scripts/k6/seckill_test.js
// ========================================================================
