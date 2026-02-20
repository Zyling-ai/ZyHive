<template>
  <div class="team-view">
    <div class="page-header">
      <h2>团队关系图谱</h2>
      <div style="display:flex;gap:8px;">
        <el-button size="small" @click="loadGraph">
          <el-icon><Refresh /></el-icon> 刷新
        </el-button>
        <el-button size="small" type="danger" plain @click="clearAllRelations">
          <el-icon><Delete /></el-icon> 清空所有关系
        </el-button>
      </div>
    </div>

    <el-card v-loading="loading" class="graph-card">
      <!-- Empty: no members -->
      <div v-if="!loading && !graph.nodes.length" class="empty-state">
        <el-icon style="font-size:64px;color:#c0c4cc;display:block;margin:0 auto 16px"><Share /></el-icon>
        <p style="color:#909399;text-align:center;margin:0">暂无成员数据</p>
      </div>

      <!-- Empty: members but no relations -->
      <div v-else-if="!loading && graph.nodes.length && !graph.edges.length" class="empty-state">
        <el-icon style="font-size:64px;color:#c0c4cc;display:block;margin:0 auto 16px"><Connection /></el-icon>
        <p style="color:#909399;text-align:center;margin:0">为成员配置 RELATIONS.md 即可自动生成关系图谱</p>
        <p style="color:#c0c4cc;text-align:center;font-size:13px;margin:6px 0 0">
          前往成员详情页 → 「关系」Tab 编辑
        </p>
      </div>

      <!-- Hierarchical SVG Graph -->
      <div v-else class="graph-container" ref="containerRef">
        <svg :width="svgWidth" :height="svgHeight" class="graph-svg">

          <!-- Edges: simple lines, no arrows, neutral color -->
          <g v-for="edge in graph.edges" :key="`${edge.from}-${edge.to}`">
            <line
              :x1="edgePt(edge.from, edge.to, 'start').x"
              :y1="edgePt(edge.from, edge.to, 'start').y"
              :x2="edgePt(edge.from, edge.to, 'end').x"
              :y2="edgePt(edge.from, edge.to, 'end').y"
              stroke="#94a3b8"
              :stroke-width="edgeWidth(edge.strength)"
              stroke-opacity="0.6"
              stroke-linecap="round"
              class="graph-edge"
              @mouseenter="(e: MouseEvent) => showEdgeTooltip(e, edge)"
              @mouseleave="hideTooltip"
            />
          </g>

          <!-- Nodes -->
          <g
            v-for="node in graph.nodes"
            :key="node.id"
            :transform="`translate(${nodePos(node.id).x}, ${nodePos(node.id).y})`"
            class="graph-node"
            @click="goToAgent(node.id)"
          >
            <!-- Shadow -->
            <circle r="30" fill="rgba(0,0,0,0.07)" transform="translate(2,3)" />
            <!-- Main circle -->
            <circle r="28" :fill="nodeColor(node.id)" stroke="#fff" stroke-width="2.5" />
            <!-- Initials -->
            <text
              text-anchor="middle"
              dominant-baseline="central"
              fill="#fff"
              font-weight="700"
              font-size="15"
              font-family="system-ui, sans-serif"
            >{{ nodeInitial(node.id) }}</text>
            <!-- Status dot -->
            <circle
              cx="20" cy="-20" r="6"
              :fill="node.status === 'running' ? '#67C23A' : '#c0c4cc'"
              stroke="#fff"
              stroke-width="1.5"
            />
            <!-- Name -->
            <text
              text-anchor="middle"
              y="46"
              font-size="12"
              fill="#303133"
              font-family="system-ui, sans-serif"
              paint-order="stroke"
              stroke="#f5f7fa"
              stroke-width="3"
            >{{ node.name }}</text>
            <text
              text-anchor="middle"
              y="58"
              font-size="10"
              fill="#909399"
              font-family="system-ui, monospace"
            >{{ node.id }}</text>
          </g>
        </svg>

        <!-- Edge tooltip -->
        <div
          v-if="tooltip.visible"
          class="edge-tooltip"
          :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }"
        >
          <div class="tooltip-members">
            <span class="tooltip-name">{{ nodeName(tooltip.from) }}</span>
            <span class="tooltip-sep">↔</span>
            <span class="tooltip-name">{{ nodeName(tooltip.to) }}</span>
          </div>
          <div class="tooltip-tags">
            <el-tag size="small" type="info" effect="plain">{{ tooltip.type }}</el-tag>
            <el-tag size="small" effect="plain">{{ tooltip.strength }}</el-tag>
          </div>
          <div v-if="tooltip.label" class="tooltip-label">{{ tooltip.label }}</div>
        </div>
      </div>
    </el-card>

    <!-- Legend -->
    <el-card v-if="graph.nodes.length" class="legend-card">
      <div class="legend">
        <span class="legend-title">布局规则：</span>
        <span class="legend-item"><el-icon><ArrowUp /></el-icon> 上方 = 上级</span>
        <span class="legend-item">— 同层 = 平级协作</span>
        <span class="legend-item"><el-icon><ArrowDown /></el-icon> 下方 = 下级</span>
        <span class="legend-divider"> | </span>
        <span class="legend-title">线粗：</span>
        <span v-for="(w, s) in strengthWidths" :key="s" class="legend-item">
          <svg width="28" height="8"><line x1="0" y1="4" x2="28" y2="4" stroke="#64748b" :stroke-width="w" stroke-linecap="round" /></svg>
          {{ s }}
        </span>
        <span class="legend-divider"> | </span>
        <span class="legend-item">
          <svg width="12" height="12"><circle cx="6" cy="6" r="5" fill="#67C23A" /></svg>
          运行中
        </span>
        <span class="legend-item">
          <svg width="12" height="12"><circle cx="6" cy="6" r="5" fill="#c0c4cc" /></svg>
          空闲
        </span>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { relationsApi, type TeamGraph, type TeamGraphEdge, type TeamGraphNode } from '../api'

