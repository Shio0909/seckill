<template>
  <div class="home">
    <!-- Hero 区域 -->
    <div class="hero">
      <h1>🎫 EventHub</h1>
      <p>热门演出 · 极速抢票 · 高并发秒杀系统</p>
      <div class="hero-stats">
        <div class="stat-item">
          <span class="stat-num">3000+</span>
          <span class="stat-label">RPS 吞吐量</span>
        </div>
        <div class="stat-item">
          <span class="stat-num">25ms</span>
          <span class="stat-label">P99 延迟</span>
        </div>
        <div class="stat-item">
          <span class="stat-num">0%</span>
          <span class="stat-label">超卖率</span>
        </div>
      </div>
    </div>

    <!-- 活动列表 -->
    <div class="section-header">
      <h2>🔥 热门活动</h2>
      <el-input
        v-model="keyword"
        placeholder="搜索活动..."
        :prefix-icon="Search"
        clearable
        style="width: 260px"
        @keyup.enter="loadProducts"
      />
    </div>

    <div v-loading="loading" class="product-grid">
      <ProductCard
        v-for="item in products"
        :key="item.ID"
        :product="item"
        @click="goDetail(item.ID)"
      />
      <el-empty v-if="!loading && products.length === 0" description="暂无活动" />
    </div>

    <!-- 分页 -->
    <div class="pagination" v-if="total > pageSize">
      <el-pagination
        background
        layout="prev, pager, next"
        :total="total"
        :page-size="pageSize"
        v-model:current-page="page"
        @current-change="loadProducts"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Search } from '@element-plus/icons-vue'
import { getProducts } from '../api'
import ProductCard from '../components/ProductCard.vue'

const router = useRouter()
const products = ref([])
const loading = ref(false)
const keyword = ref('')
const page = ref(1)
const pageSize = 12
const total = ref(0)

const loadProducts = async () => {
  loading.value = true
  try {
    const res = await getProducts({
      page: page.value,
      page_size: pageSize,
      keyword: keyword.value,
    })
    products.value = res.data?.list || []
    total.value = res.data?.total || 0
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

const goDetail = (id) => {
  router.push(`/products/${id}`)
}

onMounted(loadProducts)
</script>

<style scoped>
.hero {
  text-align: center;
  padding: 48px 20px 40px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 16px;
  color: #fff;
  margin-bottom: 32px;
}

.hero h1 {
  font-size: 36px;
  margin-bottom: 8px;
}

.hero p {
  font-size: 16px;
  opacity: 0.9;
  margin-bottom: 24px;
}

.hero-stats {
  display: flex;
  justify-content: center;
  gap: 48px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.stat-num {
  font-size: 28px;
  font-weight: 700;
}

.stat-label {
  font-size: 13px;
  opacity: 0.8;
  margin-top: 4px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.section-header h2 {
  font-size: 20px;
}

.product-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 20px;
  min-height: 200px;
}

.pagination {
  display: flex;
  justify-content: center;
  margin-top: 32px;
}
</style>
