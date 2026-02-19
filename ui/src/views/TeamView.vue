<template>
  <div class="team-view">
    <div class="page-header">
      <h2>团队关系图谱</h2>
      <el-button size="small" @click="loadGraph">
        <el-icon><Refresh /></el-icon> 刷新
      </el-button>
    </div>

    <el-card v-loading="loading" class="graph-card">
      <!-- Empty state -->
      <div v-if="!loading && !graph.nodes.length" class="empty-state">
        <el-icon style="font-size: 64px; color: #c0c4cc; display: block; margin: 0 auto 16px"><Share /></el-icon>
        <p style="color: #909399; text-align: center; margin: 0;">
          暂无成员数据
        </p>
      </div>

      <div v-else-if="!loading && graph.nodes.length && !graph.edges.length" class="empty-state">
        <el-icon style="font-size: 64px; color: #c0c4cc; display: block; margin: 0 auto 16px"><Connection /></el-icon>
        <p style="color: #909399; text-align: center; margin: 0;">
          为成员配置 RELATIONS.md 即可自动生成关系图谱
        </p>
        <p style="color: #c0c4cc; text-align: center; font-size: 13px; margin: 6px 0 0;">
          前往成员详情页 → 「关系」Tab 编辑 RELATIONS.md
        </p>
      </div>

      <!-- SVG Graph -->
      <div v-else class="graph-container" ref="containerRef">
        <svg :width="svgWidth" :height="svgHeight" class="graph-svg">
          <!-- Arrowhead marker definitions -->
          <defs>
            <marker
              v-for="(color, type) in typeColors"
              :key="type"
              :id="`arrow-${encodeType(type)}`"
              markerWidth="8"
              markerHeight="8"
              refX="6"
              refY="3"
              orient="auto"
            >
              <path d="M0,0 L0,6 L8,3 z" :fill="color" />
            </marker>
          </defs>

          <!-- Edges -->
          <g v-for="edge in graph.edges" :key="`${edge.from}-${edge.to}`">
            <line
              :x1="edgeStart(edge).x"
              :y1="edgeStart(edge).y"
              :x2="edgeEnd(edge).x"
              :y2="edgeEnd(edge).y"
              :stroke="edgeColor(edge.type)"
              :stroke-width="edgeWidth(edge.strength)"
              stroke-opacity="0.85"
              :marker-end="`url(#arrow-${encodeType(edge.type)})`"
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
            <circle r="30" fill="rgba(0,0,0,0.08)" transform="translate(2,3)" />
            <!-- Main circle -->
            <circle
              r="28"
              :fill="nodeColor(node.id)"
              stroke="#fff"
              stroke-width="2.5"
            />
            <!-- Initial letter -->
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
              cx="20"
              cy="-20"
              r="6"
              :fill="node.status === 'running' ? '#67C23A' : '#c0c4cc'"
              stroke="#fff"
              stroke-width="1.5"
            />
            <!-- Name label -->
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
            <span class="tooltip-from">{{ nodeName(tooltip.from) }}</span>
            <span class="tooltip-arrow">→</span>
            <span class="tooltip-to">{{ nodeName(tooltip.to) }}</span>
          </div>
          <div class="tooltip-tags">
            <el-tag :type="typeTagColor(tooltip.type)" size="small">{{ tooltip.type }}</el-tag>
            <el-tag :type="strengthTagColor(tooltip.strength)" size="small" effect="plain">{{ tooltip.strength }}</el-tag>
          </div>
          <div v-if="tooltip.label" class="tooltip-label">{{ tooltip.label }}</div>
        </div>
      </div>
    </el-card>

    <!-- Legend -->
    <el-card v-if="graph.nodes.length" class="legend-card">
      <div class="legend">
        <span class="legend-title">关系类型：</span>
        <span v-for="(color, type) in typeColors" :key="type" class="legend-item">
          <svg width="28" height="6"><line x1="0" y1="3" x2="28" y2="3" :stroke="color" stroke-width="3" /></svg>
          {{ type }}
        </span>
        <span class="legend-divider"> | </span>
        <span class="legend-title">关系程度（线粗）：</span>
        <span v-for="(width, strength) in strengthWidths" :key="strength" class="legend-item">
          <svg width="28" height="8"><line x1="0" y1="4" x2="28" y2="4" stroke="#606266" :stroke-width="width" /></svg>
          {{ strength }}
        </span>
        <span class="legend-divider"> | </span>
        <span class="legend-item">
          <svg width="12" height="12"><circle cx="6" cy="6" r="5" fill="#67C23A" /></svg>
          运行中
        </span>
        <span class="legend-item">
          <svg width="12" height="12"><circle cx="6" cy="6" r="5" fill="#c0c4cc" /></svg>
          空闲/停止
        </span>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { relationsApi, type TeamGraph, type TeamGraphEdge } from '../api'

const router = useRouter()
const containerRef = ref<HTMLElement>()

const loading = ref(false)
const graph = ref<TeamGraph>({ nodes: [], edges: [] })

const svgWidth = 820
const svgHeight = 580
const cx = svgWidth / 2
const cy = svgHeight / 2 - 10
const nodeRadius = 28

// Compute radius based on node count for better spacing
function layoutRadius(n: number): number {
  if (n <= 1) return 0
  if (n <= 4) return 160
  if (n <= 7) return 200
  return Math.min(240, 60 * n / (2 * Math.PI) + 40)
}

