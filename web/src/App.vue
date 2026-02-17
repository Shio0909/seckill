<template>
  <el-config-provider :locale="zhCn">
    <div class="app-container">
      <!-- 顶部导航 -->
      <el-header class="app-header">
        <div class="header-content">
          <div class="logo" @click="router.push('/')">
            <el-icon :size="24"><Ticket /></el-icon>
            <span>EventHub</span>
          </div>
          <div class="nav-right">
            <template v-if="userStore.isLoggedIn">
              <el-button text @click="router.push('/orders')">
                <el-icon><List /></el-icon>我的订单
              </el-button>
              <el-dropdown @command="handleCommand">
                <span class="user-info">
                  <el-avatar :size="28" :icon="UserFilled" />
                  {{ userStore.username }}
                </span>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="logout">退出登录</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </template>
            <template v-else>
              <el-button type="primary" @click="router.push('/login')">登录</el-button>
              <el-button @click="router.push('/register')">注册</el-button>
            </template>
          </div>
        </div>
      </el-header>

      <!-- 主内容 -->
      <el-main class="app-main">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>

      <!-- 底部 -->
      <el-footer class="app-footer">
        <span>EventHub © 2026 — 高并发抢票平台 Demo</span>
      </el-footer>
    </div>
  </el-config-provider>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { Ticket, List, UserFilled } from '@element-plus/icons-vue'
import { useUserStore } from './stores/user'
import zhCn from 'element-plus/es/locale/lang/zh-cn'

const router = useRouter()
const userStore = useUserStore()

const handleCommand = (cmd) => {
  if (cmd === 'logout') {
    userStore.logout()
    router.push('/')
    ElMessage.success('已退出登录')
  }
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  background: #f5f7fa;
  color: #303133;
}

.app-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.app-header {
  background: #fff;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
  position: sticky;
  top: 0;
  z-index: 100;
  height: 60px !important;
  padding: 0;
}

.header-content {
  max-width: 1200px;
  margin: 0 auto;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}

.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 20px;
  font-weight: 700;
  color: #409eff;
  cursor: pointer;
  user-select: none;
}

.nav-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-size: 14px;
  color: #606266;
}

.app-main {
  max-width: 1200px;
  width: 100%;
  margin: 0 auto;
  padding: 20px;
  flex: 1;
}

.app-footer {
  text-align: center;
  color: #909399;
  font-size: 13px;
  height: 48px !important;
  line-height: 48px;
  background: #fff;
  border-top: 1px solid #ebeef5;
}

/* 页面切换动画 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
