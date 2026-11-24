<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ size: string }>()
const width = ref<number>()
let startX = 0
let startWidth = 0

function resizeTo(value: number) {
  width.value = Math.max(440, Math.min(window.innerWidth - 40, value))
}

function startResize(event: PointerEvent) {
  if (!event.isPrimary || event.button !== 0) return
  const handle = event.currentTarget as HTMLElement
  const drawer = handle.closest('.el-drawer')
  if (!drawer) return
  startWidth = drawer.getBoundingClientRect().width
  startX = event.clientX
  handle.setPointerCapture(event.pointerId)
  event.preventDefault()
}

function moveResize(event: PointerEvent) {
  if ((event.currentTarget as HTMLElement).hasPointerCapture(event.pointerId)) {
    resizeTo(startWidth + startX - event.clientX)
  }
}

function stopResize(event: PointerEvent) {
  const handle = event.currentTarget as HTMLElement
  if (handle.hasPointerCapture(event.pointerId)) handle.releasePointerCapture(event.pointerId)
}

function resizeWithKey(event: KeyboardEvent, delta: number) {
  const drawer = (event.currentTarget as HTMLElement).closest('.el-drawer')
  if (drawer) resizeTo(drawer.getBoundingClientRect().width + delta)
}
</script>

<template>
  <el-drawer :size="width === undefined ? props.size : `min(${width}px, calc(100vw - 40px))`">
    <button type="button" class="drawer-resize-handle" aria-label="调整抽屉宽度，左方向键加宽，右方向键缩窄"
      @pointerdown="startResize" @pointermove="moveResize" @pointerup="stopResize" @pointercancel="stopResize"
      @keydown.left.prevent="resizeWithKey($event, 24)" @keydown.right.prevent="resizeWithKey($event, -24)" />
    <slot />
    <template v-if="$slots.footer" #footer><slot name="footer" /></template>
  </el-drawer>
</template>

<style scoped>
.drawer-resize-handle {
  position: absolute;
  z-index: 5;
  inset: 0 auto 0 0;
  width: 12px;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: ew-resize;
  touch-action: none;
}
.drawer-resize-handle::after {
  content: '';
  display: block;
  margin: auto;
  width: 4px;
  height: 48px;
  border-radius: 4px;
  background: #d4deeb;
}
.drawer-resize-handle:hover::after,
.drawer-resize-handle:focus-visible::after { background: var(--ops-primary); }
@media (max-width: 760px) {
  .drawer-resize-handle { display: none; }
}
</style>
