"""
EventHub 抢票系统 - Locust 压力测试脚本

使用 Python 编写的压测脚本，适合：
1. 更复杂的测试逻辑
2. Python 开发者更熟悉
3. 实时 Web UI 监控
4. 分布式压测

运行方式：
    # 单机模式（Web UI）
    locust -f locustfile.py --host=http://localhost:8080
    
    # 无头模式
    locust -f locustfile.py --host=http://localhost:8080 --headless -u 1000 -r 100 -t 5m
    
    # 分布式模式
    locust -f locustfile.py --master
    locust -f locustfile.py --worker --master-host=<master-ip>
"""

import random
import string
import time
import json
from locust import HttpUser, task, between, events, tag
from locust.runners import MasterRunner, WorkerRunner


class EventHubUser(HttpUser):
    """模拟抢票用户"""
    
    # 用户请求间隔：1-3秒
    wait_time = between(1, 3)
    
    # 用户 Token
    token = None
    username = None
    
    def on_start(self):
        """用户开始时执行：注册并登录"""
        self.username = f"locust_{int(time.time()*1000)}_{random.randint(1000,9999)}"
        self.password = "123456"
        self.phone = f"138{random.randint(10000000, 99999999)}"
        
        # 注册
        register_payload = {
            "username": self.username,
            "password": self.password,
            "phone": self.phone,
        }
        
        with self.client.post(
            "/api/user/register",
            json=register_payload,
            catch_response=True,
            name="/api/user/register"
        ) as response:
            if response.status_code == 200:
                response.success()
            else:
                # 用户可能已存在，继续尝试登录
                response.success()
        
        # 登录获取 Token
        login_payload = {
            "username": self.username,
            "password": self.password,
        }
        
        with self.client.post(
            "/api/user/login",
            json=login_payload,
            catch_response=True,
            name="/api/user/login"
        ) as response:
            if response.status_code == 200:
                try:
                    data = response.json()
                    if data.get("data", {}).get("token"):
                        self.token = data["data"]["token"]
                        response.success()
                    else:
                        response.failure("登录成功但未返回 Token")
                except:
                    response.failure("解析响应失败")
            else:
                response.failure(f"登录失败: {response.status_code}")
    
    def _get_auth_headers(self):
        """获取认证头"""
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers
    
    def _generate_idempotency_key(self):
        """生成幂等性 Key"""
        return f"locust_{self.username}_{int(time.time()*1000)}_{random.randint(1, 999999)}"
    
    # =====================================================================
    # 任务定义（权重控制执行频率）
    # =====================================================================
    
    @task(1)
    @tag('browse')
    def browse_events(self):
        """浏览活动列表"""
        with self.client.get(
            "/api/events",
            params={"page": 1, "size": 20},
            headers=self._get_auth_headers(),
            catch_response=True,
            name="/api/events"
        ) as response:
            if response.status_code == 200:
                response.success()
            else:
                response.failure(f"状态码: {response.status_code}")
    
    @task(2)
    @tag('browse')
    def view_event_detail(self):
        """查看活动详情"""
        event_id = random.randint(1, 100)
        
        with self.client.get(
            f"/api/events/{event_id}",
            headers=self._get_auth_headers(),
            catch_response=True,
            name="/api/events/[id]"
        ) as response:
            if response.status_code == 200:
                response.success()
            elif response.status_code == 404:
                response.success()  # 活动不存在也算正常
            else:
                response.failure(f"状态码: {response.status_code}")
    
    @task(5)
    @tag('seckill')
    def seckill_ticket(self):
        """【核心】抢票"""
        if not self.token:
            return
        
        event_id = random.randint(1, 10)  # 随机抢购活动
        
        headers = self._get_auth_headers()
        headers["X-Idempotency-Key"] = self._generate_idempotency_key()
        
        payload = {
            "ticket_type": random.randint(1, 3),
            "quantity": 1,
        }
        
        with self.client.post(
            f"/api/seckill/{event_id}",
            json=payload,
            headers=headers,
            catch_response=True,
            name="/api/seckill/[id]"
        ) as response:
            if response.status_code == 200:
                try:
                    data = response.json()
                    code = data.get("code", -1)
                    
                    if code == 0 or code == 200:
                        # 抢票成功
                        response.success()
                    elif code == 4001:
                        # 库存不足（正常业务失败）
                        response.success()
                    else:
                        response.failure(f"业务失败: {data.get('message', 'unknown')}")
                except:
                    response.failure("解析响应失败")
            elif response.status_code == 429:
                # 被限流（正常保护机制）
                response.success()
            elif response.status_code == 401:
                response.failure("未授权")
            else:
                response.failure(f"状态码: {response.status_code}")
    
    @task(1)
    @tag('order')
    def check_orders(self):
        """查询我的订单"""
        if not self.token:
            return
        
        with self.client.get(
            "/api/orders",
            headers=self._get_auth_headers(),
            catch_response=True,
            name="/api/orders"
        ) as response:
            if response.status_code == 200:
                response.success()
            elif response.status_code == 401:
                response.failure("未授权")
            else:
                response.failure(f"状态码: {response.status_code}")


