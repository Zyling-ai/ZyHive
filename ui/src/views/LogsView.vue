<template>
  <div class="logs-page">
    <!-- Header -->
    <div class="logs-header">
      <div>
        <h2 style="margin: 0"><el-icon style="vertical-align:-2px;margin-right:6px"><List /></el-icon>系统日志</h2>
        <div style="font-size: 13px; color: #909399; margin-top: 4px">
          共 {{ filteredLines.length }} 条 · 每 5 秒自动刷新
        </div>
      </div>
      <div style="display: flex; gap: 10px; align-items: center;">
        <el-input
          v-model="keyword"
          placeholder="关键词过滤…"
          clearable
          style="width: 220px"
          :prefix-icon="Search"
        />
        <el-switch v-model="autoScroll" active-text="自动滚动" />
        <el-button @click="fetchLogs" :loading="loading" :icon="Refresh">刷新</el-button>
      </div>
    </div>

    <!-- Log container -->
    <el-card shadow="never" class="log-card">
      <div ref="logContainer" class="log-container">
        <div
          v-for="(line, idx) in filteredLines"
          :key="idx"
          :class="['log-line', logLevel(line)]"
        >
          <span class="log-text">{{ line }}</span>
        </div>
        <div v-if="filteredLines.length === 0" class="log-empty">
          <el-empty :description="keyword ? '无匹配日志' : '暂无日志内容'" />
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { Search, Refresh } from '@element-plus/icons-vue'
import { logsApi } from '../api'

const lines = ref<string[]>([])
const keyword = ref('')
const loading = ref(false)
const autoScroll = ref(true)
const logContainer = ref<HTMLElement | null>(null)

let timer: ReturnType<typeof setInterval> | null = null

const filteredLines = computed(() => {
  if (!keyword.value) return lines.value
  const kw = keyword.value.toLowerCase()
  return lines.value.filter(l => l.toLowerCase().includes(kw))
})

function logLevel(line: string): string {
  const u = line.toUpperCase()
  if (u.includes('ERROR') || u.includes('[ERR]') || u.includes('FATAL')) return 'level-error'
  if (u.includes('WARN') || u.includes('[WARN]') || u.includes('WARNING')) return 'level-warn'
  if (u.includes('INFO') || u.includes('[INFO]')) return 'level-info'
  return 'level-default'
}

async function fetchLogs() {
  loading.value = true
  try {
    const res = await logsApi.get(500)
    lines.value = res.data.lines ?? []
    if (autoScroll.value) {
      await nextTick()
      scrollToBottom()
    }
  } catch {
    // silently fail — log file may not exist yet
  } finally {
    loading.value = false
  }
}

function scrollToBottom() {
  const el = logContainer.value
  if (el) el.scrollTop = el.scrollHeight
}

watch(autoScroll, (val) => {
  if (val) scrollToBottom()
})

watch(filteredLines, async () => {
  if (autoScroll.value) {
    await nextTick()
    scrollToBottom()
  }
})

onMounted(() => {
  fetchLogs()
  timer = setInterval(fetchLogs, 5000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.logs-page { display: flex; flex-direction: column; height: 100%; }

.logs-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.log-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.log-card :deep(.el-card__body) {
  flex: 1;
  padding: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.log-container {
  flex: 1;
  height: calc(100vh - 180px);
  overflow-y: auto;
  background: #1a1a2e;
  border-radius: 6px;
  padding: 12px 16px;
  font-family: 'Menlo', 'Monaco', 'Consolas', monospace;
  font-size: 12.5px;
  line-height: 1.7;
}

.log-line {
  padding: 1px 0;
  word-break: break-all;
  white-space: pre-wrap;
}

.log-text { display: block; }

/* Level colors */
.level-error .log-text  { color: #ff6b6b; }
.level-warn  .log-text  { color: #ffd93d; }
.level-info  .log-text  { color: #6bcb77; }
.level-default .log-text { color: #adb5bd; }

.log-empty {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
}
.log-empty :deep(.el-empty__description) { color: #666; }
</style>
