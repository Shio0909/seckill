<template>
  <div class="orders-page">
    <h2>我的订单</h2>

    <el-empty v-if="!loading && orders.length === 0" description="暂无订单" />

    <div v-loading="loading" class="order-list">
      <el-card v-for="order in orders" :key="order.id" class="order-card" shadow="hover">
        <div class="order-header">
          <span class="order-no">订单号：{{ order.order_no }}</span>
          <el-tag :type="statusType(order.status)" size="small">{{ statusText(order.status) }}</el-tag>
        </div>

        <div class="order-body">
          <div class="order-info">
            <p class="product-name">{{ order.product_name || `商品 #${order.product_id}` }}</p>
            <p class="order-time">下单时间：{{ formatTime(order.created_at) }}</p>
          </div>
          <div class="order-price">
            <span class="price-label">¥</span>
            <span class="price-value">{{ (order.amount / 100).toFixed(2) }}</span>
          </div>
        </div>

        <div class="order-footer">
          <el-button
            v-if="order.status === 0"
            type="primary"
            size="small"
            @click="handlePay(order)"
            :loading="payingId === order.id"
          >
            去支付
          </el-button>
          <el-button
            v-if="order.status === 0"
            size="small"
            @click="handleCancel(order)"
            :loading="cancellingId === order.id"
          >
            取消订单
          </el-button>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getOrders, cancelOrder, payOrder } from '../api'

const loading = ref(false)
const orders = ref([])
const payingId = ref(null)
const cancellingId = ref(null)

const statusMap = {
  0: { text: '待支付', type: 'warning' },
  1: { text: '已支付', type: 'success' },
  2: { text: '已取消', type: 'info' },
  3: { text: '已超时', type: 'danger' },
}

const statusText = (s) => statusMap[s]?.text || '未知'
const statusType = (s) => statusMap[s]?.type || 'info'

const formatTime = (t) => {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN')
}

const fetchOrders = async () => {
  loading.value = true
  try {
    const res = await getOrders()
    orders.value = res.data || []
  } catch (e) {
    // handled by interceptor
  } finally {
    loading.value = false
  }
}

const handlePay = async (order) => {
  payingId.value = order.id
  try {
    await payOrder(order.order_no)
    ElMessage.success('支付成功')
    order.status = 1
  } catch (e) {
    // handled
  } finally {
    payingId.value = null
  }
}

const handleCancel = async (order) => {
  try {
    await ElMessageBox.confirm('确定取消该订单吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '再想想',
      type: 'warning',
    })
  } catch {
    return
  }

  cancellingId.value = order.id
  try {
    await cancelOrder(order.order_no)
    ElMessage.success('订单已取消')
    order.status = 2
  } catch (e) {
    // handled
  } finally {
    cancellingId.value = null
  }
}

onMounted(fetchOrders)
</script>

<style scoped>
.orders-page {
  max-width: 720px;
  margin: 0 auto;
}

.orders-page h2 {
  font-size: 22px;
  margin-bottom: 24px;
}

.order-card {
  margin-bottom: 16px;
  border-radius: 12px;
}

.order-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.order-no {
  font-size: 13px;
  color: #909399;
}

.order-body {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.product-name {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 4px;
}

.order-time {
  font-size: 13px;
  color: #909399;
}

.order-price {
  text-align: right;
}

.price-label {
  font-size: 14px;
  color: #f56c6c;
}

.price-value {
  font-size: 24px;
  font-weight: 700;
  color: #f56c6c;
}

.order-footer {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