class SeckillSpikeUser(HttpUser):
    """
    纯抢票用户 - 模拟开票瞬间高并发
    
    特点：
    - 极短的等待时间（模拟疯狂点击）
    - 只执行抢票操作
    - 适合峰值测试
    """
    
    wait_time = between(0.1, 0.5)  # 极短间隔
    
    token = None
    username = None
    event_id = 1  # 固定抢购的活动
    
    def on_start(self):
        """登录"""
        self.username = f"spike_{int(time.time()*1000)}_{random.randint(1000,9999)}"
        self.password = "123456"
        self.phone = f"139{random.randint(10000000, 99999999)}"
        
        # 快速注册登录
        self.client.post("/api/user/register", json={
            "username": self.username,
            "password": self.password,
            "phone": self.phone,
        }, name="/api/user/register")
        
        resp = self.client.post("/api/user/login", json={
            "username": self.username,
            "password": self.password,
        }, name="/api/user/login")
        
        if resp.status_code == 200:
            try:
                self.token = resp.json().get("data", {}).get("token")
            except:
                pass
    
    @task
    def grab_ticket(self):
        """疯狂抢票"""
        if not self.token:
            return
        
        headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.token}",
            "X-Idempotency-Key": f"spike_{self.username}_{int(time.time()*1000000)}",
        }
        
        with self.client.post(
            f"/api/seckill/{self.event_id}",
            json={"ticket_type": 1, "quantity": 1},
            headers=headers,
            catch_response=True,
            name="/api/seckill/[id] (spike)"
        ) as response:
            # 200 或 429(限流) 都算成功处理
            if response.status_code in [200, 429]:
                response.success()
            else:
                response.failure(f"状态码: {response.status_code}")


# =========================================================================
# 事件钩子：自定义统计
# =========================================================================

seckill_success = 0
seckill_stock_empty = 0
seckill_rate_limited = 0

@events.request.add_listener
def on_request(request_type, name, response_time, response_length, response, exception, **kwargs):
    """监听请求，统计抢票结果"""
    global seckill_success, seckill_stock_empty, seckill_rate_limited
    
    if "seckill" in name and response:
        if response.status_code == 200:
            try:
                data = response.json()
                code = data.get("code", -1)
                if code == 0 or code == 200:
                    seckill_success += 1
                elif code == 4001:
                    seckill_stock_empty += 1
            except:
                pass
        elif response.status_code == 429:
            seckill_rate_limited += 1


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    """测试结束时打印统计"""
    print("\n" + "="*60)
    print("EventHub 抢票压测统计")
    print("="*60)
    print(f"抢票成功数: {seckill_success}")
    print(f"库存不足数: {seckill_stock_empty}")
    print(f"被限流数: {seckill_rate_limited}")
    print("="*60 + "\n")


# =========================================================================
# 使用说明
# =========================================================================

"""
📝 常用命令：

1. 启动 Web UI 模式：
   locust -f locustfile.py --host=http://localhost:8080
   然后访问 http://localhost:8089

2. 无头模式（自动化测试）：
   locust -f locustfile.py \
       --host=http://localhost:8080 \
       --headless \
       -u 500 \           # 最大用户数
       -r 50 \            # 每秒增加用户数
       -t 5m \            # 持续时间
       --html=report.html # 生成报告

3. 只运行抢票场景：
   locust -f locustfile.py \
       --host=http://localhost:8080 \
       --tags seckill \
       --headless -u 1000 -r 100 -t 2m

4. 分布式压测（多机器）：
   # 主节点
   locust -f locustfile.py --master
   
   # 工作节点（可以多个）
   locust -f locustfile.py --worker --master-host=192.168.1.100

5. 使用指定用户类：
   locust -f locustfile.py SeckillSpikeUser --host=http://localhost:8080
"""
