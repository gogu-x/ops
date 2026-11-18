<script setup lang="ts">
import type { ServiceParameterField } from '../api/services'

defineProps<{ fields: ServiceParameterField[]; values: Record<string, string>; optionalOverrides?: boolean }>()
const emit = defineEmits<{ change: [flag: string, value: string] }>()
</script>

<template>
  <el-form-item v-for="field in fields" :key="field.flag" :label="field.label" :required="!optionalOverrides && field.required" class="service-parameter-field">
    <el-select
      v-if="field.kind === 'select'"
      :model-value="values[field.flag]"
      :clearable="optionalOverrides || !field.required"
      :placeholder="optionalOverrides ? '沿用实例值' : field.flag"
      @update:model-value="emit('change', field.flag, $event ?? '')"
    >
      <el-option v-for="option in field.options" :key="option" :label="option" :value="option" />
    </el-select>
    <el-input
      v-else
      :model-value="values[field.flag]"
      :inputmode="field.kind === 'integer' ? 'numeric' : 'text'"
      :placeholder="optionalOverrides ? '沿用实例值' : field.flag"
      @update:model-value="emit('change', field.flag, $event)"
    />
  </el-form-item>
</template>

<style scoped>
.service-parameter-field :deep(.el-form-item__label) {
  min-height: var(--el-component-size);
  height: auto;
  align-items: center;
  line-height: 1.4;
  text-align: right;
  overflow-wrap: anywhere;
}
.service-parameter-field :deep(.el-input),
.service-parameter-field :deep(.el-select) {
  width: 100%;
  min-width: 0;
}
</style>
