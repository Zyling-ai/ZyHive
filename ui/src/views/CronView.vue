<template>
  <div class="cron-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <h2 style="margin: 0">⏰ 定时任务</h2>
      <el-button type="primary" @click="showCreate = true">
        <el-icon><Plus /></el-icon> 新建任务
      </el-button>
    </div>

    <el-card shadow="hover">
      <el-table :data="jobs" stripe>
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column label="调度" min-width="200">
          <template #default="{ row }">{{ row.schedule?.expr }} ({{ row.schedule?.tz }})</template>
        </el-table-column>
        <el-table-column label="最近运行" width="200">
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
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggleCron(row)" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="runNow(row)">立即运行</el-button>
            <el-button size="small" type="danger" @click="deleteCron(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="jobs.length === 0" description="暂无定时任务" />
    </el-card>

    <el-dialog v-model="showCreate" title="新建定时任务" width="520px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="Cron 表达式">
          <el-input v-model="form.expr" placeholder="0 9 * * *" />
        </el-form-item>
        <el-form-item label="时区">
          <el-select v-model="form.tz">
            <el-option label="Asia/Shanghai" value="Asia/Shanghai" />
            <el-option label="UTC" value="UTC" />
            <el-option label="America/New_York" value="America/New_York" />
          </el-select>
        </el-form-item>
        <el-form-item label="消息">
          <el-input v-model="form.message" type="textarea" :rows="3" />
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
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { cron as cronApi, type CronJob } from '../api'

const jobs = ref<CronJob[]>([])
const showCreate = ref(false)
const form = reactive({ name: '', expr: '0 9 * * *', tz: 'Asia/Shanghai', message: '', enabled: true })

onMounted(loadJobs)

async function loadJobs() {
  try {
    const res = await cronApi.list()
    jobs.value = res.data || []
  } catch {}
}

function formatTime(ms: number) {
  return ms ? new Date(ms).toLocaleString() : ''
}

async function createCron() {
  try {
    await cronApi.create({
      name: form.name,
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

async function toggleCron(job: any) {
  try { await cronApi.update(job.id, job) } catch { ElMessage.error('更新失败') }
}

async function runNow(job: any) {
  try {
    await cronApi.run(job.id)
    ElMessage.success('已触发')
    setTimeout(loadJobs, 2000)
  } catch { ElMessage.error('失败') }
}

async function deleteCron(job: any) {
  try {
    await cronApi.delete(job.id)
    ElMessage.success('已删除')
    loadJobs()
  } catch { ElMessage.error('删除失败') }
}
</script>
