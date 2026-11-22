import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import Workspace from './Workspace.vue'
import './style.css'

const router = createRouter({
  history: createWebHistory(new URL(document.baseURI).pathname),
  routes: [
    { path: '/', name: 'overview', component: Workspace },
    { path: '/configs', name: 'configs', component: Workspace },
    { path: '/servers', name: 'servers', component: Workspace },
    { path: '/users', name: 'users', component: Workspace },
    { path: '/subscriptions', name: 'subscriptions', component: Workspace },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

createApp(App).use(router).mount('#app')

