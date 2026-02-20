<template>
  <div class="cron-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
      <h2 style="margin: 0">⏰ 定时任务</h2>
      <el-button type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon> 新建任务
      </el-button>
    </div>

    <!-- Filter bar -->
    <div style="margin-bottom: 12px; display: flex; gap: 10px; align-items: center; flex-wrap: wrap">
      <el-text type="info" size="small" style="margin-right: 2px">筛选成员：</el-text>
      <el-radio-group v-model="filterAgentId" size="small" @change="loadJobs">
        <el-radio-button value="">全部</el-radio-button>
        <el-radio-button value="__global__">全局任务</el-radio-button>
        <el-radio-button v-for="ag in agentList" :key="ag.id" :value="ag.id">{{ ag.name }}</el-radio-button>
      </el-radio-group>
    </div>

    <el-card shadow="hover">
      <el-table :data="jobs" stripe>
        <el-table-column prop="name" label="名称" min-width="150" />
        <el-table-column label="所属成员" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.agentId" size="small" type="primary" style="cursor:pointer" @click="goToAgent(row)">
              {{ agentNameMap[row.agentId] || row.agentId }}
            </el-tag>
            <el-tag v-else size="small" type="info">全局</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.remark" style="font-size: 13px; color: #606266;">{{ row.remark }}</span>
            <span v-else style="color: #c0c4cc; font-size: 12px;">—</span>
          </template>
        </el-table-column>
        <el-table-column label="调度" min-width="160">
          <template #default="{ row }">
            <span style="font-size: 12px; font-family: monospace;">{{ row.schedule?.expr }}</span>
            <el-text type="info" size="small" style="margin-left: 4px;">({{ row.schedule?.tz }})</el-text>
          </template>
        </el-table-column>
        <el-table-column label="最近运行" width="170">
          <template #default="{ row }">
            <template v-if="row.state?.lastRunAtMs">
              <el-tag :type="row.state?.lastStatus === 'ok' ? 'success' : 'danger'" size="small">
                {{ row.state?.lastStatus }}
              </el-tag>
              <el-text type="info" size="small" style="margin-left: 4px">
                {{ formatTime(row.state?.lastRunAtMs) }}
              </el-text>
            </template>
            <el-text v-else type="info" size="small">未运行</el-text>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch
              v-model="row.enabled"
              @change="toggleCron(row)"
              size="small"
              :disabled="isMemoryJob(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="190">
          <template #default="{ row }">
            <template v-if="isMemoryJob(row)">
              <el-tag type="info" size="small" style="margin-right: 6px;">记忆管理</el-tag>
              <el-button size="small" @click="goToAgent(row)">查看</el-button>
            </template>
            <template v-else>
              <el-button size="small" @click="runNow(row)">立即运行</el-button>
              <el-button size="small" type="danger" @click="deleteCron(row)">删除</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="jobs.length === 0" description="暂无定时任务" />
    </el-card>

    <!-- Create Dialog -->
    <el-dialog v-model="showCreate" title="新建定时任务" width="520px">
      <el-form :model="form" label-width="110px">
        <el-form-item label="所属成员">
          <el-select v-model="form.agentId" placeholder="不选则为全局任务" clearable style="width: 100%">
            <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
          </el-select>
          <el-text type="info" size="small" style="display:block;margin-top:4px">
            💡 不选则为全局任务；选择成员后该任务只在成员的「定时任务」Tab 显示
          </el-text>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="任务名称" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="可选，说明这个任务的用途" />
        </el-form-item>
        <el-form-item label="Cron 表达式">
          <el-input v-model="form.expr" placeholder="0 9 * * *" />
          <el-text type="info" size="small" style="margin-top: 4px; display: block;">
            格式：秒(可选) 分 时 日 月 周。例：0 0 9 * * * = 每天09:00
          </el-text>
        </el-form-item>
        <el-form-item label="时区">
          <el-select v-model="form.tz" style="width: 100%">
            <el-option label="Asia/Shanghai" value="Asia/Shanghai" />
            <el-option label="UTC" value="UTC" />
            <el-option label="America/New_York" value="America/New_York" />
          </el-select>
        </el-form-item>
        <el-form-item label="消息内容">
          <el-input v-model="form.message" type="textarea" :rows="3" placeholder="发送给 Agent 的消息内容" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createCron">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { cron as cronApi, agents as agentsApi, type CronJob, type AgentInfo } from '../api'

const router = useRouter()
const jobs = ref<CronJob[]>([])
const agentList = ref<AgentInfo[]>([])
const filterAgentId = ref('')
const showCreate = ref(false)

const agentNameMap = computed(() => {
  const m: Record<string, string> = {}
  for (const ag of agentList.value) m[ag.id] = ag.name
  return m
})

const form = reactive({
  agentId: '',
  name: '',
  remark: '',
  expr: '0 0 9 * * *',
  tz: 'Asia/Shanghai',
  message: '',
  enabled: true,
})

onMounted(async () => {
  const res = await agentsApi.list().catch(() => ({ data: [] as AgentInfo[] }))
  agentList.value = res.data || []
  loadJobs()
})

async function loadJobs() {
  try {
    const res = await cronApi.list(filterAgentId.value || undefined)
    jobs.value = res.data || []
  } catch {}
}

function formatTime(ms: number) {
  return ms ? new Date(ms).toLocaleString('zh-CN') : ''
}

function isMemoryJob(row: CronJob): boolean {
  return row.payload?.message === '__MEMORY_CONSOLIDATE__'
}

function goToAgent(row: CronJob) {
  if (row.agentId) {
    router.push({ path: `/agents/${row.agentId}`, query: { tab: 'cron' } })
  }
}

function openCreate() {
  form.agentId = ''
  form.name = ''
  form.remark = ''
  form.expr = '0 0 9 * * *'
  form.tz = 'Asia/Shanghai'
  form.message = ''
  form.enabled = true
  showCreate.value = true
}

async function createCron() {
  try {
    await cronApi.create({
      name: form.name,
      remark: form.remark || undefined,
      agentId: form.agentId || undefined,
      enabled: form.enabled,
      schedule: { kind: 'cron', expr: form.expr, tz: form.tz },
      payload: { kind: 'agentTurn', message: form.message },
      delivery: { mode: 'announce' },
    } as any)
    ElMessage.success('创建成功')
    showCreate.value = false
    loadJobs()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  }
}

async function toggleCron(job: CronJob) {
  try { await cronApi.update(job.id, job as any) } catch { ElMessage.error('更新失败') }
}

async function runNow(job: CronJob) {
  try {
    await cronApi.run(job.id)
    ElMessage.success('已触发')
    setTimeout(loadJobs, 2000)
  } catch { ElMessage.error('触发失败') }
}

async function deleteCron(job: CronJob) {
  try {
    await cronApi.delete(job.id)
    ElMessage.success('已删除')
    loadJobs()
  } catch { ElMessage.error('删除失败') }
}
</script>

<style scoped>
.cron-page {
  padding: 20px;
}
</style>
