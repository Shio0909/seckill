import axios from 'axios'
import { ElMessage } from 'element-plus'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
})

// 请求拦截器：自动携带 JWT
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截器：统一错误处理
api.interceptors.response.use(
  (res) => {
    const data = res.data
    // 后端返回格式: { code, message, data }
    if (data.code && data.code !== 200) {
      ElMessage.error(data.message || '请求失败')
      return Promise.reject(new Error(data.message))
    }
    return data
  },
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      ElMessage.error('登录已过期，请重新登录')
      window.location.href = '/login'
    } else if (err.response?.status === 429) {
      ElMessage.warning('操作过于频繁，请稍后再试')
    } else {
      ElMessage.error(err.response?.data?.message || '网络错误')
    }
    return Promise.reject(err)
  }
)

export default api
