import test from 'node:test'
import assert from 'node:assert/strict'
import { scopeInstances, statusCounts, needsAttention, projectOverview } from '../src/utils/dashboard.ts'

const projects = [{ id: 'a', name: '游戏' }, { id: 'b', name: '平台' }]
const instances = [
  { id: 'one', project_id: 'a', service_type_id: 'game', status: 'running' },
  { id: 'two', project_id: 'a', service_type_id: 'game', status: 'restarting' },
  { id: 'three', project_id: 'b', service_type_id: 'game', status: 'stopped' },
  { id: 'four', project_id: 'b', service_type_id: 'game', status: '' },
  { id: 'five', service_type_id: 'deleted-type', status: 'unknown' },
]

test('project scope follows service ownership, including instances sharing a global service type', () => {
  assert.deepEqual(scopeInstances(instances, 'a').map((item) => item.id), ['one', 'two'])
  assert.deepEqual(scopeInstances(instances, 'missing'), [])
  assert.equal(scopeInstances(instances, '').length, 5)
})

test('missing runtime state is unknown, never running or stopped', () => {
  const counts = Object.fromEntries(statusCounts(instances).map((item) => [item.key, item.count]))
  assert.deepEqual(counts, { running: 1, stopped: 1, restarting: 1, error: 0, unknown: 2, not_deployed: 0 })
  assert.equal(Object.values(counts).reduce((a, b) => a + b), instances.length)
  assert.equal(needsAttention(instances[3]), true)
  assert.equal(needsAttention(instances[2]), false)
})

test('shared hosts count once per project despite both legacy and new bindings', () => {
  const hosts = [{ id: 'shared', project_id: 'a', project_ids: ['a', 'b'] }, { id: 'legacy', project_id: 'a' }]
  const result = projectOverview(projects, [{ project_id: 'a' }, { project_id: 'b' }], instances, hosts)
  assert.deepEqual(result.map((row) => ({ total: row.total, running: row.running, attention: row.attention, hosts: row.hosts })), [
    { total: 2, running: 1, attention: 1, hosts: 2 },
    { total: 2, running: 0, attention: 1, hosts: 1 },
  ])
  assert.equal(result[0].percentage, 50)
})

test('empty data has zero counts and no invalid percentage', () => {
  assert.ok(statusCounts([]).every((item) => item.count === 0))
  const [row] = projectOverview([projects[0]], [], [], [])
  assert.equal(row.percentage, 0)
  assert.equal(row.attention, 0)
})