// Map from node id → {x, y}
const posMap = computed<Record<string, { x: number; y: number }>>(() => {
  const nodes = graph.value.nodes
  const map: Record<string, { x: number; y: number }> = {}
  if (nodes.length === 1 && nodes[0]) {
    map[nodes[0].id] = { x: cx, y: cy }
    return map
  }
  const r = layoutRadius(nodes.length)
  nodes.forEach((n, i) => {
    const angle = (2 * Math.PI * i / nodes.length) - Math.PI / 2
    map[n.id] = {
      x: Math.round(cx + r * Math.cos(angle)),
      y: Math.round(cy + r * Math.sin(angle)),
    }
  })
  return map
})

function nodePos(id: string): { x: number; y: number } {
  return posMap.value[id] ?? { x: cx, y: cy }
}

// Shrink edge endpoints to node boundary so lines don't overlap circles.
// Direction: source (edge.from) → destination (edge.to)
function edgeStart(edge: TeamGraphEdge): { x: number; y: number } {
  const a = nodePos(edge.from)
  const b = nodePos(edge.to)
  const dx = b.x - a.x, dy = b.y - a.y
  const len = Math.sqrt(dx * dx + dy * dy) || 1
  const r = nodeRadius + 2
  return { x: a.x + (dx / len) * r, y: a.y + (dy / len) * r }
}

function edgeEnd(edge: TeamGraphEdge): { x: number; y: number } {
  const a = nodePos(edge.from)
  const b = nodePos(edge.to)
  const dx = b.x - a.x, dy = b.y - a.y
  const len = Math.sqrt(dx * dx + dy * dy) || 1
  const r = nodeRadius + 12 // extra space for arrowhead
  return { x: b.x - (dx / len) * r, y: b.y - (dy / len) * r }
}

const nodeColorPalette = ['#409EFF', '#67C23A', '#E6A23C', '#F56C6C', '#7C3AED', '#0891B2', '#B45309', '#64748B']

function nodeColor(id: string): string {
  let hash = 0
  for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash)
  return nodeColorPalette[Math.abs(hash) % nodeColorPalette.length] ?? '#409EFF'
}

function nodeInitial(id: string): string {
  return (id || '?').charAt(0).toUpperCase()
}

function nodeName(id: string): string {
  return graph.value.nodes.find(n => n.id === id)?.name ?? id
}

const typeColors: Record<string, string> = {
  '上级': '#F56C6C',
  '下级': '#409EFF',
  '平级协作': '#67C23A',
  '支持': '#909399',
}

const strengthWidths: Record<string, number> = {
  '核心': 4,
  '常用': 2.5,
  '偶尔': 1.5,
}

function edgeColor(type: string): string {
  return typeColors[type] ?? '#909399'
}

function edgeWidth(strength: string): number {
  return strengthWidths[strength] ?? 1.5
}

function encodeType(type: string): string {
  return encodeURIComponent(type).replace(/%/g, '_')
}

function goToAgent(id: string) {
  router.push(`/agents/${id}`)
}

// Tooltip
interface TooltipState {
  visible: boolean
  x: number
  y: number
  type: string
  strength: string
  label: string
  from: string
  to: string
}

const tooltip = ref<TooltipState>({
  visible: false, x: 0, y: 0, type: '', strength: '', label: '', from: '', to: '',
})

function showEdgeTooltip(e: MouseEvent, edge: TeamGraphEdge) {
  const container = containerRef.value
  if (!container) return
  const rect = container.getBoundingClientRect()
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

function hideTooltip() {
  tooltip.value.visible = false
}

function typeTagColor(type: string): '' | 'success' | 'warning' | 'info' | 'danger' {
  if (type === '上级') return 'danger'
  if (type === '下级') return ''
  if (type === '平级协作') return 'success'
  return 'info'
}

function strengthTagColor(s: string): '' | 'success' | 'warning' | 'info' | 'danger' {
  if (s === '核心') return 'danger'
  if (s === '常用') return 'warning'
  return 'info'
}

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

onMounted(loadGraph)
</script>

<style scoped>
.team-view {
  padding: 0;
}
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
.graph-card {
  margin-bottom: 16px;
}
.empty-state {
  padding: 60px 0;
}
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
  transition: stroke-opacity 0.15s, stroke-width 0.15s;
}
.graph-edge:hover {
  stroke-opacity: 1 !important;
}
.graph-node {
  cursor: pointer;
  transition: opacity 0.15s;
}
.graph-node:hover {
  opacity: 0.85;
}
.edge-tooltip {
  position: absolute;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 10px 14px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
  pointer-events: none;
  z-index: 200;
  min-width: 140px;
  max-width: 240px;
}
.tooltip-members {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #606266;
  margin-bottom: 6px;
}
.tooltip-from, .tooltip-to {
  font-weight: 600;
  color: #303133;
}
.tooltip-arrow {
  color: #909399;
}
.tooltip-tags {
  display: flex;
  gap: 6px;
  margin-bottom: 4px;
}
.tooltip-label {
  font-size: 12px;
  color: #606266;
  line-height: 1.5;
  word-break: break-all;
}
.legend-card {
  padding: 0;
}
.legend {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px;
  font-size: 13px;
  color: #606266;
}
.legend-title {
  font-weight: 600;
  color: #303133;
}
.legend-item {
  display: flex;
  align-items: center;
  gap: 5px;
}
.legend-divider {
  color: #dcdfe6;
  user-select: none;
}
</style>
