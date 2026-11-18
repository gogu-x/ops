import type { ServiceParameterField } from '../api/services'

export function parameterOverrides(fields: ServiceParameterField[], values: Record<string, string>) {
  const result: Record<string, string> = {}
  for (const field of fields) {
    const value = values[field.flag] ?? ''
    if (value.trim()) result[field.flag] = field.kind === 'text' ? value : value.trim()
  }
  return result
}
