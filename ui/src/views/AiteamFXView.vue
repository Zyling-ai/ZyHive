<!-- AiteamFXView placeholder. Full content lands in P2-S5/S6/S7. -->
<template>
  <div class="aiteam-stub">
    <h1>{{ title }}</h1>
    <el-empty description="此页面在 Phase 2 后续阶段填充">
      <p style="color:#888;font-size:13px;max-width:520px">
        P2-S4 落骨架（路由 + 顶栏切换器 + 总览页）；
        AiteamFXView 的完整 UI 将在 P2-S5 / P2-S6 / P2-S7 中陆续填充。
        当前 placeholder 仅占位避免路由 404。
      </p>
      <el-button type="primary" @click="goHome">回到总览</el-button>
    </el-empty>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
const router = useRouter()
function goHome() { router.push('/aiteam') }

const titleMap: Record<string, string> = {
  AiteamWalletView:  '🧪 aiteam · 钱包',
  AiteamFXView:      '🧪 aiteam · 汇率',
  AiteamGuardView:   '🧪 aiteam · 护栏',
  AiteamJudgeView:   '🧪 aiteam · 评分',
  AiteamPayrollView: '🧪 aiteam · 工资',
}
const title = titleMap.AiteamFXView
</script>

<style scoped>
.aiteam-stub { padding: 40px; text-align: center; }
</style>
