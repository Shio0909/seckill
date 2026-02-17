import api from './request'

// ============ 用户 ============
export const register = (data) => api.post('/register', data)
export const login = (data) => api.post('/login', data)

// ============ 商品 ============
export const getProducts = (params) => api.get('/products', { params })
export const getProduct = (id) => api.get(`/products/${id}`)
export const getStock = (id) => api.get(`/products/${id}/stock`)

// ============ 秒杀 ============
export const getIdempotentToken = () => api.get('/idempotent/token')
export const seckillBuy = (productId, token) =>
  api.post('/seckill/buy', `product_id=${productId}`, {
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'X-Idempotent-Token': token,
    },
  })

// ============ 订单 ============
export const getOrders = (params) => api.get('/orders', { params })
export const getOrder = (id) => api.get(`/orders/${id}`)
export const cancelOrder = (id) => api.post(`/orders/${id}/cancel`)
export const payOrder = (id) => api.post(`/orders/${id}/pay`)
