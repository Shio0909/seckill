<template>
  <el-card class="product-card" shadow="hover" :body-style="{ padding: '0' }">
    <div class="card-image">
      <img :src="product.image_url || defaultImage" :alt="product.name" />
      <el-tag
        class="card-tag"
        :type="product.stock > 0 ? 'danger' : 'info'"
        effect="dark"
        size="small"
      >
        {{ product.stock > 0 ? '即将开抢' : '已售罄' }}
      </el-tag>
    </div>
    <div class="card-body">
      <h3 class="card-title">{{ product.name }}</h3>
      <p class="card-venue" v-if="product.venue || product.city">
        📍 {{ product.venue || product.city }}
      </p>
      <p class="card-time" v-if="product.event_time">
        🕐 {{ formatDate(product.event_time) }}
      </p>
      <div class="card-footer">
        <span class="card-price">¥{{ product.price }}</span>
        <span class="card-stock">余票 {{ product.stock }}</span>
      </div>
    </div>
  </el-card>
</template>

<script setup>
defineProps({
  product: {
    type: Object,
    required: true,
  },
})

const defaultImage = 'https://via.placeholder.com/400x200/667eea/ffffff?text=EventHub'

const formatDate = (dateStr) => {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  return `${d.getMonth() + 1}月${d.getDate()}日 ${d.getHours()}:${String(d.getMinutes()).padStart(2, '0')}`
}
</script>

<style scoped>
.product-card {
  cursor: pointer;
  transition: transform 0.2s;
  border-radius: 12px;
  overflow: hidden;
}

.product-card:hover {
  transform: translateY(-4px);
}

.card-image {
  position: relative;
  height: 160px;
  overflow: hidden;
  background: #f0f2f5;
}

.card-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.card-tag {
  position: absolute;
  top: 8px;
  right: 8px;
}

.card-body {
  padding: 14px 16px 16px;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-venue,
.card-time {
  font-size: 13px;
  color: #909399;
  margin-bottom: 4px;
}

.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 12px;
}

.card-price {
  font-size: 20px;
  font-weight: 700;
  color: #f56c6c;
}

.card-stock {
  font-size: 12px;
  color: #909399;
}
</style>
