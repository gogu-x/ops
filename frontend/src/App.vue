<script setup lang="ts">
import { onMounted } from 'vue'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()
onMounted(() => { void auth.loadCurrentUser() })
</script>

<template>
  <router-view />
</template>