const router = useRouter()
const containerRef = ref<HTMLElement>()
const loading = ref(false)
const graph = ref<TeamGraph>({ nodes: [], edges: [] })

// ── Layout constants ───────────────────────────────────────────────────────
const svgWidth = 820
const NODE_R = 28
const LEVEL_H = 150  // vertical gap between levels
const PAD_TOP = 80   // top padding
const PAD_X = 80     // horizontal padding per side

const strengthWidths: Record<string, number> = {
  '核心': 4,
  '常用': 2.5,
  '偶尔': 1.5,
}

// ── Hierarchy level computation ────────────────────────────────────────────
// edge.type === '上级': edge.to is edge.from's boss → edge.to should be above (lower level number)
// edge.type === '下级': edge.to is edge.from's subordinate → edge.to should be below (higher level number)
// 平级协作 / 支持: no vertical shift

function computeLevels(nodes: TeamGraphNode[], edges: TeamGraphEdge[]): Record<string, number> {
  const levels: Record<string, number> = {}
  nodes.forEach(n => { levels[n.id] = 0 })

  // Iterative relaxation (converges in at most N passes for a DAG)
  const maxIter = nodes.length + 2
  for (let iter = 0; iter < maxIter; iter++) {
    let changed = false
    for (const edge of edges) {
      const lf = levels[edge.from] ?? 0
      const lt = levels[edge.to] ?? 0
      if (edge.type === '上级') {
        // to is boss of from → to should be above from → to.level < from.level
        const want = lf - 1
        if (lt > want) { levels[edge.to] = want; changed = true }
      } else if (edge.type === '下级') {
        // to is subordinate of from → to should be below from → to.level > from.level
        const want = lf + 1
        if (lt < want) { levels[edge.to] = want; changed = true }
      }
    }
    if (!changed) break
  }

  // Normalize so minimum level = 0 (top row)
  const vals = Object.values(levels)
  const minL = vals.length ? Math.min(...vals) : 0
  nodes.forEach(n => { levels[n.id] = (levels[n.id] ?? 0) - minL })
  return levels
}

// ── Position map ───────────────────────────────────────────────────────────
const levelMap = computed(() => computeLevels(graph.value.nodes, graph.value.edges))

const svgHeight = computed(() => {
  const maxLevel = Object.values(levelMap.value).reduce((m, v) => Math.max(m, v), 0)
  return Math.max(400, PAD_TOP + maxLevel * LEVEL_H + 140)
})

const posMap = computed<Record<string, { x: number; y: number }>>(() => {
  const nodes = graph.value.nodes
  const levels = levelMap.value

  // Group node ids by level
  const byLevel: Record<number, string[]> = {}
  nodes.forEach(n => {
    const lv = levels[n.id] ?? 0
    if (!byLevel[lv]) byLevel[lv] = []
    byLevel[lv].push(n.id)
  })

  const map: Record<string, { x: number; y: number }> = {}
  for (const [lvStr, ids] of Object.entries(byLevel)) {
    const lv = Number(lvStr)
    const y = PAD_TOP + lv * LEVEL_H
    const usableW = svgWidth - PAD_X * 2
    const gap = ids.length > 1 ? usableW / (ids.length - 1) : 0
    ids.forEach((id, i) => {
      const x = ids.length === 1
        ? svgWidth / 2
        : PAD_X + i * gap
      map[id] = { x: Math.round(x), y: Math.round(y) }
    })
  }
  return map
})

