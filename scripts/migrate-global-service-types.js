// Run with mongosh using the same MongoDB connection options as Ops.
// The default is a read-only preview. Set OPS_MIGRATION_APPLY=1 to write.
// Optional: OPS_SERVICE_TYPE_KEEP='{"server":"id-to-keep"}' resolves a
// configuration conflict by explicitly selecting the type that will be shared.
(() => {
  const apply = process.env.OPS_MIGRATION_APPLY === '1'
  const overrides = JSON.parse(process.env.OPS_SERVICE_TYPE_KEEP || '{}')
  if (!overrides || typeof overrides !== 'object' || Array.isArray(overrides)) {
    throw new Error('OPS_SERVICE_TYPE_KEEP 必须是类型名称到保留 ID 的 JSON 对象')
  }

  const ops = db.getSiblingDB('ops_platform')
  const types = ops.getCollection('service_types')
  const instances = ops.getCollection('service_instances')
  const activeRollout = ops.getCollection('rollout_jobs').findOne(
    { status: { $in: ['queued', 'running', 'paused'] } },
    { _id: 1, status: 1 },
  )
  if (activeRollout) {
    throw new Error(`请先完成滚动发布任务 ${activeRollout._id}（${activeRollout.status}）`)
  }

  const documents = types.find().sort({ _id: 1 }).toArray()
  const sourceByID = new Map()
  const groups = new Map()
  for (const type of documents) {
    if (typeof type._id !== 'string' || typeof type.name !== 'string' || !type.name.trim()) {
      throw new Error(`服务类型 ${type._id} 的 ID 或名称无效`)
    }
    sourceByID.set(type._id, type)
    if (!groups.has(type.name)) groups.set(type.name, [])
    groups.get(type.name).push(type)
  }

  for (const [name, id] of Object.entries(overrides)) {
    if (!groups.get(name)?.some(type => type._id === id)) {
      throw new Error(`指定保留的类型不存在：${name} / ${id}`)
    }
  }

  function configKey(type) {
    const probe = type.readiness_probe || {}
    const fields = (type.parameter_fields || []).map(field => ({
      flag: field.flag,
      label: field.label,
      kind: field.kind,
      required: Boolean(field.required),
      default_value: field.default_value || '',
      options: field.kind === 'select' ? field.options || [] : [],
    }))
    const probeType = probe.type || ''
    return JSON.stringify({
      probe: {
        type: probeType,
        port: ['tcp', 'http'].includes(probeType) ? probe.port || 0 : 0,
        path: probeType === 'http' ? probe.path || '/health' : '',
      },
      fields,
    })
  }

  const targetByID = new Map()
  const mergeGroups = []
  const conflicts = []
  for (const [name, candidates] of groups) {
    const override = Object.hasOwn(overrides, name) ? overrides[name] : undefined
    const keep = override ? candidates.find(type => type._id === override) : candidates[0]
    if (!override && new Set(candidates.map(configKey)).size > 1) {
      conflicts.push({ name, candidates: candidates.map(type => ({
        id: type._id,
        project_id: type.project_id,
        environment_id: type.environment_id,
        readiness_probe: type.readiness_probe?.type || '未配置',
        parameter_fields: (type.parameter_fields || []).map(field => field.flag),
      })) })
    }
    for (const candidate of candidates) targetByID.set(candidate._id, keep._id)
    const remove = candidates.filter(type => type._id !== keep._id).map(type => type._id)
    if (remove.length) mergeGroups.push({ name, keep: keep._id, remove })
  }
  if (conflicts.length) {
    printjson(conflicts)
    throw new Error('同名类型存在不同配置。请通过 OPS_SERVICE_TYPE_KEEP 指定保留哪条记录，再重新预览。')
  }

  const projects = new Set(ops.getCollection('projects').distinct('_id'))
  const environmentProjects = new Map(ops.getCollection('environments')
    .find({}, { _id: 1, project_id: 1 }).toArray().map(item => [item._id, item.project_id]))
  const changes = []
  for (const instance of instances.find({}, { service_type_id: 1, project_id: 1, environment_id: 1 })) {
    const source = sourceByID.get(instance.service_type_id)
    if (!source) {
      throw new Error(`实例 ${instance._id} 引用不存在的服务类型 ${instance.service_type_id}`)
    }
    const projectID = instance.project_id || source.project_id
    const environmentID = instance.environment_id || source.environment_id
    if (!projectID || !environmentID || !projects.has(projectID) || environmentProjects.get(environmentID) !== projectID) {
      throw new Error(`实例 ${instance._id} 的项目或环境无效，请先补齐归属`)
    }
    const targetID = targetByID.get(source._id)
    if (targetID === source._id && projectID === instance.project_id && environmentID === instance.environment_id) continue
    changes.push({
      filter: {
        _id: instance._id,
        service_type_id: source._id,
        project_id: instance.project_id ?? null,
        environment_id: instance.environment_id ?? null,
      },
      update: { $set: { service_type_id: targetID, project_id: projectID, environment_id: environmentID } },
    })
  }

  if (ops.getCollectionNames().includes('service_types')) {
    const existingNameIndex = types.getIndexes().find(index => index.name === 'name_1')
    if (existingNameIndex && (
      Object.keys(existingNameIndex.key).length !== 1 || existingNameIndex.key.name !== 1 ||
      !existingNameIndex.unique || existingNameIndex.sparse || existingNameIndex.partialFilterExpression
    )) {
      throw new Error('现有 name_1 索引与新版要求不同，请先检查该索引')
    }
  }

  const removedIDs = mergeGroups.flatMap(group => group.remove)
  printjson({
    database: ops.getName(),
    mode: apply ? 'apply' : 'preview',
    instancesToUpdate: changes.length,
    duplicateTypesToRemove: removedIDs.length,
    mergeGroups,
  })
  if (!apply) return

  // Update each reference and its scope together. Keep source types until all
  // instance updates succeed so an interrupted run can safely resume.
  for (let offset = 0; offset < changes.length; offset += 500) {
    const batch = changes.slice(offset, offset + 500)
    const result = instances.bulkWrite(batch.map(updateOne => ({ updateOne })), { ordered: true })
    if (result.matchedCount !== batch.length) {
      throw new Error('实例在迁移期间发生变化，请停止 Ops 后重新预览并运行；尚未删除重复类型')
    }
  }
  if (removedIDs.length) {
    const remaining = instances.findOne({ service_type_id: { $in: removedIDs } }, { _id: 1 })
    if (remaining) throw new Error(`实例 ${remaining._id} 仍引用待合并类型，已停止删除`)
    types.deleteMany({ _id: { $in: removedIDs } })
  }
  types.updateMany(
    { $or: [{ project_id: { $exists: true } }, { environment_id: { $exists: true } }] },
    { $unset: { project_id: '', environment_id: '' } },
  )
  types.createIndex({ name: 1 }, { name: 'name_1', unique: true })
  print('迁移完成，可以启动 Ops。')
})()
