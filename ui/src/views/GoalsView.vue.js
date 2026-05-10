/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, reactive, onMounted, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Plus, Refresh, Delete, Flag, Loading, ChatLineRound, DocumentChecked, Close, ArrowLeft } from '@element-plus/icons-vue';
import { goalsApi, agents as agentsApi, } from '../api';
import AiChat from '../components/AiChat.vue';
// ── 布局状态 ─────────────────────────────────────────────────────────────
const sideW = ref(260);
const chatW = ref(360);
const dragging = ref('');
let startX = 0, startW2 = 0;
function startResize(e, target) {
    dragging.value = target;
    startX = e.clientX;
    startW2 = target === 'side' ? sideW.value : chatW.value;
    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
    e.preventDefault();
}
function onMouseMove(e) {
    const d = e.clientX - startX;
    if (dragging.value === 'side')
        sideW.value = Math.max(200, Math.min(400, startW2 + d));
    else
        chatW.value = Math.max(280, Math.min(560, startW2 - d));
}
function onMouseUp() {
    dragging.value = '';
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
}
// ── 数据状态 ─────────────────────────────────────────────────────────────
const goals = ref([]);
const agentList = ref([]);
const filterStatus = ref('');
const filterAgentId = ref('');
const selectedGoal = ref(null);
const creating = ref(false);
const saving = ref(false);
const editorTab = ref('basic');
const checkDialogVisible = ref(false);
const checkRecords = ref([]);
const checkRecordsLoading = ref(false);
const checkFreqPreset = ref('0 9 * * 1');
const selectedChatAgentId = ref('');
// ── 表单 ─────────────────────────────────────────────────────────────────
const form = reactive({
    title: '',
    description: '',
    type: 'team',
    agentIds: [],
    status: 'draft',
    progress: 0,
    startAt: '',
    endAt: '',
    milestones: [],
});
const checkForm = reactive({
    name: '',
    agentId: '',
    schedule: '0 9 * * 1',
    tz: 'Asia/Shanghai',
    prompt: '请检查目标「{goal.title}」的进展情况（当前进度 {goal.progress}），距截止日期 {goal.endAt} 还有一段时间，请总结近期进展并建议下一步行动。',
    enabled: true,
});
// ── 计算属性 ─────────────────────────────────────────────────────────────
const agentNameMap = computed(() => {
    const m = {};
    agentList.value.forEach(a => { m[a.id] = a.name; });
    return m;
});
const agentColorMap = computed(() => {
    const m = {};
    agentList.value.forEach(a => { m[a.id] = a.avatarColor || '#409eff'; });
    return m;
});
const filteredGoals = computed(() => {
    let list = [...goals.value];
    if (filterStatus.value)
        list = list.filter(g => g.status === filterStatus.value);
    if (filterAgentId.value)
        list = list.filter(g => (g.agentIds || []).includes(filterAgentId.value));
    return list;
});
// ── 甘特图：连续缩放 + 惯性平移（地图式操作）────────────────────────────
const GANTT_MIN_MS = 2 * 60_000; // 最小可见时长：2 分钟
const GANTT_MAX_MS = 20 * 365 * 86400_000; // 最大可见时长：20 年
const GANTT_INIT_MS = 30 * 86400_000; // 默认：30 天（天级别，避免 |0 溢出）
// 可见时长（连续值，取代离散 scale）
const ganttDuration = ref(GANTT_INIT_MS);
// 视口中心时刻：初始让今天出现在左侧约 10% 处
const viewCenterMs = ref(Date.now() + GANTT_INIT_MS * 0.4);
const MS_MONTH = Math.round(30.44 * 86400_000); // ~2,630,016,000 — 浮点安全
const TICK_STEPS = [
    { ms: 60_000, kind: 'minute', step: 1, label: '1分钟' },
    { ms: 5 * 60_000, kind: 'minute', step: 5, label: '5分钟' },
    { ms: 15 * 60_000, kind: 'minute', step: 15, label: '15分钟' },
    { ms: 30 * 60_000, kind: 'minute', step: 30, label: '30分钟' },
    { ms: 3600_000, kind: 'hour', step: 1, label: '1小时' },
    { ms: 6 * 3600_000, kind: 'hour', step: 6, label: '6小时' },
    { ms: 12 * 3600_000, kind: 'hour', step: 12, label: '12小时' },
    { ms: 86400_000, kind: 'day', step: 1, label: '1天' },
    { ms: 7 * 86400_000, kind: 'week', step: 7, label: '1周' },
    { ms: MS_MONTH, kind: 'month', step: 1, label: '1月' },
    { ms: 3 * MS_MONTH, kind: 'month', step: 3, label: '3月' },
    { ms: 6 * MS_MONTH, kind: 'month', step: 6, label: '6月' },
    { ms: 365 * 86400_000, kind: 'year', step: 1, label: '1年' },
    { ms: 2 * 365 * 86400_000, kind: 'year', step: 2, label: '2年' },
    { ms: 5 * 365 * 86400_000, kind: 'year', step: 5, label: '5年' },
];
// 自动选最近似「目标 8 格」的步长
const tickStep = computed(() => {
    const target = ganttDuration.value / 8;
    return TICK_STEPS.reduce((b, s) => Math.abs(s.ms - target) < Math.abs(b.ms - target) ? s : b);
});
// 甘特图可见范围（无吸附，连续平滑）
const ganttRange = computed(() => ({
    start: new Date(viewCenterMs.value - ganttDuration.value / 2),
    end: new Date(viewCenterMs.value + ganttDuration.value / 2),
}));
const gridTicks = computed(() => calcGridTicks(ganttRange.value.start, ganttRange.value.end, tickStep.value));
// 追踪时间轴容器宽度，用于动态过滤标签密度
const ganttTimelineW = ref(700);
const ganttTimelineRef = ref(null);
const labelTicks = computed(() => {
    const ticks = gridTicks.value;
    const w = ganttTimelineW.value;
    if (!ticks.length || !w)
        return ticks;
    const minPx = 50;
    const maxLabels = Math.max(1, Math.floor(w / minPx));
    if (ticks.length <= maxLabels)
        return ticks;
    const step = Math.ceil(ticks.length / maxLabels);
    return ticks.filter((_, i) => i % step === 0);
});
// 网格线使用全密度刻度
const monthLabels = gridTicks;
// ── 拖拽平移（非响应式变量，避免 Vue 追踪开销）──────────────────────────
const ganttDragging = ref(false);
const ganttDragged = ref(false);
let _gDragActive = false, _gDragMoved = false;
let _gLastX = 0, _gLastT = 0, _gVel = 0, _gMomentumId = 0;
function _cancelMomentum() { cancelAnimationFrame(_gMomentumId); _gVel = 0; }
function onGanttMouseDown(e) {
    _cancelMomentum();
    _gDragActive = true;
    _gDragMoved = false;
    ganttDragging.value = true;
    ganttDragged.value = false;
    _gLastX = e.clientX;
    _gLastT = Date.now();
    _gVel = 0;
}
function onGanttMouseMove(e) {
    if (!_gDragActive)
        return;
    const dx = e.clientX - _gLastX;
    const dt = Date.now() - _gLastT;
    if (Math.abs(dx) > 3) {
        _gDragMoved = true;
        ganttDragged.value = true;
    }
    if (_gDragMoved) {
        const w = Math.max(100, ganttTimelineW.value || 700);
        // maxV 正比于屏幕宽度：确保惯性距离与屏幕宽窄无关，始终 ≈ 1/3 × ganttDuration
        // 推导：total = (w/K)/w × ganttDuration × 16/0.12 → total/ganttDuration = 16/(K×0.12)
        // 取 K=400 → total ≈ 0.33 × ganttDuration（无论手机/桌面、30天/90天视图）
        const K = 400;
        const maxV = Math.max(100, ganttTimelineW.value || 700) / K;
        const rawV = dt > 0 ? dx / dt : 0;
        const clampedV = Math.max(-maxV, Math.min(maxV, rawV));
        if (dt > 0)
            _gVel = _gVel * 0.65 + clampedV * 0.35;
        viewCenterMs.value -= (dx / w) * ganttDuration.value;
    }
    _gLastX = e.clientX;
    _gLastT = Date.now();
}
function onGanttMouseUp() {
    if (!_gDragActive)
        return;
    _gDragActive = false;
    ganttDragging.value = false;
    if (_gDragMoved && Math.abs(_gVel) > 0.05) {
        let v = _gVel, lt = Date.now();
        const run = () => {
            const now = Date.now(), dt = Math.min(now - lt, 64);
            lt = now; // dt 最多 64ms，防止帧延迟导致大跳
            const w = Math.max(100, ganttTimelineW.value || 700); // 宽度至少 100px，防止除以超小值
            viewCenterMs.value -= v * dt / w * ganttDuration.value;
            // viewCenterMs 夹在合理范围：前后各 5 年
            const fiveYears = 5 * 365 * 86400_000;
            viewCenterMs.value = Math.max(Date.now() - fiveYears, Math.min(Date.now() + fiveYears, viewCenterMs.value));
            v *= Math.pow(0.88, dt / 16); // 摩擦力：0.88/16ms，比 0.92 衰减更快，避免惯性飞太远
            if (Math.abs(v) > 0.008)
                _gMomentumId = requestAnimationFrame(run);
        };
        _gMomentumId = requestAnimationFrame(run);
    }
}
// ── 滚轮缩放（向光标位置锚定，指数线性）──────────────────────────────────
function handleGanttWheel(e) {
    e.preventDefault();
    _cancelMomentum();
    const factor = Math.exp(e.deltaY * 0.0015); // 每 deltaY=100 约缩放 16%
    const newDur = Math.max(GANTT_MIN_MS, Math.min(GANTT_MAX_MS, ganttDuration.value * factor));
    // 以光标所在时刻为锚点，保证缩放后光标下的时间不变
    const rect = ganttTimelineRef.value?.getBoundingClientRect();
    if (rect && rect.width > 0) {
        const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
        const timeAtCursor = (viewCenterMs.value - ganttDuration.value / 2) + ratio * ganttDuration.value;
        viewCenterMs.value = timeAtCursor + newDur * (0.5 - ratio);
    }
    ganttDuration.value = newDur;
}
// ── 甘特图摘要选中 ──────────────────────────────────────────────────────
const ganttSelectedGoal = ref(null);
function onGanttBarClick(g) {
    if (ganttDragged.value)
        return;
    ganttSelectedGoal.value = ganttSelectedGoal.value?.id === g.id ? null : g;
}
const todayLeft = computed(() => {
    const { start, end } = ganttRange.value;
    const now = Date.now();
    if (now < start.getTime() || now > end.getTime())
        return null;
    return `${((now - start.getTime()) / (end.getTime() - start.getTime())) * 100}%`;
});
// AI 聊天上下文
const goalChatContext = computed(() => {
    const token = localStorage.getItem('aipanel_token') || 'TOKEN';
    const base = `${window.location.protocol}//${window.location.host}`;
    const agentCtx = agentList.value.map(a => `- ${a.id}: ${a.name}`).join('\n');
    const currentGoalCtx = selectedGoal.value
        ? `\n### 当前选中目标\nID: ${selectedGoal.value.id}\n标题: ${selectedGoal.value.title}\n状态: ${selectedGoal.value.status}\n进度: ${selectedGoal.value.progress}%\n开始: ${selectedGoal.value.startAt || '未设置'}\n结束: ${selectedGoal.value.endAt || '未设置'}`
        : '';
    return `## 目标规划助手

你是团队的目标规划助手。

### 🎯 核心能力：填写表单（优先使用）

当用户描述目标信息时，**直接输出 JSON 填充表单**，格式如下：

\`\`\`json
{"action":"fill_goal","data":{"title":"目标标题","description":"描述（可选）","type":"team","agentIds":["agentId1"],"status":"active","startAt":"2026-03-01T00:00:00Z","endAt":"2026-06-30T00:00:00Z","progress":0,"milestones":[{"title":"里程碑1","dueAt":"2026-04-01T00:00:00Z","done":false}]}}
\`\`\`

- type: "personal"（个人）或 "team"（团队）
- status: "draft" / "active" / "completed" / "cancelled"
- agentIds: 参与成员的 ID 列表
- 时间格式：ISO 8601（如 "2026-03-01T00:00:00Z"）
- 输出 JSON 后，页面会自动填充表单，用户确认后保存

### API 操作（如需直接更新已有目标）

**更新进度：**
\`\`\`bash
curl -s -X PATCH ${base}/api/goals/{id}/progress -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"progress":50}'
\`\`\`

**添加定期检查：**
\`\`\`bash
curl -s -X POST ${base}/api/goals/{id}/checks -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"name":"每周检查","schedule":"0 9 * * 1","agentId":"agentId","tz":"Asia/Shanghai","prompt":"请检查目标「{goal.title}」本周进展","enabled":true}'
\`\`\`

### 当前团队成员
${agentCtx}
${currentGoalCtx}

**工作流程：**
1. 用户描述目标 → 你输出 fill_goal JSON → 页面自动填表
2. 用户确认内容后自行点击「创建」或「保存」按钮
3. 如需更新已存在目标的进度/检查，使用 API 操作`.trim();
});
// ── 生命周期 ─────────────────────────────────────────────────────────────
onMounted(async () => {
    const res = await agentsApi.list().catch(() => ({ data: [] }));
    agentList.value = (res.data || []).filter(a => !a.system);
    selectedChatAgentId.value = agentList.value[0]?.id || '';
    await loadGoals();
});
// ganttTimelineRef 在 v-else 条件块内，DOM 渲染后才会挂载
// 用 watch 监听 ref 变化，避免 onMounted 时 ref 为 null
watch(ganttTimelineRef, (el) => {
    if (!el)
        return;
    const ro = new ResizeObserver(entries => {
        if (entries[0])
            ganttTimelineW.value = entries[0].contentRect.width;
    });
    ro.observe(el);
    // 立即读取一次当前宽度
    ganttTimelineW.value = el.getBoundingClientRect().width;
}, { immediate: true });
watch(editorTab, async (tab) => {
    if (tab === 'records' && selectedGoal.value) {
        await loadCheckRecords(selectedGoal.value.id);
    }
});
// ── 数据加载 ─────────────────────────────────────────────────────────────
async function loadGoals() {
    try {
        const res = await goalsApi.list();
        goals.value = res.data || [];
    }
    catch (e) {
        ElMessage.error('加载目标失败: ' + (e?.message || '未知错误'));
        return;
    }
    try {
        // Refresh selectedGoal if still present
        if (selectedGoal.value) {
            const updated = goals.value.find(g => g.id === selectedGoal.value.id);
            if (updated)
                selectedGoal.value = updated;
        }
    }
    catch { }
}
async function loadCheckRecords(goalId) {
    checkRecordsLoading.value = true;
    try {
        const res = await goalsApi.listCheckRecords(goalId);
        checkRecords.value = (res.data || []).slice().reverse();
    }
    catch {
        checkRecords.value = [];
    }
    finally {
        checkRecordsLoading.value = false;
    }
}
// ── 选择/新建 ─────────────────────────────────────────────────────────────
function selectGoal(g) {
    selectedGoal.value = g;
    creating.value = false;
    editorTab.value = 'basic';
    Object.assign(form, {
        title: g.title,
        description: g.description || '',
        type: g.type,
        agentIds: [...(g.agentIds || [])],
        status: g.status,
        progress: g.progress,
        startAt: isValidDate(g.startAt) ? g.startAt : '',
        endAt: isValidDate(g.endAt) ? g.endAt : '',
        milestones: (g.milestones || []).map(m => ({ ...m })),
    });
}
function openCreate() {
    selectedGoal.value = null;
    creating.value = true;
    editorTab.value = 'basic';
    createSessionStamp.value = Date.now(); // 每次新建都刷新 session
    Object.assign(form, {
        title: '', description: '', type: 'team', agentIds: [],
        status: 'draft', progress: 0, startAt: '', endAt: '', milestones: [],
    });
}
// ── 保存/删除 ─────────────────────────────────────────────────────────────
async function saveGoal() {
    if (!form.title.trim()) {
        ElMessage.warning('请填写目标标题');
        return;
    }
    saving.value = true;
    const payload = {
        title: form.title,
        description: form.description || undefined,
        type: form.type,
        agentIds: form.agentIds,
        status: form.status,
        progress: form.progress,
        startAt: form.startAt || undefined,
        endAt: form.endAt || undefined,
        milestones: form.milestones.map(m => ({
            ...m,
            id: m.id || 'ms-' + Math.random().toString(36).slice(2, 10),
        })),
    };
    try {
        if (selectedGoal.value) {
            await goalsApi.update(selectedGoal.value.id, payload);
            ElMessage.success('保存成功');
            await loadGoals();
        }
        else {
            const res = await goalsApi.create(payload);
            ElMessage.success('创建成功');
            creating.value = false;
            await loadGoals();
            // 自动选中刚创建的目标
            const newGoal = goals.value.find(g => g.id === res.data.id) || res.data;
            selectGoal(newGoal);
        }
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '操作失败');
    }
    finally {
        saving.value = false;
    }
}
async function deleteGoal() {
    if (!selectedGoal.value)
        return;
    try {
        await goalsApi.delete(selectedGoal.value.id);
        ElMessage.success('已删除');
        selectedGoal.value = null;
        await loadGoals();
    }
    catch {
        ElMessage.error('删除失败');
    }
}
function addMilestone() {
    form.milestones.push({
        id: 'ms-' + Math.random().toString(36).slice(2, 10),
        title: '', dueAt: '', done: false, agentIds: [],
    });
}
// ── 定期检查 ─────────────────────────────────────────────────────────────
function openAddCheckDialog() {
    Object.assign(checkForm, {
        name: '', agentId: agentList.value[0]?.id || '',
        schedule: '0 9 * * 1', tz: 'Asia/Shanghai',
        prompt: '请检查目标「{goal.title}」的进展情况（当前进度 {goal.progress}），距截止日期 {goal.endAt} 还有一段时间，请总结近期进展并建议下一步行动。',
        enabled: true,
    });
    checkFreqPreset.value = '0 9 * * 1';
    checkDialogVisible.value = true;
}
function onPresetChange(val) {
    if (val !== 'custom')
        checkForm.schedule = val;
}
async function submitAddCheck() {
    if (!checkForm.name.trim()) {
        ElMessage.warning('请填写检查名称');
        return;
    }
    if (!checkForm.agentId) {
        ElMessage.warning('请选择执行成员');
        return;
    }
    if (!selectedGoal.value)
        return;
    try {
        await goalsApi.addCheck(selectedGoal.value.id, { ...checkForm });
        ElMessage.success('添加成功');
        checkDialogVisible.value = false;
        const res = await goalsApi.get(selectedGoal.value.id);
        selectedGoal.value = res.data;
        await loadGoals();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '添加失败');
    }
}
async function toggleCheck(check) {
    if (!selectedGoal.value)
        return;
    try {
        await goalsApi.updateCheck(selectedGoal.value.id, check.id, { enabled: check.enabled });
    }
    catch {
        ElMessage.error('更新失败');
    }
}
async function runCheckNow(check) {
    if (!selectedGoal.value)
        return;
    try {
        await goalsApi.runCheck(selectedGoal.value.id, check.id);
        ElMessage.success('已触发检查');
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '触发失败');
    }
}
async function removeCheck(check) {
    if (!selectedGoal.value)
        return;
    try {
        await ElMessageBox.confirm(`确定删除检查计划「${check.name}」？`, '删除确认', { type: 'warning' });
        await goalsApi.removeCheck(selectedGoal.value.id, check.id);
        ElMessage.success('已删除');
        const res = await goalsApi.get(selectedGoal.value.id);
        selectedGoal.value = res.data;
        await loadGoals();
    }
    catch (e) {
        if (e !== 'cancel')
            ElMessage.error('删除失败');
    }
}
// 每次打开"新建目标"时生成独立 session，防止复用历史对话
const createSessionStamp = ref(0);
// 每个目标独立的对话 session（切换目标自动切换历史）
const goalChatSessionId = computed(() => {
    if (!selectedChatAgentId.value)
        return '';
    if (selectedGoal.value)
        return `goal-${selectedGoal.value.id}-${selectedChatAgentId.value}`;
    // 新建目标：每次 openCreate() 都会更新 stamp，保证 session 全新
    return `goals-new-${createSessionStamp.value}-${selectedChatAgentId.value}`;
});
// AI 输出 JSON 后自动填充表单
function onAiResponse(text) {
    // 刷新目标列表
    setTimeout(() => loadGoals(), 2000);
    // 尝试解析 JSON fill 指令
    // 支持两种格式：
    //   {"action":"fill_goal","data":{...}}
    //   ```json\n{"action":"fill_goal",...}\n```
    const tryFill = (jsonStr) => {
        try {
            const obj = JSON.parse(jsonStr);
            if (obj.action === 'fill_goal' && obj.data) {
                applyFormFill(obj.data);
                return true;
            }
        }
        catch { }
        return false;
    };
    // 先尝试代码块内
    const codeBlock = text.match(/```(?:json)?\s*(\{[\s\S]*?\})\s*```/);
    if (codeBlock?.[1] && tryFill(codeBlock[1]))
        return;
    // 再尝试裸 JSON
    const bare = text.match(/(\{"action"\s*:\s*"fill_goal"[\s\S]*?\})/);
    if (bare?.[1] && tryFill(bare[1]))
        return;
}
function applyFormFill(data) {
    if (!creating.value && !selectedGoal.value) {
        // 先进入新建状态
        openCreate();
    }
    if (data.title)
        form.title = data.title;
    if (data.description)
        form.description = data.description;
    if (data.type)
        form.type = data.type;
    if (data.status)
        form.status = data.status;
    if (typeof data.progress === 'number')
        form.progress = data.progress;
    if (data.agentIds && Array.isArray(data.agentIds))
        form.agentIds = data.agentIds;
    if (data.startAt)
        form.startAt = data.startAt;
    if (data.endAt)
        form.endAt = data.endAt;
    if (data.milestones && Array.isArray(data.milestones)) {
        form.milestones = data.milestones.map((m) => ({
            id: m.id || 'ms-' + Math.random().toString(36).slice(2, 10),
            title: m.title || '',
            dueAt: m.dueAt || '',
            done: !!m.done,
            agentIds: m.agentIds || [],
        }));
    }
    ElMessage.success('AI 已填写表单，确认后点击保存');
}
// ── 甘特图辅助 ────────────────────────────────────────────────────────────
function isValidDate(val) {
    if (!val)
        return false;
    const d = new Date(val);
    return !isNaN(d.getTime()) && d.getFullYear() > 1970;
}
function isValidBar(g) {
    return isValidDate(g.startAt) && isValidDate(g.endAt);
}
function ganttBarRange(g) {
    const { start, end } = ganttRange.value;
    const total = end.getTime() - start.getTime();
    const gS = new Date(g.startAt).getTime();
    const gE = new Date(g.endAt).getTime();
    // 关键：right 用右端位置减去左端夹住后的值，否则 gS 超出左侧时 width 不随滚动缩减
    const leftRaw = (gS - start.getTime()) / total * 100;
    const rightRaw = (gE - start.getTime()) / total * 100;
    const left = Math.max(0, leftRaw);
    const right = Math.max(left, rightRaw); // right 不能小于 left
    const width = Math.max(1, right - left); // 可见宽度（随滚动动态缩减）
    return { left, width };
}
function calcBarWidth(g) {
    return ganttBarRange(g).width;
}
function ganttBarStyle(g) {
    const { left, width } = ganttBarRange(g);
    const c1 = (g.agentIds?.[0] && agentColorMap.value[g.agentIds[0]]) ? agentColorMap.value[g.agentIds[0]] : '#409eff';
    const c2 = (g.agentIds?.[1] && agentColorMap.value[g.agentIds[1]]) ? agentColorMap.value[g.agentIds[1]] : c1;
    return { left: `${left}%`, width: `${width}%`, background: g.agentIds?.length > 1 ? `linear-gradient(90deg,${c1},${c2})` : c1 };
}
function milestoneLeft(ms) {
    const { start, end } = ganttRange.value;
    if (!isValidDate(ms.dueAt))
        return '-100%';
    return `${((new Date(ms.dueAt).getTime() - start.getTime()) / (end.getTime() - start.getTime())) * 100}%`;
}
function calcGridTicks(rangeStart, rangeEnd, tick) {
    const ticks = [];
    const total = rangeEnd.getTime() - rangeStart.getTime();
    if (total <= 0 || !isFinite(total))
        return ticks;
    // 保护：最多渲染 300 条刻度线。若当前步长会超出，自动升档到更粗的步长
    const MAX_TICKS = 300;
    const estimatedCount = total / tick.ms;
    if (estimatedCount > MAX_TICKS) {
        const saferTick = [...TICK_STEPS].reverse().find(s => total / s.ms <= MAX_TICKS);
        if (saferTick)
            tick = saferTick;
    }
    const pct = (d) => ((d.getTime() - rangeStart.getTime()) / total * 100).toFixed(2) + '%';
    let seenParent = '';
    // 找到 rangeStart 之前最近的对齐刻度点
    let cur;
    if (tick.kind === 'minute') {
        cur = new Date(rangeStart);
        cur.setSeconds(0, 0);
        cur.setMinutes(Math.floor(cur.getMinutes() / tick.step) * tick.step);
    }
    else if (tick.kind === 'hour') {
        cur = new Date(rangeStart);
        cur.setMinutes(0, 0, 0);
        cur.setHours(Math.floor(cur.getHours() / tick.step) * tick.step);
    }
    else if (tick.kind === 'day') {
        cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), rangeStart.getDate());
    }
    else if (tick.kind === 'week') {
        cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), rangeStart.getDate());
        const dow = cur.getDay();
        cur.setDate(cur.getDate() - (dow === 0 ? 6 : dow - 1));
    }
    else if (tick.kind === 'month') {
        const mo = rangeStart.getMonth();
        cur = new Date(rangeStart.getFullYear(), Math.floor(mo / tick.step) * tick.step, 1);
    }
    else { // year
        cur = new Date(Math.floor(rangeStart.getFullYear() / tick.step) * tick.step, 0, 1);
    }
    while (cur.getTime() <= rangeEnd.getTime()) {
        const yr = cur.getFullYear(), mo = cur.getMonth() + 1;
        const d = cur.getDate(), h = cur.getHours(), m = cur.getMinutes();
        // 上层父标签（日期/月份/年份变化时显示）
        let parentKey = '', parentLabel = '';
        if (tick.kind === 'minute' || tick.kind === 'hour') {
            parentKey = `${yr}-${mo}-${d}`;
            parentLabel = `${mo}/${d}`;
        }
        else if (tick.kind === 'day' || tick.kind === 'week') {
            parentKey = `${yr}-${mo}`;
            parentLabel = mo === 1 ? `${yr}年` : `${mo}月`;
        }
        else if (tick.kind === 'month') {
            parentKey = String(yr);
            parentLabel = String(yr);
        }
        const yearMark = (parentKey && parentKey !== seenParent) ? parentLabel : undefined;
        if (yearMark)
            seenParent = parentKey;
        // 刻度主标签
        let label = '';
        if (tick.kind === 'minute')
            label = `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
        else if (tick.kind === 'hour')
            label = `${String(h).padStart(2, '0')}时`;
        else if (tick.kind === 'day')
            label = String(d);
        else if (tick.kind === 'week')
            label = `${mo}/${d}`;
        else if (tick.kind === 'month')
            label = `${mo}月`;
        else
            label = String(yr);
        ticks.push({ label, yearMark, left: pct(cur), ts: cur.getTime() });
        // 步进到下一刻度
        if (tick.kind === 'minute')
            cur.setMinutes(cur.getMinutes() + tick.step);
        else if (tick.kind === 'hour')
            cur.setHours(cur.getHours() + tick.step);
        else if (tick.kind === 'day' || tick.kind === 'week')
            cur.setDate(cur.getDate() + tick.step);
        else if (tick.kind === 'month')
            cur.setMonth(cur.getMonth() + tick.step);
        else
            cur.setFullYear(cur.getFullYear() + tick.step);
    }
    return ticks;
}
// 时间进度：(今天 - 开始日) / (结束日 - 开始日) × 100，夹在 0~100
function timeProgress(g) {
    if (g.status === 'completed')
        return 100;
    if (!isValidDate(g.startAt) || !isValidDate(g.endAt))
        return g.progress;
    const now = Date.now();
    const start = new Date(g.startAt).getTime();
    const end = new Date(g.endAt).getTime();
    if (end <= start)
        return g.progress;
    return Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)));
}
function progressColor(g) {
    if (g.status === 'completed')
        return '#67c23a';
    const tp = timeProgress(g);
    if (tp >= 80)
        return '#409eff';
    if (tp >= 40)
        return '#e6a23c';
    return '#909399';
}
// ── 辅助 ─────────────────────────────────────────────────────────────────
function statusLabel(s) {
    return { draft: '草稿', active: '进行中', completed: '已完成', cancelled: '已取消' }[s] ?? s;
}
function statusTagType(s) {
    return { draft: 'info', active: '', completed: 'success', cancelled: 'danger' }[s] ?? 'info';
}
function formatDate(val) {
    if (!isValidDate(val))
        return '—';
    return new Date(val).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' });
}
function formatDateTime(val) {
    if (!val)
        return '';
    const d = new Date(val);
    return isNaN(d.getTime()) ? '' : d.toLocaleString('zh-CN');
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['goal-item']} */ ;
/** @type {__VLS_StyleScopedClasses['goal-item']} */ ;
/** @type {__VLS_StyleScopedClasses['gs-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['gs-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-overlay']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-overlay']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-label-col']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-timeline-col']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-row']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-row']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-row']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-label-col']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-row']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-timeline-col']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-pct']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-pct']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-pct']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-pct']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-bar']} */ ;
/** @type {__VLS_StyleScopedClasses['gantt-milestone']} */ ;
/** @type {__VLS_StyleScopedClasses['gsp-close']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "goals-studio" },
});
/** @type {__VLS_StyleScopedClasses['goals-studio']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "gs-sidebar" },
    ...{ style: ({ width: __VLS_ctx.sideW + 'px' }) },
});
/** @type {__VLS_StyleScopedClasses['gs-sidebar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sidebar-top" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-top']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "sidebar-title" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sidebar-acts" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-acts']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    size: "small",
    circle: true,
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    size: "small",
    circle: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (__VLS_ctx.loadGoals) });
const { default: __VLS_7 } = __VLS_3.slots;
let __VLS_8;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent1(__VLS_8, new __VLS_8({}));
const __VLS_10 = __VLS_9({}, ...__VLS_functionalComponentArgsRest(__VLS_9));
const { default: __VLS_13 } = __VLS_11.slots;
let __VLS_14;
/** @ts-ignore @type { | typeof __VLS_components.Refresh} */
Refresh;
// @ts-ignore
const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({}));
const __VLS_16 = __VLS_15({}, ...__VLS_functionalComponentArgsRest(__VLS_15));
// @ts-ignore
[sideW, loadGoals,];
var __VLS_11;
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    circle: true,
}));
const __VLS_21 = __VLS_20({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    circle: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
let __VLS_24;
const __VLS_25 = ({ click: {} },
    { onClick: (__VLS_ctx.openCreate) });
const { default: __VLS_26 } = __VLS_22.slots;
let __VLS_27;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_28 = __VLS_asFunctionalComponent1(__VLS_27, new __VLS_27({}));
const __VLS_29 = __VLS_28({}, ...__VLS_functionalComponentArgsRest(__VLS_28));
const { default: __VLS_32 } = __VLS_30.slots;
let __VLS_33;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_34 = __VLS_asFunctionalComponent1(__VLS_33, new __VLS_33({}));
const __VLS_35 = __VLS_34({}, ...__VLS_functionalComponentArgsRest(__VLS_34));
// @ts-ignore
[openCreate,];
var __VLS_30;
// @ts-ignore
[];
var __VLS_22;
var __VLS_23;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "gs-filter" },
});
/** @type {__VLS_StyleScopedClasses['gs-filter']} */ ;
let __VLS_38;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_39 = __VLS_asFunctionalComponent1(__VLS_38, new __VLS_38({
    modelValue: (__VLS_ctx.filterStatus),
    placeholder: "所有状态",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}));
const __VLS_40 = __VLS_39({
    modelValue: (__VLS_ctx.filterStatus),
    placeholder: "所有状态",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}, ...__VLS_functionalComponentArgsRest(__VLS_39));
/** @type {__VLS_StyleScopedClasses['filter-sel']} */ ;
const { default: __VLS_43 } = __VLS_41.slots;
let __VLS_44;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({
    label: "草稿",
    value: "draft",
}));
const __VLS_46 = __VLS_45({
    label: "草稿",
    value: "draft",
}, ...__VLS_functionalComponentArgsRest(__VLS_45));
let __VLS_49;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_50 = __VLS_asFunctionalComponent1(__VLS_49, new __VLS_49({
    label: "进行中",
    value: "active",
}));
const __VLS_51 = __VLS_50({
    label: "进行中",
    value: "active",
}, ...__VLS_functionalComponentArgsRest(__VLS_50));
let __VLS_54;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_55 = __VLS_asFunctionalComponent1(__VLS_54, new __VLS_54({
    label: "已完成",
    value: "completed",
}));
const __VLS_56 = __VLS_55({
    label: "已完成",
    value: "completed",
}, ...__VLS_functionalComponentArgsRest(__VLS_55));
let __VLS_59;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_60 = __VLS_asFunctionalComponent1(__VLS_59, new __VLS_59({
    label: "已取消",
    value: "cancelled",
}));
const __VLS_61 = __VLS_60({
    label: "已取消",
    value: "cancelled",
}, ...__VLS_functionalComponentArgsRest(__VLS_60));
// @ts-ignore
[filterStatus,];
var __VLS_41;
let __VLS_64;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_65 = __VLS_asFunctionalComponent1(__VLS_64, new __VLS_64({
    modelValue: (__VLS_ctx.filterAgentId),
    placeholder: "所有成员",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}));
const __VLS_66 = __VLS_65({
    modelValue: (__VLS_ctx.filterAgentId),
    placeholder: "所有成员",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}, ...__VLS_functionalComponentArgsRest(__VLS_65));
/** @type {__VLS_StyleScopedClasses['filter-sel']} */ ;
const { default: __VLS_69 } = __VLS_67.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    let __VLS_70;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_71 = __VLS_asFunctionalComponent1(__VLS_70, new __VLS_70({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_72 = __VLS_71({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_71));
    // @ts-ignore
    [filterAgentId, agentList,];
}
// @ts-ignore
[];
var __VLS_67;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "goal-list" },
});
/** @type {__VLS_StyleScopedClasses['goal-list']} */ ;
if (__VLS_ctx.creating) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "goal-item goal-item-new active" },
    });
    /** @type {__VLS_StyleScopedClasses['goal-item']} */ ;
    /** @type {__VLS_StyleScopedClasses['goal-item-new']} */ ;
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gi-top" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-top']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "gi-title gi-title-new" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-title']} */ ;
    /** @type {__VLS_StyleScopedClasses['gi-title-new']} */ ;
    (__VLS_ctx.form.title.trim() || '新目标…');
    let __VLS_75;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_76 = __VLS_asFunctionalComponent1(__VLS_75, new __VLS_75({
        type: "info",
        size: "small",
        effect: "plain",
    }));
    const __VLS_77 = __VLS_76({
        type: "info",
        size: "small",
        effect: "plain",
    }, ...__VLS_functionalComponentArgsRest(__VLS_76));
    const { default: __VLS_80 } = __VLS_78.slots;
    // @ts-ignore
    [creating, form,];
    var __VLS_78;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gi-new-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-new-hint']} */ ;
}
if (__VLS_ctx.filteredGoals.length === 0 && !__VLS_ctx.creating) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "list-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['list-empty']} */ ;
}
for (const [g] of __VLS_vFor((__VLS_ctx.filteredGoals))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.selectGoal(g);
                // @ts-ignore
                [creating, filteredGoals, filteredGoals, selectGoal,];
            } },
        key: (g.id),
        ...{ class: (['goal-item', { active: __VLS_ctx.selectedGoal?.id === g.id }]) },
    });
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    /** @type {__VLS_StyleScopedClasses['goal-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gi-top" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-top']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "gi-title" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-title']} */ ;
    (g.title);
    let __VLS_81;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_82 = __VLS_asFunctionalComponent1(__VLS_81, new __VLS_81({
        type: (__VLS_ctx.statusTagType(g.status)),
        size: "small",
        effect: "plain",
    }));
    const __VLS_83 = __VLS_82({
        type: (__VLS_ctx.statusTagType(g.status)),
        size: "small",
        effect: "plain",
    }, ...__VLS_functionalComponentArgsRest(__VLS_82));
    const { default: __VLS_86 } = __VLS_84.slots;
    (__VLS_ctx.statusLabel(g.status));
    // @ts-ignore
    [selectedGoal, statusTagType, statusLabel,];
    var __VLS_84;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gi-progress-wrap" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-progress-wrap']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
        ...{ class: "gi-progress-bar" },
        ...{ style: ({ width: g.progress + '%', background: __VLS_ctx.progressColor(g) }) },
    });
    /** @type {__VLS_StyleScopedClasses['gi-progress-bar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "gi-progress-num" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-progress-num']} */ ;
    (g.progress);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gi-bottom" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-bottom']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gi-avatars" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-avatars']} */ ;
    for (const [id] of __VLS_vFor(((g.agentIds || []).slice(0, 3)))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (id),
            ...{ class: "gi-avatar" },
            ...{ style: ({ background: __VLS_ctx.agentColorMap[id] || '#6366f1' }) },
        });
        /** @type {__VLS_StyleScopedClasses['gi-avatar']} */ ;
        ((__VLS_ctx.agentNameMap[id] || id)[0]);
        // @ts-ignore
        [progressColor, agentColorMap, agentNameMap,];
    }
    if ((g.agentIds || []).length > 3) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "gi-avatar-more" },
        });
        /** @type {__VLS_StyleScopedClasses['gi-avatar-more']} */ ;
        (g.agentIds.length - 3);
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "gi-dates" },
    });
    /** @type {__VLS_StyleScopedClasses['gi-dates']} */ ;
    (__VLS_ctx.formatDate(g.startAt));
    (__VLS_ctx.formatDate(g.endAt));
    // @ts-ignore
    [formatDate, formatDate,];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.startResize($event, 'side');
            // @ts-ignore
            [startResize,];
        } },
    ...{ class: "gs-handle" },
    ...{ class: ({ dragging: __VLS_ctx.dragging === 'side' }) },
});
/** @type {__VLS_StyleScopedClasses['gs-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "gs-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['gs-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "gs-editor" },
});
/** @type {__VLS_StyleScopedClasses['gs-editor']} */ ;
if (!__VLS_ctx.selectedGoal && !__VLS_ctx.creating) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-gantt-overview" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-gantt-overview']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "overview-header" },
    });
    /** @type {__VLS_StyleScopedClasses['overview-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "overview-title" },
    });
    /** @type {__VLS_StyleScopedClasses['overview-title']} */ ;
    let __VLS_87;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_88 = __VLS_asFunctionalComponent1(__VLS_87, new __VLS_87({
        ...{ style: {} },
    }));
    const __VLS_89 = __VLS_88({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_88));
    const { default: __VLS_92 } = __VLS_90.slots;
    let __VLS_93;
    /** @ts-ignore @type { | typeof __VLS_components.Flag} */
    Flag;
    // @ts-ignore
    const __VLS_94 = __VLS_asFunctionalComponent1(__VLS_93, new __VLS_93({}));
    const __VLS_95 = __VLS_94({}, ...__VLS_functionalComponentArgsRest(__VLS_94));
    // @ts-ignore
    [creating, selectedGoal, dragging,];
    var __VLS_90;
    let __VLS_98;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_99 = __VLS_asFunctionalComponent1(__VLS_98, new __VLS_98({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }));
    const __VLS_100 = __VLS_99({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_99));
    let __VLS_103;
    const __VLS_104 = ({ click: {} },
        { onClick: (__VLS_ctx.openCreate) });
    const { default: __VLS_105 } = __VLS_101.slots;
    let __VLS_106;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_107 = __VLS_asFunctionalComponent1(__VLS_106, new __VLS_106({}));
    const __VLS_108 = __VLS_107({}, ...__VLS_functionalComponentArgsRest(__VLS_107));
    const { default: __VLS_111 } = __VLS_109.slots;
    let __VLS_112;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_113 = __VLS_asFunctionalComponent1(__VLS_112, new __VLS_112({}));
    const __VLS_114 = __VLS_113({}, ...__VLS_functionalComponentArgsRest(__VLS_113));
    // @ts-ignore
    [openCreate,];
    var __VLS_109;
    // @ts-ignore
    [];
    var __VLS_101;
    var __VLS_102;
    if (__VLS_ctx.filteredGoals.length === 0) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-empty" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-empty']} */ ;
        let __VLS_117;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_118 = __VLS_asFunctionalComponent1(__VLS_117, new __VLS_117({
            size: "48",
            color: "#c0c4cc",
        }));
        const __VLS_119 = __VLS_118({
            size: "48",
            color: "#c0c4cc",
        }, ...__VLS_functionalComponentArgsRest(__VLS_118));
        const { default: __VLS_122 } = __VLS_120.slots;
        let __VLS_123;
        /** @ts-ignore @type { | typeof __VLS_components.Flag} */
        Flag;
        // @ts-ignore
        const __VLS_124 = __VLS_asFunctionalComponent1(__VLS_123, new __VLS_123({}));
        const __VLS_125 = __VLS_124({}, ...__VLS_functionalComponentArgsRest(__VLS_124));
        // @ts-ignore
        [filteredGoals,];
        var __VLS_120;
        __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onWheel: (__VLS_ctx.handleGanttWheel) },
            ...{ onMousedown: (__VLS_ctx.onGanttMouseDown) },
            ...{ onMousemove: (__VLS_ctx.onGanttMouseMove) },
            ...{ onMouseup: (__VLS_ctx.onGanttMouseUp) },
            ...{ onMouseleave: (__VLS_ctx.onGanttMouseUp) },
            ...{ class: "gantt-wrap" },
            ...{ class: ({ 'is-dragging': __VLS_ctx.ganttDragging }) },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-wrap']} */ ;
        /** @type {__VLS_StyleScopedClasses['is-dragging']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-header" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-header']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-label-col" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-label-col']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "gantt-scale-hint" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-scale-hint']} */ ;
        (__VLS_ctx.tickStep.label);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-timeline-col" },
            ref: "ganttTimelineRef",
        });
        /** @type {__VLS_StyleScopedClasses['gantt-timeline-col']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-years" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-years']} */ ;
        for (const [t] of __VLS_vFor((__VLS_ctx.labelTicks.filter(t => t.yearMark)))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: ('yr-' + t.ts),
                ...{ class: "gantt-year-label" },
                ...{ style: ({ left: t.left }) },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-year-label']} */ ;
            (t.yearMark);
            // @ts-ignore
            [handleGanttWheel, onGanttMouseDown, onGanttMouseMove, onGanttMouseUp, onGanttMouseUp, ganttDragging, tickStep, labelTicks,];
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-months" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-months']} */ ;
        for (const [t] of __VLS_vFor((__VLS_ctx.labelTicks))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: ('lbl-' + t.ts),
                ...{ class: "gantt-month-label" },
                ...{ style: ({ left: t.left }) },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-month-label']} */ ;
            (t.label);
            // @ts-ignore
            [labelTicks,];
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-body" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-body']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-overlay" },
            'aria-hidden': "true",
        });
        /** @type {__VLS_StyleScopedClasses['gantt-overlay']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
            ...{ class: "gantt-label-col" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-label-col']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-timeline-col" },
            ...{ style: {} },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-timeline-col']} */ ;
        for (const [m] of __VLS_vFor((__VLS_ctx.monthLabels))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
                key: ('gv-' + m.ts),
                ...{ class: "gantt-grid-line" },
                ...{ style: ({ left: m.left }) },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-grid-line']} */ ;
            // @ts-ignore
            [monthLabels,];
        }
        if (__VLS_ctx.todayLeft !== null) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
                ...{ class: "gantt-today-line" },
                ...{ style: ({ left: __VLS_ctx.todayLeft }) },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-today-line']} */ ;
        }
        for (const [g] of __VLS_vFor((__VLS_ctx.filteredGoals))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ onClick: (...[$event]) => {
                        if (!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                            return;
                        if (!!(__VLS_ctx.filteredGoals.length === 0))
                            return;
                        __VLS_ctx.onGanttBarClick(g);
                        // @ts-ignore
                        [filteredGoals, todayLeft, todayLeft, onGanttBarClick,];
                    } },
                key: (g.id),
                ...{ class: "gantt-row" },
                ...{ class: ({ 'is-selected': __VLS_ctx.ganttSelectedGoal?.id === g.id }) },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-row']} */ ;
            /** @type {__VLS_StyleScopedClasses['is-selected']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "gantt-label-col" },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-label-col']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "gantt-label-inner" },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-label-inner']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "gantt-agent-avatars" },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-agent-avatars']} */ ;
            for (const [id] of __VLS_vFor(((g.agentIds || []).slice(0, 2)))) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    key: (id),
                    ...{ class: "gantt-avatar" },
                    ...{ style: ({ background: __VLS_ctx.agentColorMap[id] || '#409eff' }) },
                });
                /** @type {__VLS_StyleScopedClasses['gantt-avatar']} */ ;
                ((__VLS_ctx.agentNameMap[id] || id).slice(0, 1));
                // @ts-ignore
                [agentColorMap, agentNameMap, ganttSelectedGoal,];
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "gantt-goal-name" },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-goal-name']} */ ;
            (g.title);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "gantt-pct" },
                ...{ class: ('s-' + g.status) },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-pct']} */ ;
            (__VLS_ctx.timeProgress(g));
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "gantt-timeline-col" },
            });
            /** @type {__VLS_StyleScopedClasses['gantt-timeline-col']} */ ;
            if (__VLS_ctx.isValidBar(g)) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "gantt-bar" },
                    ...{ style: (__VLS_ctx.ganttBarStyle(g)) },
                });
                /** @type {__VLS_StyleScopedClasses['gantt-bar']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
                    ...{ class: "gantt-bar-progress" },
                    ...{ style: ({ width: __VLS_ctx.timeProgress(g) + '%' }) },
                });
                /** @type {__VLS_StyleScopedClasses['gantt-bar-progress']} */ ;
                if (__VLS_ctx.calcBarWidth(g) > 8) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "gantt-bar-label" },
                    });
                    /** @type {__VLS_StyleScopedClasses['gantt-bar-label']} */ ;
                    (g.title);
                }
                for (const [ms] of __VLS_vFor(((g.milestones || [])))) {
                    (ms.id);
                    if (__VLS_ctx.isValidDate(ms.dueAt)) {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
                            ...{ class: "gantt-milestone" },
                            ...{ class: ({ done: ms.done }) },
                            ...{ style: ({ left: __VLS_ctx.milestoneLeft(ms) }) },
                            title: (ms.title),
                        });
                        /** @type {__VLS_StyleScopedClasses['gantt-milestone']} */ ;
                        /** @type {__VLS_StyleScopedClasses['done']} */ ;
                    }
                    // @ts-ignore
                    [timeProgress, timeProgress, isValidBar, ganttBarStyle, calcBarWidth, isValidDate, milestoneLeft,];
                }
            }
            else {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "gantt-no-date" },
                });
                /** @type {__VLS_StyleScopedClasses['gantt-no-date']} */ ;
            }
            // @ts-ignore
            [];
        }
    }
    let __VLS_128;
    /** @ts-ignore @type { | typeof __VLS_components.transition | typeof __VLS_components.Transition | typeof __VLS_components.transition | typeof __VLS_components.Transition} */
    transition;
    // @ts-ignore
    const __VLS_129 = __VLS_asFunctionalComponent1(__VLS_128, new __VLS_128({
        name: "gsp-slide",
    }));
    const __VLS_130 = __VLS_129({
        name: "gsp-slide",
    }, ...__VLS_functionalComponentArgsRest(__VLS_129));
    const { default: __VLS_133 } = __VLS_131.slots;
    if (__VLS_ctx.ganttSelectedGoal) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gantt-summary-panel" },
        });
        /** @type {__VLS_StyleScopedClasses['gantt-summary-panel']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gsp-header" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-header']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gsp-avatars" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-avatars']} */ ;
        for (const [id] of __VLS_vFor(((__VLS_ctx.ganttSelectedGoal.agentIds || []).slice(0, 3)))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: (id),
                ...{ class: "gsp-avatar" },
                ...{ style: ({ background: __VLS_ctx.agentColorMap[id] || '#409eff' }) },
            });
            /** @type {__VLS_StyleScopedClasses['gsp-avatar']} */ ;
            ((__VLS_ctx.agentNameMap[id] || id).slice(0, 1));
            // @ts-ignore
            [agentColorMap, agentNameMap, ganttSelectedGoal, ganttSelectedGoal,];
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gsp-title-block" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-title-block']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "gsp-title" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-title']} */ ;
        (__VLS_ctx.ganttSelectedGoal.title);
        let __VLS_134;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_135 = __VLS_asFunctionalComponent1(__VLS_134, new __VLS_134({
            type: (__VLS_ctx.statusTagType(__VLS_ctx.ganttSelectedGoal.status)),
            size: "small",
        }));
        const __VLS_136 = __VLS_135({
            type: (__VLS_ctx.statusTagType(__VLS_ctx.ganttSelectedGoal.status)),
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_135));
        const { default: __VLS_139 } = __VLS_137.slots;
        (__VLS_ctx.statusLabel(__VLS_ctx.ganttSelectedGoal.status));
        // @ts-ignore
        [statusTagType, statusLabel, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal,];
        var __VLS_137;
        let __VLS_140;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_141 = __VLS_asFunctionalComponent1(__VLS_140, new __VLS_140({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
        }));
        const __VLS_142 = __VLS_141({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_141));
        let __VLS_145;
        const __VLS_146 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                        return;
                    if (!(__VLS_ctx.ganttSelectedGoal))
                        return;
                    __VLS_ctx.selectGoal(__VLS_ctx.ganttSelectedGoal);
                    __VLS_ctx.ganttSelectedGoal = null;
                    // @ts-ignore
                    [selectGoal, ganttSelectedGoal, ganttSelectedGoal,];
                } });
        const { default: __VLS_147 } = __VLS_143.slots;
        // @ts-ignore
        [];
        var __VLS_143;
        var __VLS_144;
        let __VLS_148;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_149 = __VLS_asFunctionalComponent1(__VLS_148, new __VLS_148({
            ...{ 'onClick': {} },
            ...{ class: "gsp-close" },
        }));
        const __VLS_150 = __VLS_149({
            ...{ 'onClick': {} },
            ...{ class: "gsp-close" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_149));
        let __VLS_153;
        const __VLS_154 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                        return;
                    if (!(__VLS_ctx.ganttSelectedGoal))
                        return;
                    __VLS_ctx.ganttSelectedGoal = null;
                    // @ts-ignore
                    [ganttSelectedGoal,];
                } });
        /** @type {__VLS_StyleScopedClasses['gsp-close']} */ ;
        const { default: __VLS_155 } = __VLS_151.slots;
        let __VLS_156;
        /** @ts-ignore @type { | typeof __VLS_components.Close} */
        Close;
        // @ts-ignore
        const __VLS_157 = __VLS_asFunctionalComponent1(__VLS_156, new __VLS_156({}));
        const __VLS_158 = __VLS_157({}, ...__VLS_functionalComponentArgsRest(__VLS_157));
        // @ts-ignore
        [];
        var __VLS_151;
        var __VLS_152;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gsp-body" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-body']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gsp-stat" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-stat']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "gsp-stat-label" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-stat-label']} */ ;
        let __VLS_161;
        /** @ts-ignore @type { | typeof __VLS_components.elProgress | typeof __VLS_components.ElProgress | typeof __VLS_components['el-progress']} */
        elProgress;
        // @ts-ignore
        const __VLS_162 = __VLS_asFunctionalComponent1(__VLS_161, new __VLS_161({
            percentage: (__VLS_ctx.timeProgress(__VLS_ctx.ganttSelectedGoal)),
            color: (__VLS_ctx.progressColor(__VLS_ctx.ganttSelectedGoal)),
            strokeWidth: (8),
            ...{ style: {} },
        }));
        const __VLS_163 = __VLS_162({
            percentage: (__VLS_ctx.timeProgress(__VLS_ctx.ganttSelectedGoal)),
            color: (__VLS_ctx.progressColor(__VLS_ctx.ganttSelectedGoal)),
            strokeWidth: (8),
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_162));
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "gsp-stat-val" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-stat-val']} */ ;
        (__VLS_ctx.timeProgress(__VLS_ctx.ganttSelectedGoal));
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gsp-stat" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-stat']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "gsp-stat-label" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-stat-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "gsp-stat-val" },
        });
        /** @type {__VLS_StyleScopedClasses['gsp-stat-val']} */ ;
        (__VLS_ctx.formatDate(__VLS_ctx.ganttSelectedGoal.startAt));
        (__VLS_ctx.formatDate(__VLS_ctx.ganttSelectedGoal.endAt));
        if (__VLS_ctx.ganttSelectedGoal.milestones?.length) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "gsp-stat" },
            });
            /** @type {__VLS_StyleScopedClasses['gsp-stat']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "gsp-stat-label" },
            });
            /** @type {__VLS_StyleScopedClasses['gsp-stat-label']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "gsp-stat-val" },
            });
            /** @type {__VLS_StyleScopedClasses['gsp-stat-val']} */ ;
            (__VLS_ctx.ganttSelectedGoal.milestones.filter(m => m.done).length);
            (__VLS_ctx.ganttSelectedGoal.milestones.length);
        }
        if (__VLS_ctx.ganttSelectedGoal.description) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "gsp-desc" },
            });
            /** @type {__VLS_StyleScopedClasses['gsp-desc']} */ ;
            (__VLS_ctx.ganttSelectedGoal.description);
        }
    }
    // @ts-ignore
    [progressColor, formatDate, formatDate, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, ganttSelectedGoal, timeProgress, timeProgress,];
    var __VLS_131;
}
else if (__VLS_ctx.creating) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-toolbar" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-toolbar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-breadcrumb" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-breadcrumb']} */ ;
    let __VLS_166;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_167 = __VLS_asFunctionalComponent1(__VLS_166, new __VLS_166({
        ...{ style: {} },
    }));
    const __VLS_168 = __VLS_167({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_167));
    const { default: __VLS_171 } = __VLS_169.slots;
    let __VLS_172;
    /** @ts-ignore @type { | typeof __VLS_components.Flag} */
    Flag;
    // @ts-ignore
    const __VLS_173 = __VLS_asFunctionalComponent1(__VLS_172, new __VLS_172({}));
    const __VLS_174 = __VLS_173({}, ...__VLS_functionalComponentArgsRest(__VLS_173));
    // @ts-ignore
    [creating,];
    var __VLS_169;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-sep" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-sep']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-name" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-name']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "toolbar-acts" },
    });
    /** @type {__VLS_StyleScopedClasses['toolbar-acts']} */ ;
    let __VLS_177;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_178 = __VLS_asFunctionalComponent1(__VLS_177, new __VLS_177({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_179 = __VLS_178({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_178));
    let __VLS_182;
    const __VLS_183 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                    return;
                if (!(__VLS_ctx.creating))
                    return;
                __VLS_ctx.creating = false;
                // @ts-ignore
                [creating,];
            } });
    const { default: __VLS_184 } = __VLS_180.slots;
    // @ts-ignore
    [];
    var __VLS_180;
    var __VLS_181;
    let __VLS_185;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_186 = __VLS_asFunctionalComponent1(__VLS_185, new __VLS_185({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.saving),
    }));
    const __VLS_187 = __VLS_186({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.saving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_186));
    let __VLS_190;
    const __VLS_191 = ({ click: {} },
        { onClick: (__VLS_ctx.saveGoal) });
    const { default: __VLS_192 } = __VLS_188.slots;
    let __VLS_193;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_194 = __VLS_asFunctionalComponent1(__VLS_193, new __VLS_193({}));
    const __VLS_195 = __VLS_194({}, ...__VLS_functionalComponentArgsRest(__VLS_194));
    const { default: __VLS_198 } = __VLS_196.slots;
    let __VLS_199;
    /** @ts-ignore @type { | typeof __VLS_components.DocumentChecked} */
    DocumentChecked;
    // @ts-ignore
    const __VLS_200 = __VLS_asFunctionalComponent1(__VLS_199, new __VLS_199({}));
    const __VLS_201 = __VLS_200({}, ...__VLS_functionalComponentArgsRest(__VLS_200));
    // @ts-ignore
    [saving, saveGoal,];
    var __VLS_196;
    // @ts-ignore
    [];
    var __VLS_188;
    var __VLS_189;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-form" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-form']} */ ;
    let __VLS_204;
    /** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
    elForm;
    // @ts-ignore
    const __VLS_205 = __VLS_asFunctionalComponent1(__VLS_204, new __VLS_204({
        model: (__VLS_ctx.form),
        labelWidth: "90px",
        size: "small",
        ...{ class: "goal-inner-form" },
    }));
    const __VLS_206 = __VLS_205({
        model: (__VLS_ctx.form),
        labelWidth: "90px",
        size: "small",
        ...{ class: "goal-inner-form" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_205));
    /** @type {__VLS_StyleScopedClasses['goal-inner-form']} */ ;
    const { default: __VLS_209 } = __VLS_207.slots;
    let __VLS_210;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_211 = __VLS_asFunctionalComponent1(__VLS_210, new __VLS_210({
        label: "标题",
        required: true,
    }));
    const __VLS_212 = __VLS_211({
        label: "标题",
        required: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_211));
    const { default: __VLS_215 } = __VLS_213.slots;
    let __VLS_216;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_217 = __VLS_asFunctionalComponent1(__VLS_216, new __VLS_216({
        modelValue: (__VLS_ctx.form.title),
        placeholder: "目标标题",
    }));
    const __VLS_218 = __VLS_217({
        modelValue: (__VLS_ctx.form.title),
        placeholder: "目标标题",
    }, ...__VLS_functionalComponentArgsRest(__VLS_217));
    // @ts-ignore
    [form, form,];
    var __VLS_213;
    let __VLS_221;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_222 = __VLS_asFunctionalComponent1(__VLS_221, new __VLS_221({
        label: "描述",
    }));
    const __VLS_223 = __VLS_222({
        label: "描述",
    }, ...__VLS_functionalComponentArgsRest(__VLS_222));
    const { default: __VLS_226 } = __VLS_224.slots;
    let __VLS_227;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_228 = __VLS_asFunctionalComponent1(__VLS_227, new __VLS_227({
        modelValue: (__VLS_ctx.form.description),
        type: "textarea",
        rows: (2),
        placeholder: "（可选）",
    }));
    const __VLS_229 = __VLS_228({
        modelValue: (__VLS_ctx.form.description),
        type: "textarea",
        rows: (2),
        placeholder: "（可选）",
    }, ...__VLS_functionalComponentArgsRest(__VLS_228));
    // @ts-ignore
    [form,];
    var __VLS_224;
    let __VLS_232;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_233 = __VLS_asFunctionalComponent1(__VLS_232, new __VLS_232({
        label: "类型",
    }));
    const __VLS_234 = __VLS_233({
        label: "类型",
    }, ...__VLS_functionalComponentArgsRest(__VLS_233));
    const { default: __VLS_237 } = __VLS_235.slots;
    let __VLS_238;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
    elRadioGroup;
    // @ts-ignore
    const __VLS_239 = __VLS_asFunctionalComponent1(__VLS_238, new __VLS_238({
        modelValue: (__VLS_ctx.form.type),
    }));
    const __VLS_240 = __VLS_239({
        modelValue: (__VLS_ctx.form.type),
    }, ...__VLS_functionalComponentArgsRest(__VLS_239));
    const { default: __VLS_243 } = __VLS_241.slots;
    let __VLS_244;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_245 = __VLS_asFunctionalComponent1(__VLS_244, new __VLS_244({
        value: "personal",
    }));
    const __VLS_246 = __VLS_245({
        value: "personal",
    }, ...__VLS_functionalComponentArgsRest(__VLS_245));
    const { default: __VLS_249 } = __VLS_247.slots;
    // @ts-ignore
    [form,];
    var __VLS_247;
    let __VLS_250;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_251 = __VLS_asFunctionalComponent1(__VLS_250, new __VLS_250({
        value: "team",
    }));
    const __VLS_252 = __VLS_251({
        value: "team",
    }, ...__VLS_functionalComponentArgsRest(__VLS_251));
    const { default: __VLS_255 } = __VLS_253.slots;
    // @ts-ignore
    [];
    var __VLS_253;
    // @ts-ignore
    [];
    var __VLS_241;
    // @ts-ignore
    [];
    var __VLS_235;
    let __VLS_256;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_257 = __VLS_asFunctionalComponent1(__VLS_256, new __VLS_256({
        label: "参与成员",
    }));
    const __VLS_258 = __VLS_257({
        label: "参与成员",
    }, ...__VLS_functionalComponentArgsRest(__VLS_257));
    const { default: __VLS_261 } = __VLS_259.slots;
    let __VLS_262;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_263 = __VLS_asFunctionalComponent1(__VLS_262, new __VLS_262({
        modelValue: (__VLS_ctx.form.agentIds),
        multiple: true,
        placeholder: "选择成员",
        ...{ style: {} },
    }));
    const __VLS_264 = __VLS_263({
        modelValue: (__VLS_ctx.form.agentIds),
        multiple: true,
        placeholder: "选择成员",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_263));
    const { default: __VLS_267 } = __VLS_265.slots;
    for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
        let __VLS_268;
        /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
        elOption;
        // @ts-ignore
        const __VLS_269 = __VLS_asFunctionalComponent1(__VLS_268, new __VLS_268({
            key: (ag.id),
            label: (ag.name),
            value: (ag.id),
        }));
        const __VLS_270 = __VLS_269({
            key: (ag.id),
            label: (ag.name),
            value: (ag.id),
        }, ...__VLS_functionalComponentArgsRest(__VLS_269));
        // @ts-ignore
        [agentList, form,];
    }
    // @ts-ignore
    [];
    var __VLS_265;
    // @ts-ignore
    [];
    var __VLS_259;
    let __VLS_273;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_274 = __VLS_asFunctionalComponent1(__VLS_273, new __VLS_273({
        label: "状态",
    }));
    const __VLS_275 = __VLS_274({
        label: "状态",
    }, ...__VLS_functionalComponentArgsRest(__VLS_274));
    const { default: __VLS_278 } = __VLS_276.slots;
    let __VLS_279;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_280 = __VLS_asFunctionalComponent1(__VLS_279, new __VLS_279({
        modelValue: (__VLS_ctx.form.status),
        ...{ style: {} },
    }));
    const __VLS_281 = __VLS_280({
        modelValue: (__VLS_ctx.form.status),
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_280));
    const { default: __VLS_284 } = __VLS_282.slots;
    let __VLS_285;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_286 = __VLS_asFunctionalComponent1(__VLS_285, new __VLS_285({
        label: "草稿",
        value: "draft",
    }));
    const __VLS_287 = __VLS_286({
        label: "草稿",
        value: "draft",
    }, ...__VLS_functionalComponentArgsRest(__VLS_286));
    let __VLS_290;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_291 = __VLS_asFunctionalComponent1(__VLS_290, new __VLS_290({
        label: "进行中",
        value: "active",
    }));
    const __VLS_292 = __VLS_291({
        label: "进行中",
        value: "active",
    }, ...__VLS_functionalComponentArgsRest(__VLS_291));
    let __VLS_295;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_296 = __VLS_asFunctionalComponent1(__VLS_295, new __VLS_295({
        label: "已完成",
        value: "completed",
    }));
    const __VLS_297 = __VLS_296({
        label: "已完成",
        value: "completed",
    }, ...__VLS_functionalComponentArgsRest(__VLS_296));
    let __VLS_300;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_301 = __VLS_asFunctionalComponent1(__VLS_300, new __VLS_300({
        label: "已取消",
        value: "cancelled",
    }));
    const __VLS_302 = __VLS_301({
        label: "已取消",
        value: "cancelled",
    }, ...__VLS_functionalComponentArgsRest(__VLS_301));
    // @ts-ignore
    [form,];
    var __VLS_282;
    // @ts-ignore
    [];
    var __VLS_276;
    let __VLS_305;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_306 = __VLS_asFunctionalComponent1(__VLS_305, new __VLS_305({
        label: "开始时间",
    }));
    const __VLS_307 = __VLS_306({
        label: "开始时间",
    }, ...__VLS_functionalComponentArgsRest(__VLS_306));
    const { default: __VLS_310 } = __VLS_308.slots;
    let __VLS_311;
    /** @ts-ignore @type { | typeof __VLS_components.elDatePicker | typeof __VLS_components.ElDatePicker | typeof __VLS_components['el-date-picker']} */
    elDatePicker;
    // @ts-ignore
    const __VLS_312 = __VLS_asFunctionalComponent1(__VLS_311, new __VLS_311({
        modelValue: (__VLS_ctx.form.startAt),
        type: "datetime",
        placeholder: "选择开始时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }));
    const __VLS_313 = __VLS_312({
        modelValue: (__VLS_ctx.form.startAt),
        type: "datetime",
        placeholder: "选择开始时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }, ...__VLS_functionalComponentArgsRest(__VLS_312));
    // @ts-ignore
    [form,];
    var __VLS_308;
    let __VLS_316;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_317 = __VLS_asFunctionalComponent1(__VLS_316, new __VLS_316({
        label: "结束时间",
    }));
    const __VLS_318 = __VLS_317({
        label: "结束时间",
    }, ...__VLS_functionalComponentArgsRest(__VLS_317));
    const { default: __VLS_321 } = __VLS_319.slots;
    let __VLS_322;
    /** @ts-ignore @type { | typeof __VLS_components.elDatePicker | typeof __VLS_components.ElDatePicker | typeof __VLS_components['el-date-picker']} */
    elDatePicker;
    // @ts-ignore
    const __VLS_323 = __VLS_asFunctionalComponent1(__VLS_322, new __VLS_322({
        modelValue: (__VLS_ctx.form.endAt),
        type: "datetime",
        placeholder: "选择结束时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }));
    const __VLS_324 = __VLS_323({
        modelValue: (__VLS_ctx.form.endAt),
        type: "datetime",
        placeholder: "选择结束时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }, ...__VLS_functionalComponentArgsRest(__VLS_323));
    // @ts-ignore
    [form,];
    var __VLS_319;
    if (__VLS_ctx.selectedGoal) {
        let __VLS_327;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_328 = __VLS_asFunctionalComponent1(__VLS_327, new __VLS_327({
            label: "进度",
        }));
        const __VLS_329 = __VLS_328({
            label: "进度",
        }, ...__VLS_functionalComponentArgsRest(__VLS_328));
        const { default: __VLS_332 } = __VLS_330.slots;
        let __VLS_333;
        /** @ts-ignore @type { | typeof __VLS_components.elSlider | typeof __VLS_components.ElSlider | typeof __VLS_components['el-slider']} */
        elSlider;
        // @ts-ignore
        const __VLS_334 = __VLS_asFunctionalComponent1(__VLS_333, new __VLS_333({
            modelValue: (__VLS_ctx.form.progress),
            min: (0),
            max: (100),
            showInput: true,
        }));
        const __VLS_335 = __VLS_334({
            modelValue: (__VLS_ctx.form.progress),
            min: (0),
            max: (100),
            showInput: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_334));
        // @ts-ignore
        [form, selectedGoal,];
        var __VLS_330;
    }
    let __VLS_338;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_339 = __VLS_asFunctionalComponent1(__VLS_338, new __VLS_338({
        label: "里程碑",
    }));
    const __VLS_340 = __VLS_339({
        label: "里程碑",
    }, ...__VLS_functionalComponentArgsRest(__VLS_339));
    const { default: __VLS_343 } = __VLS_341.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    for (const [ms, idx] of __VLS_vFor((__VLS_ctx.form.milestones))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (idx),
            ...{ class: "milestone-row" },
        });
        /** @type {__VLS_StyleScopedClasses['milestone-row']} */ ;
        let __VLS_344;
        /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
        elCheckbox;
        // @ts-ignore
        const __VLS_345 = __VLS_asFunctionalComponent1(__VLS_344, new __VLS_344({
            modelValue: (ms.done),
        }));
        const __VLS_346 = __VLS_345({
            modelValue: (ms.done),
        }, ...__VLS_functionalComponentArgsRest(__VLS_345));
        let __VLS_349;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_350 = __VLS_asFunctionalComponent1(__VLS_349, new __VLS_349({
            modelValue: (ms.title),
            placeholder: "里程碑标题",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_351 = __VLS_350({
            modelValue: (ms.title),
            placeholder: "里程碑标题",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_350));
        let __VLS_354;
        /** @ts-ignore @type { | typeof __VLS_components.elDatePicker | typeof __VLS_components.ElDatePicker | typeof __VLS_components['el-date-picker']} */
        elDatePicker;
        // @ts-ignore
        const __VLS_355 = __VLS_asFunctionalComponent1(__VLS_354, new __VLS_354({
            modelValue: (ms.dueAt),
            type: "date",
            placeholder: "截止日",
            size: "small",
            ...{ style: {} },
            valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
        }));
        const __VLS_356 = __VLS_355({
            modelValue: (ms.dueAt),
            type: "date",
            placeholder: "截止日",
            size: "small",
            ...{ style: {} },
            valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
        }, ...__VLS_functionalComponentArgsRest(__VLS_355));
        let __VLS_359;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_360 = __VLS_asFunctionalComponent1(__VLS_359, new __VLS_359({
            ...{ 'onClick': {} },
            type: "danger",
            size: "small",
            circle: true,
        }));
        const __VLS_361 = __VLS_360({
            ...{ 'onClick': {} },
            type: "danger",
            size: "small",
            circle: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_360));
        let __VLS_364;
        const __VLS_365 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                        return;
                    if (!(__VLS_ctx.creating))
                        return;
                    __VLS_ctx.form.milestones.splice(idx, 1);
                    // @ts-ignore
                    [form, form,];
                } });
        const { default: __VLS_366 } = __VLS_362.slots;
        let __VLS_367;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_368 = __VLS_asFunctionalComponent1(__VLS_367, new __VLS_367({}));
        const __VLS_369 = __VLS_368({}, ...__VLS_functionalComponentArgsRest(__VLS_368));
        const { default: __VLS_372 } = __VLS_370.slots;
        let __VLS_373;
        /** @ts-ignore @type { | typeof __VLS_components.Delete} */
        Delete;
        // @ts-ignore
        const __VLS_374 = __VLS_asFunctionalComponent1(__VLS_373, new __VLS_373({}));
        const __VLS_375 = __VLS_374({}, ...__VLS_functionalComponentArgsRest(__VLS_374));
        // @ts-ignore
        [];
        var __VLS_370;
        // @ts-ignore
        [];
        var __VLS_362;
        var __VLS_363;
        // @ts-ignore
        [];
    }
    let __VLS_378;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_379 = __VLS_asFunctionalComponent1(__VLS_378, new __VLS_378({
        ...{ 'onClick': {} },
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_380 = __VLS_379({
        ...{ 'onClick': {} },
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_379));
    let __VLS_383;
    const __VLS_384 = ({ click: {} },
        { onClick: (__VLS_ctx.addMilestone) });
    const { default: __VLS_385 } = __VLS_381.slots;
    let __VLS_386;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_387 = __VLS_asFunctionalComponent1(__VLS_386, new __VLS_386({}));
    const __VLS_388 = __VLS_387({}, ...__VLS_functionalComponentArgsRest(__VLS_387));
    const { default: __VLS_391 } = __VLS_389.slots;
    let __VLS_392;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_393 = __VLS_asFunctionalComponent1(__VLS_392, new __VLS_392({}));
    const __VLS_394 = __VLS_393({}, ...__VLS_functionalComponentArgsRest(__VLS_393));
    // @ts-ignore
    [addMilestone,];
    var __VLS_389;
    // @ts-ignore
    [];
    var __VLS_381;
    var __VLS_382;
    // @ts-ignore
    [];
    var __VLS_341;
    // @ts-ignore
    [];
    var __VLS_207;
}
else if (__VLS_ctx.selectedGoal) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-toolbar" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-toolbar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-breadcrumb" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-breadcrumb']} */ ;
    let __VLS_397;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_398 = __VLS_asFunctionalComponent1(__VLS_397, new __VLS_397({
        ...{ 'onClick': {} },
        size: "small",
        text: true,
        ...{ class: "crumb-back" },
        title: "返回甘特图",
    }));
    const __VLS_399 = __VLS_398({
        ...{ 'onClick': {} },
        size: "small",
        text: true,
        ...{ class: "crumb-back" },
        title: "返回甘特图",
    }, ...__VLS_functionalComponentArgsRest(__VLS_398));
    let __VLS_402;
    const __VLS_403 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                    return;
                if (!!(__VLS_ctx.creating))
                    return;
                if (!(__VLS_ctx.selectedGoal))
                    return;
                __VLS_ctx.selectedGoal = null;
                // @ts-ignore
                [selectedGoal, selectedGoal,];
            } });
    /** @type {__VLS_StyleScopedClasses['crumb-back']} */ ;
    const { default: __VLS_404 } = __VLS_400.slots;
    let __VLS_405;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_406 = __VLS_asFunctionalComponent1(__VLS_405, new __VLS_405({}));
    const __VLS_407 = __VLS_406({}, ...__VLS_functionalComponentArgsRest(__VLS_406));
    const { default: __VLS_410 } = __VLS_408.slots;
    let __VLS_411;
    /** @ts-ignore @type { | typeof __VLS_components.ArrowLeft} */
    ArrowLeft;
    // @ts-ignore
    const __VLS_412 = __VLS_asFunctionalComponent1(__VLS_411, new __VLS_411({}));
    const __VLS_413 = __VLS_412({}, ...__VLS_functionalComponentArgsRest(__VLS_412));
    // @ts-ignore
    [];
    var __VLS_408;
    // @ts-ignore
    [];
    var __VLS_400;
    var __VLS_401;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-sep" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-sep']} */ ;
    let __VLS_416;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_417 = __VLS_asFunctionalComponent1(__VLS_416, new __VLS_416({
        ...{ style: {} },
    }));
    const __VLS_418 = __VLS_417({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_417));
    const { default: __VLS_421 } = __VLS_419.slots;
    let __VLS_422;
    /** @ts-ignore @type { | typeof __VLS_components.Flag} */
    Flag;
    // @ts-ignore
    const __VLS_423 = __VLS_asFunctionalComponent1(__VLS_422, new __VLS_422({}));
    const __VLS_424 = __VLS_423({}, ...__VLS_functionalComponentArgsRest(__VLS_423));
    // @ts-ignore
    [];
    var __VLS_419;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-name" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-name']} */ ;
    (__VLS_ctx.selectedGoal.title);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "toolbar-acts" },
    });
    /** @type {__VLS_StyleScopedClasses['toolbar-acts']} */ ;
    let __VLS_427;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_428 = __VLS_asFunctionalComponent1(__VLS_427, new __VLS_427({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.saving),
    }));
    const __VLS_429 = __VLS_428({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.saving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_428));
    let __VLS_432;
    const __VLS_433 = ({ click: {} },
        { onClick: (__VLS_ctx.saveGoal) });
    const { default: __VLS_434 } = __VLS_430.slots;
    let __VLS_435;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_436 = __VLS_asFunctionalComponent1(__VLS_435, new __VLS_435({}));
    const __VLS_437 = __VLS_436({}, ...__VLS_functionalComponentArgsRest(__VLS_436));
    const { default: __VLS_440 } = __VLS_438.slots;
    let __VLS_441;
    /** @ts-ignore @type { | typeof __VLS_components.DocumentChecked} */
    DocumentChecked;
    // @ts-ignore
    const __VLS_442 = __VLS_asFunctionalComponent1(__VLS_441, new __VLS_441({}));
    const __VLS_443 = __VLS_442({}, ...__VLS_functionalComponentArgsRest(__VLS_442));
    // @ts-ignore
    [selectedGoal, saving, saveGoal,];
    var __VLS_438;
    // @ts-ignore
    [];
    var __VLS_430;
    var __VLS_431;
    let __VLS_446;
    /** @ts-ignore @type { | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm'] | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm']} */
    elPopconfirm;
    // @ts-ignore
    const __VLS_447 = __VLS_asFunctionalComponent1(__VLS_446, new __VLS_446({
        ...{ 'onConfirm': {} },
        title: (`确认删除「${__VLS_ctx.selectedGoal.title}」？`),
    }));
    const __VLS_448 = __VLS_447({
        ...{ 'onConfirm': {} },
        title: (`确认删除「${__VLS_ctx.selectedGoal.title}」？`),
    }, ...__VLS_functionalComponentArgsRest(__VLS_447));
    let __VLS_451;
    const __VLS_452 = ({ confirm: {} },
        { onConfirm: (__VLS_ctx.deleteGoal) });
    const { default: __VLS_453 } = __VLS_449.slots;
    {
        const { reference: __VLS_454 } = __VLS_449.slots;
        let __VLS_455;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_456 = __VLS_asFunctionalComponent1(__VLS_455, new __VLS_455({
            size: "small",
            type: "danger",
            plain: true,
        }));
        const __VLS_457 = __VLS_456({
            size: "small",
            type: "danger",
            plain: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_456));
        const { default: __VLS_460 } = __VLS_458.slots;
        let __VLS_461;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_462 = __VLS_asFunctionalComponent1(__VLS_461, new __VLS_461({}));
        const __VLS_463 = __VLS_462({}, ...__VLS_functionalComponentArgsRest(__VLS_462));
        const { default: __VLS_466 } = __VLS_464.slots;
        let __VLS_467;
        /** @ts-ignore @type { | typeof __VLS_components.Delete} */
        Delete;
        // @ts-ignore
        const __VLS_468 = __VLS_asFunctionalComponent1(__VLS_467, new __VLS_467({}));
        const __VLS_469 = __VLS_468({}, ...__VLS_functionalComponentArgsRest(__VLS_468));
        // @ts-ignore
        [selectedGoal, deleteGoal,];
        var __VLS_464;
        // @ts-ignore
        [];
        var __VLS_458;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_449;
    var __VLS_450;
    let __VLS_472;
    /** @ts-ignore @type { | typeof __VLS_components.elTabs | typeof __VLS_components.ElTabs | typeof __VLS_components['el-tabs'] | typeof __VLS_components.elTabs | typeof __VLS_components.ElTabs | typeof __VLS_components['el-tabs']} */
    elTabs;
    // @ts-ignore
    const __VLS_473 = __VLS_asFunctionalComponent1(__VLS_472, new __VLS_472({
        modelValue: (__VLS_ctx.editorTab),
        ...{ class: "editor-tabs" },
    }));
    const __VLS_474 = __VLS_473({
        modelValue: (__VLS_ctx.editorTab),
        ...{ class: "editor-tabs" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_473));
    /** @type {__VLS_StyleScopedClasses['editor-tabs']} */ ;
    const { default: __VLS_477 } = __VLS_475.slots;
    let __VLS_478;
    /** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
    elTabPane;
    // @ts-ignore
    const __VLS_479 = __VLS_asFunctionalComponent1(__VLS_478, new __VLS_478({
        label: "基本信息",
        name: "basic",
    }));
    const __VLS_480 = __VLS_479({
        label: "基本信息",
        name: "basic",
    }, ...__VLS_functionalComponentArgsRest(__VLS_479));
    const { default: __VLS_483 } = __VLS_481.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-form" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-form']} */ ;
    let __VLS_484;
    /** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
    elForm;
    // @ts-ignore
    const __VLS_485 = __VLS_asFunctionalComponent1(__VLS_484, new __VLS_484({
        model: (__VLS_ctx.form),
        labelWidth: "90px",
        size: "small",
        ...{ class: "goal-inner-form" },
    }));
    const __VLS_486 = __VLS_485({
        model: (__VLS_ctx.form),
        labelWidth: "90px",
        size: "small",
        ...{ class: "goal-inner-form" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_485));
    /** @type {__VLS_StyleScopedClasses['goal-inner-form']} */ ;
    const { default: __VLS_489 } = __VLS_487.slots;
    let __VLS_490;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_491 = __VLS_asFunctionalComponent1(__VLS_490, new __VLS_490({
        label: "标题",
        required: true,
    }));
    const __VLS_492 = __VLS_491({
        label: "标题",
        required: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_491));
    const { default: __VLS_495 } = __VLS_493.slots;
    let __VLS_496;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_497 = __VLS_asFunctionalComponent1(__VLS_496, new __VLS_496({
        modelValue: (__VLS_ctx.form.title),
        placeholder: "目标标题",
    }));
    const __VLS_498 = __VLS_497({
        modelValue: (__VLS_ctx.form.title),
        placeholder: "目标标题",
    }, ...__VLS_functionalComponentArgsRest(__VLS_497));
    // @ts-ignore
    [form, form, editorTab,];
    var __VLS_493;
    let __VLS_501;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_502 = __VLS_asFunctionalComponent1(__VLS_501, new __VLS_501({
        label: "描述",
    }));
    const __VLS_503 = __VLS_502({
        label: "描述",
    }, ...__VLS_functionalComponentArgsRest(__VLS_502));
    const { default: __VLS_506 } = __VLS_504.slots;
    let __VLS_507;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_508 = __VLS_asFunctionalComponent1(__VLS_507, new __VLS_507({
        modelValue: (__VLS_ctx.form.description),
        type: "textarea",
        rows: (2),
        placeholder: "（可选）",
    }));
    const __VLS_509 = __VLS_508({
        modelValue: (__VLS_ctx.form.description),
        type: "textarea",
        rows: (2),
        placeholder: "（可选）",
    }, ...__VLS_functionalComponentArgsRest(__VLS_508));
    // @ts-ignore
    [form,];
    var __VLS_504;
    let __VLS_512;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_513 = __VLS_asFunctionalComponent1(__VLS_512, new __VLS_512({
        label: "类型",
    }));
    const __VLS_514 = __VLS_513({
        label: "类型",
    }, ...__VLS_functionalComponentArgsRest(__VLS_513));
    const { default: __VLS_517 } = __VLS_515.slots;
    let __VLS_518;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
    elRadioGroup;
    // @ts-ignore
    const __VLS_519 = __VLS_asFunctionalComponent1(__VLS_518, new __VLS_518({
        modelValue: (__VLS_ctx.form.type),
    }));
    const __VLS_520 = __VLS_519({
        modelValue: (__VLS_ctx.form.type),
    }, ...__VLS_functionalComponentArgsRest(__VLS_519));
    const { default: __VLS_523 } = __VLS_521.slots;
    let __VLS_524;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_525 = __VLS_asFunctionalComponent1(__VLS_524, new __VLS_524({
        value: "personal",
    }));
    const __VLS_526 = __VLS_525({
        value: "personal",
    }, ...__VLS_functionalComponentArgsRest(__VLS_525));
    const { default: __VLS_529 } = __VLS_527.slots;
    // @ts-ignore
    [form,];
    var __VLS_527;
    let __VLS_530;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_531 = __VLS_asFunctionalComponent1(__VLS_530, new __VLS_530({
        value: "team",
    }));
    const __VLS_532 = __VLS_531({
        value: "team",
    }, ...__VLS_functionalComponentArgsRest(__VLS_531));
    const { default: __VLS_535 } = __VLS_533.slots;
    // @ts-ignore
    [];
    var __VLS_533;
    // @ts-ignore
    [];
    var __VLS_521;
    // @ts-ignore
    [];
    var __VLS_515;
    let __VLS_536;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_537 = __VLS_asFunctionalComponent1(__VLS_536, new __VLS_536({
        label: "参与成员",
    }));
    const __VLS_538 = __VLS_537({
        label: "参与成员",
    }, ...__VLS_functionalComponentArgsRest(__VLS_537));
    const { default: __VLS_541 } = __VLS_539.slots;
    let __VLS_542;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_543 = __VLS_asFunctionalComponent1(__VLS_542, new __VLS_542({
        modelValue: (__VLS_ctx.form.agentIds),
        multiple: true,
        placeholder: "选择成员",
        ...{ style: {} },
    }));
    const __VLS_544 = __VLS_543({
        modelValue: (__VLS_ctx.form.agentIds),
        multiple: true,
        placeholder: "选择成员",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_543));
    const { default: __VLS_547 } = __VLS_545.slots;
    for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
        let __VLS_548;
        /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
        elOption;
        // @ts-ignore
        const __VLS_549 = __VLS_asFunctionalComponent1(__VLS_548, new __VLS_548({
            key: (ag.id),
            label: (ag.name),
            value: (ag.id),
        }));
        const __VLS_550 = __VLS_549({
            key: (ag.id),
            label: (ag.name),
            value: (ag.id),
        }, ...__VLS_functionalComponentArgsRest(__VLS_549));
        // @ts-ignore
        [agentList, form,];
    }
    // @ts-ignore
    [];
    var __VLS_545;
    // @ts-ignore
    [];
    var __VLS_539;
    let __VLS_553;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_554 = __VLS_asFunctionalComponent1(__VLS_553, new __VLS_553({
        label: "状态",
    }));
    const __VLS_555 = __VLS_554({
        label: "状态",
    }, ...__VLS_functionalComponentArgsRest(__VLS_554));
    const { default: __VLS_558 } = __VLS_556.slots;
    let __VLS_559;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_560 = __VLS_asFunctionalComponent1(__VLS_559, new __VLS_559({
        modelValue: (__VLS_ctx.form.status),
        ...{ style: {} },
    }));
    const __VLS_561 = __VLS_560({
        modelValue: (__VLS_ctx.form.status),
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_560));
    const { default: __VLS_564 } = __VLS_562.slots;
    let __VLS_565;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_566 = __VLS_asFunctionalComponent1(__VLS_565, new __VLS_565({
        label: "草稿",
        value: "draft",
    }));
    const __VLS_567 = __VLS_566({
        label: "草稿",
        value: "draft",
    }, ...__VLS_functionalComponentArgsRest(__VLS_566));
    let __VLS_570;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_571 = __VLS_asFunctionalComponent1(__VLS_570, new __VLS_570({
        label: "进行中",
        value: "active",
    }));
    const __VLS_572 = __VLS_571({
        label: "进行中",
        value: "active",
    }, ...__VLS_functionalComponentArgsRest(__VLS_571));
    let __VLS_575;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_576 = __VLS_asFunctionalComponent1(__VLS_575, new __VLS_575({
        label: "已完成",
        value: "completed",
    }));
    const __VLS_577 = __VLS_576({
        label: "已完成",
        value: "completed",
    }, ...__VLS_functionalComponentArgsRest(__VLS_576));
    let __VLS_580;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_581 = __VLS_asFunctionalComponent1(__VLS_580, new __VLS_580({
        label: "已取消",
        value: "cancelled",
    }));
    const __VLS_582 = __VLS_581({
        label: "已取消",
        value: "cancelled",
    }, ...__VLS_functionalComponentArgsRest(__VLS_581));
    // @ts-ignore
    [form,];
    var __VLS_562;
    // @ts-ignore
    [];
    var __VLS_556;
    let __VLS_585;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_586 = __VLS_asFunctionalComponent1(__VLS_585, new __VLS_585({
        label: "开始时间",
    }));
    const __VLS_587 = __VLS_586({
        label: "开始时间",
    }, ...__VLS_functionalComponentArgsRest(__VLS_586));
    const { default: __VLS_590 } = __VLS_588.slots;
    let __VLS_591;
    /** @ts-ignore @type { | typeof __VLS_components.elDatePicker | typeof __VLS_components.ElDatePicker | typeof __VLS_components['el-date-picker']} */
    elDatePicker;
    // @ts-ignore
    const __VLS_592 = __VLS_asFunctionalComponent1(__VLS_591, new __VLS_591({
        modelValue: (__VLS_ctx.form.startAt),
        type: "datetime",
        placeholder: "选择开始时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }));
    const __VLS_593 = __VLS_592({
        modelValue: (__VLS_ctx.form.startAt),
        type: "datetime",
        placeholder: "选择开始时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }, ...__VLS_functionalComponentArgsRest(__VLS_592));
    // @ts-ignore
    [form,];
    var __VLS_588;
    let __VLS_596;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_597 = __VLS_asFunctionalComponent1(__VLS_596, new __VLS_596({
        label: "结束时间",
    }));
    const __VLS_598 = __VLS_597({
        label: "结束时间",
    }, ...__VLS_functionalComponentArgsRest(__VLS_597));
    const { default: __VLS_601 } = __VLS_599.slots;
    let __VLS_602;
    /** @ts-ignore @type { | typeof __VLS_components.elDatePicker | typeof __VLS_components.ElDatePicker | typeof __VLS_components['el-date-picker']} */
    elDatePicker;
    // @ts-ignore
    const __VLS_603 = __VLS_asFunctionalComponent1(__VLS_602, new __VLS_602({
        modelValue: (__VLS_ctx.form.endAt),
        type: "datetime",
        placeholder: "选择结束时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }));
    const __VLS_604 = __VLS_603({
        modelValue: (__VLS_ctx.form.endAt),
        type: "datetime",
        placeholder: "选择结束时间",
        ...{ style: {} },
        valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
    }, ...__VLS_functionalComponentArgsRest(__VLS_603));
    // @ts-ignore
    [form,];
    var __VLS_599;
    if (__VLS_ctx.selectedGoal) {
        let __VLS_607;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_608 = __VLS_asFunctionalComponent1(__VLS_607, new __VLS_607({
            label: "进度",
        }));
        const __VLS_609 = __VLS_608({
            label: "进度",
        }, ...__VLS_functionalComponentArgsRest(__VLS_608));
        const { default: __VLS_612 } = __VLS_610.slots;
        let __VLS_613;
        /** @ts-ignore @type { | typeof __VLS_components.elSlider | typeof __VLS_components.ElSlider | typeof __VLS_components['el-slider']} */
        elSlider;
        // @ts-ignore
        const __VLS_614 = __VLS_asFunctionalComponent1(__VLS_613, new __VLS_613({
            modelValue: (__VLS_ctx.form.progress),
            min: (0),
            max: (100),
            showInput: true,
        }));
        const __VLS_615 = __VLS_614({
            modelValue: (__VLS_ctx.form.progress),
            min: (0),
            max: (100),
            showInput: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_614));
        // @ts-ignore
        [form, selectedGoal,];
        var __VLS_610;
    }
    let __VLS_618;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_619 = __VLS_asFunctionalComponent1(__VLS_618, new __VLS_618({
        label: "里程碑",
    }));
    const __VLS_620 = __VLS_619({
        label: "里程碑",
    }, ...__VLS_functionalComponentArgsRest(__VLS_619));
    const { default: __VLS_623 } = __VLS_621.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    for (const [ms, idx] of __VLS_vFor((__VLS_ctx.form.milestones))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (idx),
            ...{ class: "milestone-row" },
        });
        /** @type {__VLS_StyleScopedClasses['milestone-row']} */ ;
        let __VLS_624;
        /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
        elCheckbox;
        // @ts-ignore
        const __VLS_625 = __VLS_asFunctionalComponent1(__VLS_624, new __VLS_624({
            modelValue: (ms.done),
        }));
        const __VLS_626 = __VLS_625({
            modelValue: (ms.done),
        }, ...__VLS_functionalComponentArgsRest(__VLS_625));
        let __VLS_629;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_630 = __VLS_asFunctionalComponent1(__VLS_629, new __VLS_629({
            modelValue: (ms.title),
            placeholder: "里程碑标题",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_631 = __VLS_630({
            modelValue: (ms.title),
            placeholder: "里程碑标题",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_630));
        let __VLS_634;
        /** @ts-ignore @type { | typeof __VLS_components.elDatePicker | typeof __VLS_components.ElDatePicker | typeof __VLS_components['el-date-picker']} */
        elDatePicker;
        // @ts-ignore
        const __VLS_635 = __VLS_asFunctionalComponent1(__VLS_634, new __VLS_634({
            modelValue: (ms.dueAt),
            type: "date",
            placeholder: "截止日",
            size: "small",
            ...{ style: {} },
            valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
        }));
        const __VLS_636 = __VLS_635({
            modelValue: (ms.dueAt),
            type: "date",
            placeholder: "截止日",
            size: "small",
            ...{ style: {} },
            valueFormat: "YYYY-MM-DDTHH:mm:ssZ",
        }, ...__VLS_functionalComponentArgsRest(__VLS_635));
        let __VLS_639;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_640 = __VLS_asFunctionalComponent1(__VLS_639, new __VLS_639({
            ...{ 'onClick': {} },
            type: "danger",
            size: "small",
            circle: true,
        }));
        const __VLS_641 = __VLS_640({
            ...{ 'onClick': {} },
            type: "danger",
            size: "small",
            circle: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_640));
        let __VLS_644;
        const __VLS_645 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                        return;
                    if (!!(__VLS_ctx.creating))
                        return;
                    if (!(__VLS_ctx.selectedGoal))
                        return;
                    __VLS_ctx.form.milestones.splice(idx, 1);
                    // @ts-ignore
                    [form, form,];
                } });
        const { default: __VLS_646 } = __VLS_642.slots;
        let __VLS_647;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_648 = __VLS_asFunctionalComponent1(__VLS_647, new __VLS_647({}));
        const __VLS_649 = __VLS_648({}, ...__VLS_functionalComponentArgsRest(__VLS_648));
        const { default: __VLS_652 } = __VLS_650.slots;
        let __VLS_653;
        /** @ts-ignore @type { | typeof __VLS_components.Delete} */
        Delete;
        // @ts-ignore
        const __VLS_654 = __VLS_asFunctionalComponent1(__VLS_653, new __VLS_653({}));
        const __VLS_655 = __VLS_654({}, ...__VLS_functionalComponentArgsRest(__VLS_654));
        // @ts-ignore
        [];
        var __VLS_650;
        // @ts-ignore
        [];
        var __VLS_642;
        var __VLS_643;
        // @ts-ignore
        [];
    }
    let __VLS_658;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_659 = __VLS_asFunctionalComponent1(__VLS_658, new __VLS_658({
        ...{ 'onClick': {} },
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_660 = __VLS_659({
        ...{ 'onClick': {} },
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_659));
    let __VLS_663;
    const __VLS_664 = ({ click: {} },
        { onClick: (__VLS_ctx.addMilestone) });
    const { default: __VLS_665 } = __VLS_661.slots;
    let __VLS_666;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_667 = __VLS_asFunctionalComponent1(__VLS_666, new __VLS_666({}));
    const __VLS_668 = __VLS_667({}, ...__VLS_functionalComponentArgsRest(__VLS_667));
    const { default: __VLS_671 } = __VLS_669.slots;
    let __VLS_672;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_673 = __VLS_asFunctionalComponent1(__VLS_672, new __VLS_672({}));
    const __VLS_674 = __VLS_673({}, ...__VLS_functionalComponentArgsRest(__VLS_673));
    // @ts-ignore
    [addMilestone,];
    var __VLS_669;
    // @ts-ignore
    [];
    var __VLS_661;
    var __VLS_662;
    // @ts-ignore
    [];
    var __VLS_621;
    // @ts-ignore
    [];
    var __VLS_487;
    // @ts-ignore
    [];
    var __VLS_481;
    let __VLS_677;
    /** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
    elTabPane;
    // @ts-ignore
    const __VLS_678 = __VLS_asFunctionalComponent1(__VLS_677, new __VLS_677({
        label: "定期检查",
        name: "checks",
    }));
    const __VLS_679 = __VLS_678({
        label: "定期检查",
        name: "checks",
    }, ...__VLS_functionalComponentArgsRest(__VLS_678));
    const { default: __VLS_682 } = __VLS_680.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "tab-panel" },
    });
    /** @type {__VLS_StyleScopedClasses['tab-panel']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "tab-panel-head" },
    });
    /** @type {__VLS_StyleScopedClasses['tab-panel-head']} */ ;
    let __VLS_683;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_684 = __VLS_asFunctionalComponent1(__VLS_683, new __VLS_683({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }));
    const __VLS_685 = __VLS_684({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_684));
    let __VLS_688;
    const __VLS_689 = ({ click: {} },
        { onClick: (__VLS_ctx.openAddCheckDialog) });
    const { default: __VLS_690 } = __VLS_686.slots;
    let __VLS_691;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_692 = __VLS_asFunctionalComponent1(__VLS_691, new __VLS_691({}));
    const __VLS_693 = __VLS_692({}, ...__VLS_functionalComponentArgsRest(__VLS_692));
    const { default: __VLS_696 } = __VLS_694.slots;
    let __VLS_697;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_698 = __VLS_asFunctionalComponent1(__VLS_697, new __VLS_697({}));
    const __VLS_699 = __VLS_698({}, ...__VLS_functionalComponentArgsRest(__VLS_698));
    // @ts-ignore
    [openAddCheckDialog,];
    var __VLS_694;
    // @ts-ignore
    [];
    var __VLS_686;
    var __VLS_687;
    let __VLS_702;
    /** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
    elTable;
    // @ts-ignore
    const __VLS_703 = __VLS_asFunctionalComponent1(__VLS_702, new __VLS_702({
        data: (__VLS_ctx.selectedGoal.checks),
        size: "small",
        stripe: true,
        ...{ class: "checks-table" },
    }));
    const __VLS_704 = __VLS_703({
        data: (__VLS_ctx.selectedGoal.checks),
        size: "small",
        stripe: true,
        ...{ class: "checks-table" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_703));
    /** @type {__VLS_StyleScopedClasses['checks-table']} */ ;
    const { default: __VLS_707 } = __VLS_705.slots;
    let __VLS_708;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_709 = __VLS_asFunctionalComponent1(__VLS_708, new __VLS_708({
        prop: "name",
        label: "名称",
        minWidth: "120",
    }));
    const __VLS_710 = __VLS_709({
        prop: "name",
        label: "名称",
        minWidth: "120",
    }, ...__VLS_functionalComponentArgsRest(__VLS_709));
    let __VLS_713;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_714 = __VLS_asFunctionalComponent1(__VLS_713, new __VLS_713({
        label: "频率",
        minWidth: "140",
    }));
    const __VLS_715 = __VLS_714({
        label: "频率",
        minWidth: "140",
    }, ...__VLS_functionalComponentArgsRest(__VLS_714));
    const { default: __VLS_718 } = __VLS_716.slots;
    {
        const { default: __VLS_719 } = __VLS_716.slots;
        const [{ row }] = __VLS_vSlot(__VLS_719);
        __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
            ...{ class: "code-cell" },
        });
        /** @type {__VLS_StyleScopedClasses['code-cell']} */ ;
        (row.schedule);
        let __VLS_720;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_721 = __VLS_asFunctionalComponent1(__VLS_720, new __VLS_720({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_722 = __VLS_721({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_721));
        const { default: __VLS_725 } = __VLS_723.slots;
        (row.tz || 'Asia/Shanghai');
        // @ts-ignore
        [selectedGoal,];
        var __VLS_723;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_716;
    let __VLS_726;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_727 = __VLS_asFunctionalComponent1(__VLS_726, new __VLS_726({
        label: "成员",
        width: "90",
    }));
    const __VLS_728 = __VLS_727({
        label: "成员",
        width: "90",
    }, ...__VLS_functionalComponentArgsRest(__VLS_727));
    const { default: __VLS_731 } = __VLS_729.slots;
    {
        const { default: __VLS_732 } = __VLS_729.slots;
        const [{ row }] = __VLS_vSlot(__VLS_732);
        (__VLS_ctx.agentNameMap[row.agentId] || row.agentId);
        // @ts-ignore
        [agentNameMap,];
    }
    // @ts-ignore
    [];
    var __VLS_729;
    let __VLS_733;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_734 = __VLS_asFunctionalComponent1(__VLS_733, new __VLS_733({
        label: "启用",
        width: "60",
    }));
    const __VLS_735 = __VLS_734({
        label: "启用",
        width: "60",
    }, ...__VLS_functionalComponentArgsRest(__VLS_734));
    const { default: __VLS_738 } = __VLS_736.slots;
    {
        const { default: __VLS_739 } = __VLS_736.slots;
        const [{ row }] = __VLS_vSlot(__VLS_739);
        let __VLS_740;
        /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
        elSwitch;
        // @ts-ignore
        const __VLS_741 = __VLS_asFunctionalComponent1(__VLS_740, new __VLS_740({
            ...{ 'onChange': {} },
            modelValue: (row.enabled),
            size: "small",
        }));
        const __VLS_742 = __VLS_741({
            ...{ 'onChange': {} },
            modelValue: (row.enabled),
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_741));
        let __VLS_745;
        const __VLS_746 = ({ change: {} },
            { onChange: (...[$event]) => {
                    if (!!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                        return;
                    if (!!(__VLS_ctx.creating))
                        return;
                    if (!(__VLS_ctx.selectedGoal))
                        return;
                    __VLS_ctx.toggleCheck(row);
                    // @ts-ignore
                    [toggleCheck,];
                } });
        var __VLS_743;
        var __VLS_744;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_736;
    let __VLS_747;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_748 = __VLS_asFunctionalComponent1(__VLS_747, new __VLS_747({
        label: "操作",
        width: "130",
    }));
    const __VLS_749 = __VLS_748({
        label: "操作",
        width: "130",
    }, ...__VLS_functionalComponentArgsRest(__VLS_748));
    const { default: __VLS_752 } = __VLS_750.slots;
    {
        const { default: __VLS_753 } = __VLS_750.slots;
        const [{ row }] = __VLS_vSlot(__VLS_753);
        let __VLS_754;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_755 = __VLS_asFunctionalComponent1(__VLS_754, new __VLS_754({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
        }));
        const __VLS_756 = __VLS_755({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_755));
        let __VLS_759;
        const __VLS_760 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                        return;
                    if (!!(__VLS_ctx.creating))
                        return;
                    if (!(__VLS_ctx.selectedGoal))
                        return;
                    __VLS_ctx.runCheckNow(row);
                    // @ts-ignore
                    [runCheckNow,];
                } });
        const { default: __VLS_761 } = __VLS_757.slots;
        // @ts-ignore
        [];
        var __VLS_757;
        var __VLS_758;
        let __VLS_762;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_763 = __VLS_asFunctionalComponent1(__VLS_762, new __VLS_762({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "danger",
        }));
        const __VLS_764 = __VLS_763({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "danger",
        }, ...__VLS_functionalComponentArgsRest(__VLS_763));
        let __VLS_767;
        const __VLS_768 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selectedGoal && !__VLS_ctx.creating))
                        return;
                    if (!!(__VLS_ctx.creating))
                        return;
                    if (!(__VLS_ctx.selectedGoal))
                        return;
                    __VLS_ctx.removeCheck(row);
                    // @ts-ignore
                    [removeCheck,];
                } });
        const { default: __VLS_769 } = __VLS_765.slots;
        // @ts-ignore
        [];
        var __VLS_765;
        var __VLS_766;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_750;
    // @ts-ignore
    [];
    var __VLS_705;
    if (!__VLS_ctx.selectedGoal.checks?.length) {
        let __VLS_770;
        /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
        elEmpty;
        // @ts-ignore
        const __VLS_771 = __VLS_asFunctionalComponent1(__VLS_770, new __VLS_770({
            description: "暂无检查计划",
            imageSize: (60),
        }));
        const __VLS_772 = __VLS_771({
            description: "暂无检查计划",
            imageSize: (60),
        }, ...__VLS_functionalComponentArgsRest(__VLS_771));
    }
    // @ts-ignore
    [selectedGoal,];
    var __VLS_680;
    let __VLS_775;
    /** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
    elTabPane;
    // @ts-ignore
    const __VLS_776 = __VLS_asFunctionalComponent1(__VLS_775, new __VLS_775({
        label: "检查记录",
        name: "records",
    }));
    const __VLS_777 = __VLS_776({
        label: "检查记录",
        name: "records",
    }, ...__VLS_functionalComponentArgsRest(__VLS_776));
    const { default: __VLS_780 } = __VLS_778.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "tab-panel" },
    });
    /** @type {__VLS_StyleScopedClasses['tab-panel']} */ ;
    if (__VLS_ctx.checkRecordsLoading) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_781;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_782 = __VLS_asFunctionalComponent1(__VLS_781, new __VLS_781({
            ...{ class: "is-loading" },
            size: "24",
        }));
        const __VLS_783 = __VLS_782({
            ...{ class: "is-loading" },
            size: "24",
        }, ...__VLS_functionalComponentArgsRest(__VLS_782));
        /** @type {__VLS_StyleScopedClasses['is-loading']} */ ;
        const { default: __VLS_786 } = __VLS_784.slots;
        let __VLS_787;
        /** @ts-ignore @type { | typeof __VLS_components.Loading} */
        Loading;
        // @ts-ignore
        const __VLS_788 = __VLS_asFunctionalComponent1(__VLS_787, new __VLS_787({}));
        const __VLS_789 = __VLS_788({}, ...__VLS_functionalComponentArgsRest(__VLS_788));
        // @ts-ignore
        [checkRecordsLoading,];
        var __VLS_784;
    }
    else if (__VLS_ctx.checkRecords.length) {
        let __VLS_792;
        /** @ts-ignore @type { | typeof __VLS_components.elTimeline | typeof __VLS_components.ElTimeline | typeof __VLS_components['el-timeline'] | typeof __VLS_components.elTimeline | typeof __VLS_components.ElTimeline | typeof __VLS_components['el-timeline']} */
        elTimeline;
        // @ts-ignore
        const __VLS_793 = __VLS_asFunctionalComponent1(__VLS_792, new __VLS_792({
            ...{ class: "records-timeline" },
        }));
        const __VLS_794 = __VLS_793({
            ...{ class: "records-timeline" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_793));
        /** @type {__VLS_StyleScopedClasses['records-timeline']} */ ;
        const { default: __VLS_797 } = __VLS_795.slots;
        for (const [rec] of __VLS_vFor((__VLS_ctx.checkRecords))) {
            let __VLS_798;
            /** @ts-ignore @type { | typeof __VLS_components.elTimelineItem | typeof __VLS_components.ElTimelineItem | typeof __VLS_components['el-timeline-item'] | typeof __VLS_components.elTimelineItem | typeof __VLS_components.ElTimelineItem | typeof __VLS_components['el-timeline-item']} */
            elTimelineItem;
            // @ts-ignore
            const __VLS_799 = __VLS_asFunctionalComponent1(__VLS_798, new __VLS_798({
                key: (rec.id),
                timestamp: (__VLS_ctx.formatDateTime(rec.runAt)),
                type: (rec.status === 'ok' ? 'success' : 'danger'),
                placement: "top",
            }));
            const __VLS_800 = __VLS_799({
                key: (rec.id),
                timestamp: (__VLS_ctx.formatDateTime(rec.runAt)),
                type: (rec.status === 'ok' ? 'success' : 'danger'),
                placement: "top",
            }, ...__VLS_functionalComponentArgsRest(__VLS_799));
            const { default: __VLS_803 } = __VLS_801.slots;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "record-card" },
            });
            /** @type {__VLS_StyleScopedClasses['record-card']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "record-header" },
            });
            /** @type {__VLS_StyleScopedClasses['record-header']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "rec-avatar" },
                ...{ style: ({ background: __VLS_ctx.agentColorMap[rec.agentId] || '#409eff' }) },
            });
            /** @type {__VLS_StyleScopedClasses['rec-avatar']} */ ;
            ((__VLS_ctx.agentNameMap[rec.agentId] || '?')[0]);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "rec-name" },
            });
            /** @type {__VLS_StyleScopedClasses['rec-name']} */ ;
            (__VLS_ctx.agentNameMap[rec.agentId] || rec.agentId);
            let __VLS_804;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_805 = __VLS_asFunctionalComponent1(__VLS_804, new __VLS_804({
                type: (rec.status === 'ok' ? 'success' : 'danger'),
                size: "small",
            }));
            const __VLS_806 = __VLS_805({
                type: (rec.status === 'ok' ? 'success' : 'danger'),
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_805));
            const { default: __VLS_809 } = __VLS_807.slots;
            (rec.status);
            // @ts-ignore
            [agentColorMap, agentNameMap, agentNameMap, checkRecords, checkRecords, formatDateTime,];
            var __VLS_807;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "rec-output" },
            });
            /** @type {__VLS_StyleScopedClasses['rec-output']} */ ;
            (rec.output || '（无输出）');
            // @ts-ignore
            [];
            var __VLS_801;
            // @ts-ignore
            [];
        }
        // @ts-ignore
        [];
        var __VLS_795;
    }
    else {
        let __VLS_810;
        /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
        elEmpty;
        // @ts-ignore
        const __VLS_811 = __VLS_asFunctionalComponent1(__VLS_810, new __VLS_810({
            description: "暂无检查记录",
            imageSize: (60),
        }));
        const __VLS_812 = __VLS_811({
            description: "暂无检查记录",
            imageSize: (60),
        }, ...__VLS_functionalComponentArgsRest(__VLS_811));
    }
    // @ts-ignore
    [];
    var __VLS_778;
    // @ts-ignore
    [];
    var __VLS_475;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.startResize($event, 'chat');
            // @ts-ignore
            [startResize,];
        } },
    ...{ class: "gs-handle" },
    ...{ class: ({ dragging: __VLS_ctx.dragging === 'chat' }) },
});
/** @type {__VLS_StyleScopedClasses['gs-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "gs-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['gs-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "gs-chat" },
    ...{ style: ({ width: __VLS_ctx.chatW + 'px' }) },
});
/** @type {__VLS_StyleScopedClasses['gs-chat']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-panel-head" },
});
/** @type {__VLS_StyleScopedClasses['chat-panel-head']} */ ;
let __VLS_815;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_816 = __VLS_asFunctionalComponent1(__VLS_815, new __VLS_815({}));
const __VLS_817 = __VLS_816({}, ...__VLS_functionalComponentArgsRest(__VLS_816));
const { default: __VLS_820 } = __VLS_818.slots;
let __VLS_821;
/** @ts-ignore @type { | typeof __VLS_components.ChatLineRound} */
ChatLineRound;
// @ts-ignore
const __VLS_822 = __VLS_asFunctionalComponent1(__VLS_821, new __VLS_821({}));
const __VLS_823 = __VLS_822({}, ...__VLS_functionalComponentArgsRest(__VLS_822));
// @ts-ignore
[dragging, chatW,];
var __VLS_818;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
let __VLS_826;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_827 = __VLS_asFunctionalComponent1(__VLS_826, new __VLS_826({
    modelValue: (__VLS_ctx.selectedChatAgentId),
    size: "small",
    ...{ style: {} },
    placeholder: "选择成员",
}));
const __VLS_828 = __VLS_827({
    modelValue: (__VLS_ctx.selectedChatAgentId),
    size: "small",
    ...{ style: {} },
    placeholder: "选择成员",
}, ...__VLS_functionalComponentArgsRest(__VLS_827));
const { default: __VLS_831 } = __VLS_829.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    let __VLS_832;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_833 = __VLS_asFunctionalComponent1(__VLS_832, new __VLS_832({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_834 = __VLS_833({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_833));
    // @ts-ignore
    [agentList, selectedChatAgentId,];
}
// @ts-ignore
[];
var __VLS_829;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-wrap" },
});
/** @type {__VLS_StyleScopedClasses['chat-wrap']} */ ;
if (__VLS_ctx.selectedChatAgentId) {
    const __VLS_837 = AiChat;
    // @ts-ignore
    const __VLS_838 = __VLS_asFunctionalComponent1(__VLS_837, new __VLS_837({
        ...{ 'onResponse': {} },
        key: (__VLS_ctx.goalChatSessionId),
        agentId: (__VLS_ctx.selectedChatAgentId),
        sessionId: (__VLS_ctx.goalChatSessionId),
        context: (__VLS_ctx.goalChatContext),
        welcomeMessage: (__VLS_ctx.selectedGoal
            ? `正在查看目标「${__VLS_ctx.selectedGoal.title}」，我可以帮你修改目标信息、添加里程碑或设置定期检查。`
            : '你好！我可以帮你创建目标，说一下需求即可，我会自动填写表单。'),
        examples: (__VLS_ctx.selectedGoal
            ? ['帮我把进度更新到 60%', '添加3个里程碑', '每周一检查这个目标']
            : ['帮我创建一个团队目标：Q2用户增长，3月1日到6月30日', '个人目标：学习 Go 语言，本月完成']),
        height: "100%",
    }));
    const __VLS_839 = __VLS_838({
        ...{ 'onResponse': {} },
        key: (__VLS_ctx.goalChatSessionId),
        agentId: (__VLS_ctx.selectedChatAgentId),
        sessionId: (__VLS_ctx.goalChatSessionId),
        context: (__VLS_ctx.goalChatContext),
        welcomeMessage: (__VLS_ctx.selectedGoal
            ? `正在查看目标「${__VLS_ctx.selectedGoal.title}」，我可以帮你修改目标信息、添加里程碑或设置定期检查。`
            : '你好！我可以帮你创建目标，说一下需求即可，我会自动填写表单。'),
        examples: (__VLS_ctx.selectedGoal
            ? ['帮我把进度更新到 60%', '添加3个里程碑', '每周一检查这个目标']
            : ['帮我创建一个团队目标：Q2用户增长，3月1日到6月30日', '个人目标：学习 Go 语言，本月完成']),
        height: "100%",
    }, ...__VLS_functionalComponentArgsRest(__VLS_838));
    let __VLS_842;
    const __VLS_843 = ({ response: {} },
        { onResponse: (__VLS_ctx.onAiResponse) });
    var __VLS_840;
    var __VLS_841;
}
let __VLS_844;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_845 = __VLS_asFunctionalComponent1(__VLS_844, new __VLS_844({
    modelValue: (__VLS_ctx.checkDialogVisible),
    title: "添加定期检查",
    width: "480px",
}));
const __VLS_846 = __VLS_845({
    modelValue: (__VLS_ctx.checkDialogVisible),
    title: "添加定期检查",
    width: "480px",
}, ...__VLS_functionalComponentArgsRest(__VLS_845));
const { default: __VLS_849 } = __VLS_847.slots;
let __VLS_850;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_851 = __VLS_asFunctionalComponent1(__VLS_850, new __VLS_850({
    model: (__VLS_ctx.checkForm),
    labelWidth: "90px",
    size: "small",
}));
const __VLS_852 = __VLS_851({
    model: (__VLS_ctx.checkForm),
    labelWidth: "90px",
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_851));
const { default: __VLS_855 } = __VLS_853.slots;
let __VLS_856;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_857 = __VLS_asFunctionalComponent1(__VLS_856, new __VLS_856({
    label: "名称",
    required: true,
}));
const __VLS_858 = __VLS_857({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_857));
const { default: __VLS_861 } = __VLS_859.slots;
let __VLS_862;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_863 = __VLS_asFunctionalComponent1(__VLS_862, new __VLS_862({
    modelValue: (__VLS_ctx.checkForm.name),
    placeholder: "如：每周进度检查",
}));
const __VLS_864 = __VLS_863({
    modelValue: (__VLS_ctx.checkForm.name),
    placeholder: "如：每周进度检查",
}, ...__VLS_functionalComponentArgsRest(__VLS_863));
// @ts-ignore
[selectedGoal, selectedGoal, selectedGoal, selectedChatAgentId, selectedChatAgentId, goalChatSessionId, goalChatSessionId, goalChatContext, onAiResponse, checkDialogVisible, checkForm, checkForm,];
var __VLS_859;
let __VLS_867;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_868 = __VLS_asFunctionalComponent1(__VLS_867, new __VLS_867({
    label: "执行成员",
    required: true,
}));
const __VLS_869 = __VLS_868({
    label: "执行成员",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_868));
const { default: __VLS_872 } = __VLS_870.slots;
let __VLS_873;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_874 = __VLS_asFunctionalComponent1(__VLS_873, new __VLS_873({
    modelValue: (__VLS_ctx.checkForm.agentId),
    ...{ style: {} },
}));
const __VLS_875 = __VLS_874({
    modelValue: (__VLS_ctx.checkForm.agentId),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_874));
const { default: __VLS_878 } = __VLS_876.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    let __VLS_879;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_880 = __VLS_asFunctionalComponent1(__VLS_879, new __VLS_879({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_881 = __VLS_880({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_880));
    // @ts-ignore
    [agentList, checkForm,];
}
// @ts-ignore
[];
var __VLS_876;
// @ts-ignore
[];
var __VLS_870;
let __VLS_884;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_885 = __VLS_asFunctionalComponent1(__VLS_884, new __VLS_884({
    label: "检查频率",
}));
const __VLS_886 = __VLS_885({
    label: "检查频率",
}, ...__VLS_functionalComponentArgsRest(__VLS_885));
const { default: __VLS_889 } = __VLS_887.slots;
let __VLS_890;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_891 = __VLS_asFunctionalComponent1(__VLS_890, new __VLS_890({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.checkFreqPreset),
    ...{ style: {} },
}));
const __VLS_892 = __VLS_891({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.checkFreqPreset),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_891));
let __VLS_895;
const __VLS_896 = ({ change: {} },
    { onChange: (__VLS_ctx.onPresetChange) });
const { default: __VLS_897 } = __VLS_893.slots;
let __VLS_898;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_899 = __VLS_asFunctionalComponent1(__VLS_898, new __VLS_898({
    label: "每天上午9点",
    value: "0 9 * * *",
}));
const __VLS_900 = __VLS_899({
    label: "每天上午9点",
    value: "0 9 * * *",
}, ...__VLS_functionalComponentArgsRest(__VLS_899));
let __VLS_903;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_904 = __VLS_asFunctionalComponent1(__VLS_903, new __VLS_903({
    label: "每周一上午9点",
    value: "0 9 * * 1",
}));
const __VLS_905 = __VLS_904({
    label: "每周一上午9点",
    value: "0 9 * * 1",
}, ...__VLS_functionalComponentArgsRest(__VLS_904));
let __VLS_908;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_909 = __VLS_asFunctionalComponent1(__VLS_908, new __VLS_908({
    label: "每周五下午5点",
    value: "0 17 * * 5",
}));
const __VLS_910 = __VLS_909({
    label: "每周五下午5点",
    value: "0 17 * * 5",
}, ...__VLS_functionalComponentArgsRest(__VLS_909));
let __VLS_913;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_914 = __VLS_asFunctionalComponent1(__VLS_913, new __VLS_913({
    label: "每月1日上午9点",
    value: "0 9 1 * *",
}));
const __VLS_915 = __VLS_914({
    label: "每月1日上午9点",
    value: "0 9 1 * *",
}, ...__VLS_functionalComponentArgsRest(__VLS_914));
let __VLS_918;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_919 = __VLS_asFunctionalComponent1(__VLS_918, new __VLS_918({
    label: "自定义",
    value: "custom",
}));
const __VLS_920 = __VLS_919({
    label: "自定义",
    value: "custom",
}, ...__VLS_functionalComponentArgsRest(__VLS_919));
// @ts-ignore
[checkFreqPreset, onPresetChange,];
var __VLS_893;
var __VLS_894;
// @ts-ignore
[];
var __VLS_887;
if (__VLS_ctx.checkFreqPreset === 'custom') {
    let __VLS_923;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_924 = __VLS_asFunctionalComponent1(__VLS_923, new __VLS_923({
        label: "Cron",
    }));
    const __VLS_925 = __VLS_924({
        label: "Cron",
    }, ...__VLS_functionalComponentArgsRest(__VLS_924));
    const { default: __VLS_928 } = __VLS_926.slots;
    let __VLS_929;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_930 = __VLS_asFunctionalComponent1(__VLS_929, new __VLS_929({
        modelValue: (__VLS_ctx.checkForm.schedule),
        placeholder: "0 9 * * 1",
    }));
    const __VLS_931 = __VLS_930({
        modelValue: (__VLS_ctx.checkForm.schedule),
        placeholder: "0 9 * * 1",
    }, ...__VLS_functionalComponentArgsRest(__VLS_930));
    // @ts-ignore
    [checkForm, checkFreqPreset,];
    var __VLS_926;
}
let __VLS_934;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_935 = __VLS_asFunctionalComponent1(__VLS_934, new __VLS_934({
    label: "时区",
}));
const __VLS_936 = __VLS_935({
    label: "时区",
}, ...__VLS_functionalComponentArgsRest(__VLS_935));
const { default: __VLS_939 } = __VLS_937.slots;
let __VLS_940;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_941 = __VLS_asFunctionalComponent1(__VLS_940, new __VLS_940({
    modelValue: (__VLS_ctx.checkForm.tz),
    ...{ style: {} },
}));
const __VLS_942 = __VLS_941({
    modelValue: (__VLS_ctx.checkForm.tz),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_941));
const { default: __VLS_945 } = __VLS_943.slots;
let __VLS_946;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_947 = __VLS_asFunctionalComponent1(__VLS_946, new __VLS_946({
    label: "Asia/Shanghai",
    value: "Asia/Shanghai",
}));
const __VLS_948 = __VLS_947({
    label: "Asia/Shanghai",
    value: "Asia/Shanghai",
}, ...__VLS_functionalComponentArgsRest(__VLS_947));
let __VLS_951;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_952 = __VLS_asFunctionalComponent1(__VLS_951, new __VLS_951({
    label: "UTC",
    value: "UTC",
}));
const __VLS_953 = __VLS_952({
    label: "UTC",
    value: "UTC",
}, ...__VLS_functionalComponentArgsRest(__VLS_952));
let __VLS_956;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_957 = __VLS_asFunctionalComponent1(__VLS_956, new __VLS_956({
    label: "America/New_York",
    value: "America/New_York",
}));
const __VLS_958 = __VLS_957({
    label: "America/New_York",
    value: "America/New_York",
}, ...__VLS_functionalComponentArgsRest(__VLS_957));
// @ts-ignore
[checkForm,];
var __VLS_943;
// @ts-ignore
[];
var __VLS_937;
let __VLS_961;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_962 = __VLS_asFunctionalComponent1(__VLS_961, new __VLS_961({
    label: "检查提示词",
}));
const __VLS_963 = __VLS_962({
    label: "检查提示词",
}, ...__VLS_functionalComponentArgsRest(__VLS_962));
const { default: __VLS_966 } = __VLS_964.slots;
let __VLS_967;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_968 = __VLS_asFunctionalComponent1(__VLS_967, new __VLS_967({
    modelValue: (__VLS_ctx.checkForm.prompt),
    type: "textarea",
    rows: (3),
    placeholder: "可用变量：{goal.title} {goal.progress} {goal.endAt}",
}));
const __VLS_969 = __VLS_968({
    modelValue: (__VLS_ctx.checkForm.prompt),
    type: "textarea",
    rows: (3),
    placeholder: "可用变量：{goal.title} {goal.progress} {goal.endAt}",
}, ...__VLS_functionalComponentArgsRest(__VLS_968));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
// @ts-ignore
[checkForm,];
var __VLS_964;
let __VLS_972;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_973 = __VLS_asFunctionalComponent1(__VLS_972, new __VLS_972({
    label: "启用",
}));
const __VLS_974 = __VLS_973({
    label: "启用",
}, ...__VLS_functionalComponentArgsRest(__VLS_973));
const { default: __VLS_977 } = __VLS_975.slots;
let __VLS_978;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_979 = __VLS_asFunctionalComponent1(__VLS_978, new __VLS_978({
    modelValue: (__VLS_ctx.checkForm.enabled),
}));
const __VLS_980 = __VLS_979({
    modelValue: (__VLS_ctx.checkForm.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_979));
// @ts-ignore
[checkForm,];
var __VLS_975;
// @ts-ignore
[];
var __VLS_853;
{
    const { footer: __VLS_983 } = __VLS_847.slots;
    let __VLS_984;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_985 = __VLS_asFunctionalComponent1(__VLS_984, new __VLS_984({
        ...{ 'onClick': {} },
    }));
    const __VLS_986 = __VLS_985({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_985));
    let __VLS_989;
    const __VLS_990 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.checkDialogVisible = false;
                // @ts-ignore
                [checkDialogVisible,];
            } });
    const { default: __VLS_991 } = __VLS_987.slots;
    // @ts-ignore
    [];
    var __VLS_987;
    var __VLS_988;
    let __VLS_992;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_993 = __VLS_asFunctionalComponent1(__VLS_992, new __VLS_992({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_994 = __VLS_993({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_993));
    let __VLS_997;
    const __VLS_998 = ({ click: {} },
        { onClick: (__VLS_ctx.submitAddCheck) });
    const { default: __VLS_999 } = __VLS_995.slots;
    // @ts-ignore
    [submitAddCheck,];
    var __VLS_995;
    var __VLS_996;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_847;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
