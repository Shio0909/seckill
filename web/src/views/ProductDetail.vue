<template>
  <div class="product-detail" v-loading="loading">
    <el-button text :icon="ArrowLeft" @click="router.back()" style="margin-bottom: 16px">
      返回
    </el-button>

    <template v-if="product">
      <el-row :gutter="24">
        <!-- 左侧图片 -->
        <el-col :span="14">
          <div class="detail-image">
            <img :src="product.ImageURL || defaultImage" :alt="product.Name" />
          </div>
        </el-col>

        <!-- 右侧信息 -->
        <el-col :span="10">
          <div class="detail-info">
            <h1>{{ product.Name }}</h1>

            <div class="info-row" v-if="product.Artist">
              <span class="info-label">艺人</span>
              <span>{{ product.Artist }}</span>
            </div>
            <div class="info-row" v-if="product.Venue">
              <span class="info-label">场馆</span>
              <span>{{ product.Venue }}<template v-if="product.City"> · {{ product.City }}</template></span>
            </div>
            <div class="info-row" v-if="product.EventTime">
              <span class="info-label">时间</span>
              <span>{{ formatDateTime(product.EventTime) }}</span>
            </div>

            <div class="price-section">
              <span class="price">¥{{ product.Price }}</span>
              <span class="price-high" v-if="product.HighPrice">
                - ¥{{ product.HighPrice }}
              </span>
            </div>

            <div class="stock-section">
              <el-tag :type="stockTagType" size="large">
                {{ stockText }}
              </el-tag>
              <span class="stock-num">剩余 {{ stock }} 张</span>
            </div>

            <!-- 抢票按钮 -->
            <el-button
              type="danger"
              size="large"
              :loading="buying"
              :disabled="stock <= 0"
              class="buy-btn"
              @click="handleBuy"
            >
              {{ stock > 0 ? '🔥 立即抢票' : '已售罄' }}
            </el-button>

            <div class="buy-tips">
              <p>· 每人限购 1 张</p>
              <p>· 请在 30 分钟内完成支付</p>
              <p>· 抢票使用 Redis+Lua 原子扣库存</p>
            </div>
          </div>
        </el-col>
      </el-row>

      <!-- 活动详情 -->
      <el-card class="desc-card" v-if="product.Description">
        <template #header><h3>活动详情</h3></template>
        <p>{{ product.Description }}</p>
      </el-card>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { getProduct, getStock, getIdempotentToken, seckillBuy } from '../api'
import { useUserStore } from '../stores/user'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const product = ref(null)
const stock = ref(0)
const loading = ref(true)
const buying = ref(false)

const defaultImage = 'https://via.placeholder.com/800x400/667eea/ffffff?text=EventHub'

const stockTagType = computed(() => {
  if (stock.value <= 0) return 'info'
  if (stock.value < 50) return 'warning'
  return 'success'
})

const stockText = computed(() => {
  if (stock.value <= 0) return '已售罄'
  if (stock.value < 50) return '即将售罄'
  return '有票'
})

const formatDateTime = (dateStr) => {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  return d.toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  })
}

const loadProduct = async () => {
  loading.value = true
  try {
    const [prodRes, stockRes] = await Promise.all([
      getProduct(route.params.id),
      getStock(route.params.id),
    ])
    product.value = prodRes.data
    stock.value = stockRes.data?.stock ?? prodRes.data?.Stock ?? 0
  } catch (e) {
    ElMessage.error('加载活动信息失败')
  } finally {
    loading.value = false
  }
}

const handleBuy = async () => {
  if (!userStore.isLoggedIn) {
    ElMessage.warning('请先登录')
    router.push({ name: 'Login', query: { redirect: route.fullPath } })
    return
  }

  buying.value = true
  try {
    // 1. 获取幂等令牌
    const tokenRes = await getIdempotentToken()
    const idempotentToken = tokenRes.data?.token
    if (!idempotentToken) {
      ElMessage.error('获取令牌失败')
      return
    }

    // 2. 发起秒杀请求
    const res = await seckillBuy(route.params.id, idempotentToken)
    ElMessage.success(res.message || '抢票成功！正在生成订单...')

    // 3. 刷新库存
    setTimeout(async () => {
      const stockRes = await getStock(route.params.id)
      stock.value = stockRes.data?.stock ?? stock.value
    }, 1000)
  } catch (e) {
    // 错误在拦截器中已处理
  } finally {
    buying.value = false
  }
}

onMounted(loadProduct)
</script>

<style scoped>
.detail-image {
  border-radius: 12px;
  overflow: hidden;
  background: #f0f2f5;
}

.detail-image img {
  width: 100%;
  aspect-ratio: 16 / 9;
  object-fit: cover;
}

.detail-info {
  padding: 8px 0;
}

.detail-info h1 {
  font-size: 24px;
  margin-bottom: 16px;
  line-height: 1.4;
}

.info-row {
  display: flex;
  align-items: center;
  margin-bottom: 10px;
  font-size: 14px;
  color: #606266;
}

.info-label {
  background: #f0f2f5;
  padding: 2px 10px;
  border-radius: 4px;
  margin-right: 10px;
  font-size: 12px;
  color: #909399;
  flex-shrink: 0;
}

.price-section {
  margin: 20px 0 16px;
}

.price {
  font-size: 32px;
  font-weight: 700;
  color: #f56c6c;
}

.price-high {
  font-size: 20px;
  color: #f56c6c;
}

.stock-section {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
}

.stock-num {
  color: #909399;
  font-size: 14px;
}

.buy-btn {
  width: 100%;
  height: 52px;
  font-size: 18px;
  border-radius: 8px;
  margin-bottom: 16px;
}

.buy-tips {
  background: #fef0f0;
  border-radius: 8px;
  padding: 12px 16px;
  font-size: 13px;
  color: #f56c6c;
  line-height: 1.8;
}

.desc-card {
  margin-top: 24px;
  border-radius: 12px;
}

.desc-card p {
  line-height: 1.8;
  color: #606266;
}
</style>
