<script setup lang="ts">
import NoticeStack from './NoticeStack.vue'
import { FileJson, LayoutDashboard, LogOut, Rss, Server, Users } from 'lucide-vue-next'

const logoutURL = new URL('logout', document.baseURI).pathname

const pages = [
  { to: '/', label: 'Обзор', icon: LayoutDashboard },
  { to: '/subscriptions', label: 'Подписки', icon: Rss },
  { to: '/configs', label: 'Конфиги', icon: FileJson },
  { to: '/users', label: 'Пользователи', icon: Users },
  { to: '/servers', label: 'Серверы', icon: Server },
]
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <RouterLink class="brand" to="/">VPN PANEL</RouterLink>
      <nav aria-label="Разделы">
        <RouterLink v-for="page in pages" :key="page.to" :to="page.to" class="nav-item">
          <component :is="page.icon" :size="18" />
          {{ page.label }}
        </RouterLink>
      </nav>
      <form class="sidebar-logout" :action="logoutURL" method="post">
        <button class="nav-item" type="submit"><LogOut :size="18" />Выйти</button>
      </form>
    </aside>
    <main class="main"><RouterView /></main>
  </div>
  <NoticeStack />
</template>