function nodePos(id: string): { x: number; y: number } {
  return posMap.value[id] ?? { x: svgWidth / 2, y: PAD_TOP }
}

// Shrink edge endpoint to node boundary
function edgePt(fromId: string, toId: string, end: 'start' | 'end'): { x: number; y: number } {
  const a = nodePos(fromId)
  const b = nodePos(toId)
  const dx = b.x - a.x
  const dy = b.y - a.y
  const len = Math.sqrt(dx * dx + dy * dy) || 1
  const r = NODE_R + 3
  if (end === 'start') return { x: a.x + (dx / len) * r, y: a.y + (dy / len) * r }
  return { x: b.x - (dx / len) * r, y: b.y - (dy / len) * r }
}

function edgeWidth(strength: string): number {
  return strengthWidths[strength] ?? 1.5
}

// ── Node helpers ───────────────────────────────────────────────────────────
const palette = ['#409EFF', '#67C23A', '#E6A23C', '#F56C6C', '#7C3AED', '#0891B2', '#B45309', '#64748B']

function nodeColor(id: string): string {
  let h = 0
  for (let i = 0; i < id.length; i++) h = id.charCodeAt(i) + ((h << 5) - h)
  return palette[Math.abs(h) % palette.length] ?? '#409EFF'
}

function nodeInitial(id: string): string {
  return (id || '?').charAt(0).toUpperCase()
}

function nodeName(id: string): string {
  return graph.value.nodes.find(n => n.id === id)?.name ?? id
}

function goToAgent(id: string) {
  router.push(`/agents/${id}`)
}

// ── Tooltip ────────────────────────────────────────────────────────────────
interface Tooltip { visible: boolean; x: number; y: number; type: string; strength: string; label: string; from: string; to: string }
const tooltip = ref<Tooltip>({ visible: false, x: 0, y: 0, type: '', strength: '', label: '', from: '', to: '' })

function showEdgeTooltip(e: MouseEvent, edge: TeamGraphEdge) {
  const rect = containerRef.value?.getBoundingClientRect()
  if (!rect) return
  tooltip.value = {
    visible: true,
    x: e.clientX - rect.left + 14,
    y: e.clientY - rect.top - 10,
    type: edge.type,
    strength: edge.strength,
    label: edge.label,
    from: edge.from,
    to: edge.to,
  }
}
function hideTooltip() { tooltip.value.visible = false }

// ── Load ───────────────────────────────────────────────────────────────────
async function loadGraph() {
  loading.value = true
  try {
    const res = await relationsApi.graph()
    graph.value = res.data
  } catch {
    ElMessage.error('加载图谱失败')
  } finally {
    loading.value = false
  }
}

async function clearAllRelations() {
  try {
    await ElMessageBox.confirm(
      '将清空所有成员的 RELATIONS.md，此操作不可恢复。确认吗？',
      '清空所有关系',
      { confirmButtonText: '确认清空', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return // user cancelled
  }
  try {
    await relationsApi.clearAll()
    ElMessage.success('已清空所有成员关系')
    await loadGraph()
  } catch {
    ElMessage.error('清空失败')
  }
}

onMounted(loadGraph)
</script>

<style scoped>
.team-view { padding: 0; }
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}
.page-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: #303133;
}
.graph-card { margin-bottom: 16px; }
.empty-state { padding: 60px 0; }
.graph-container {
  position: relative;
  display: flex;
  justify-content: center;
  overflow: hidden;
}
.graph-svg {
  display: block;
  max-width: 100%;
}
.graph-edge {
  cursor: pointer;
  transition: stroke-opacity 0.15s;
}
.graph-edge:hover { stroke-opacity: 1 !important; }
.graph-node {
  cursor: pointer;
  transition: opacity 0.15s;
}
.graph-node:hover { opacity: 0.85; }
.edge-tooltip {
  position: absolute;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 10px 14px;
  box-shadow: 0 6px 20px rgba(0,0,0,0.12);
  pointer-events: none;
  z-index: 200;
  min-width: 140px;
  max-width: 240px;
}
.tooltip-members {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  margin-bottom: 6px;
}
.tooltip-name { font-weight: 600; color: #303133; }
.tooltip-sep { color: #909399; }
.tooltip-tags { display: flex; gap: 6px; margin-bottom: 4px; }
.tooltip-label { font-size: 12px; color: #606266; line-height: 1.5; word-break: break-all; }
.legend-card { padding: 0; }
.legend {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px;
  font-size: 13px;
  color: #606266;
}
.legend-title { font-weight: 600; color: #303133; }
.legend-item { display: flex; align-items: center; gap: 5px; }
.legend-divider { color: #dcdfe6; user-select: none; }
</style>
