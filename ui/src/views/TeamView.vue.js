/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, onUnmounted, reactive, watch, nextTick } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Search } from '@element-plus/icons-vue';
import { relationsApi, agents as agentsApi, networkApi } from '../api';
import RelTypeForm from '../components/RelTypeForm.vue';
const svgRef = ref();
const graphContainerRef = ref();
const loading = ref(false);
const graph = ref({ nodes: [], edges: [] });
// ── Layout constants ───────────────────────────────────────────────────────
const svgW = ref(860); // updated by ResizeObserver
const NODE_R = 28;
const LEVEL_H = 160;
const PAD_TOP = 90;
const PAD_X = 80;
const strengthWidths = { '核心': 4, '常用': 2.5, '偶尔': 1.5 };
const typeColors = {
    '上下级': '#7c3aed',
    // 向后兼容旧数据（Graph() 已将其转换，但保留以防万一）
    '上级': '#7c3aed', '下级': '#7c3aed',
    '平级协作': '#409eff', '支持': '#67c23a', '其他': '#909399',
};
function edgeColor(type) { return typeColors[type] ?? '#94a3b8'; }
function isDirectional(type) { return type === '上下级' || type === '上级' || type === '下级'; }
// ── Hierarchy layout ───────────────────────────────────────────────────────
function computeLevels(nodes, edges) {
    const levels = {};
    nodes.forEach(n => { levels[n.id] = 0; });
    const maxLevel = nodes.length + 1; // 防止循环依赖导致层级无限增长
    const maxIter = nodes.length + 2;
    for (let iter = 0; iter < maxIter; iter++) {
        let changed = false;
        for (const edge of edges) {
            const lf = Math.min(levels[edge.from] ?? 0, maxLevel);
            const lt = levels[edge.to] ?? 0;
            if (edge.type === '上下级') {
                const want = Math.min(lf + 1, maxLevel);
                if (lt < want) {
                    levels[edge.to] = want;
                    changed = true;
                }
            }
            else if (edge.type === '上级') {
                const want = lf - 1;
                if (lt > want) {
                    levels[edge.to] = want;
                    changed = true;
                }
            }
            else if (edge.type === '下级') {
                const want = Math.min(lf + 1, maxLevel);
                if (lt < want) {
                    levels[edge.to] = want;
                    changed = true;
                }
            }
        }
        if (!changed)
            break;
    }
    const vals = Object.values(levels);
    const minL = vals.length ? Math.min(...vals) : 0;
    nodes.forEach(n => { levels[n.id] = (levels[n.id] ?? 0) - minL; });
    return levels;
}
const levelMap = computed(() => computeLevels(graph.value.nodes, graph.value.edges));
// ── 建议连接：未建立关系的 agent 对 ────────────────────────────────────────
const suggestOpen = ref(true);
const suggestSaving = ref('');
const suggestions = computed(() => {
    const nodes = graph.value.nodes;
    if (nodes.length < 2)
        return [];
    const edgeSet = new Set();
    for (const e of graph.value.edges) {
        // 用 "小id|大id" 做无向归一化 key
        const a = e.from < e.to ? e.from : e.to;
        const b = e.from < e.to ? e.to : e.from;
        edgeSet.add(`${a}|${b}`);
    }
    const out = [];
    for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
            const na = nodes[i];
            const nb = nodes[j];
            const key = na.id < nb.id ? `${na.id}|${nb.id}` : `${nb.id}|${na.id}`;
            if (!edgeSet.has(key)) {
                out.push({ from: na.id, to: nb.id, fromName: na.name, toName: nb.name });
            }
        }
    }
    return out;
});
// 一键建立平级协作关系（默认 strength=常用, desc 空）
async function quickConnect(from, to) {
    const key = `${from}|${to}`;
    if (suggestSaving.value)
        return;
    suggestSaving.value = key;
    try {
        await relationsApi.putEdge(from, to, '平级协作', '常用', '');
        ElMessage.success('关系已建立（平级协作）');
        await loadGraph();
    }
    catch {
        ElMessage.error('建立失败');
    }
    finally {
        suggestSaving.value = '';
    }
}
const svgH = computed(() => {
    const maxLevel = Object.values(levelMap.value).reduce((m, v) => Math.max(m, v), 0);
    return Math.max(600, PAD_TOP + maxLevel * LEVEL_H + 160);
});
const posMap = computed(() => {
    const nodes = graph.value.nodes;
    const levels = levelMap.value;
    const w = svgW.value;
    const byLevel = {};
    nodes.forEach(n => {
        const lv = levels[n.id] ?? 0;
        if (!byLevel[lv])
            byLevel[lv] = [];
        byLevel[lv].push(n.id);
    });
    const map = {};
    for (const [lvStr, ids] of Object.entries(byLevel)) {
        const lv = Number(lvStr);
        const y = PAD_TOP + lv * LEVEL_H;
        const usableW = w - PAD_X * 2;
        const gap = ids.length > 1 ? usableW / (ids.length - 1) : 0;
        ids.forEach((id, i) => {
            map[id] = {
                x: Math.round(ids.length === 1 ? w / 2 : PAD_X + i * gap),
                y: Math.round(y),
            };
        });
    }
    return map;
});
const dragPositions = ref({});
const dragState = ref(null);
const mousePos = ref({ x: 400, y: PAD_TOP });
// 记录上一次是否为拖拽结束（mouseup 先于 click 触发，需跨事件传递）
const lastDragId = ref(null);
/** Effective position: drag override → computed layout */
function effPos(id) {
    return dragPositions.value[id] ?? posMap.value[id] ?? { x: svgW.value / 2, y: PAD_TOP };
}
// ── Document-level drag (works even when pointer leaves SVG) ──────────────
function onNodeMouseDown(e, nodeId) {
    e.preventDefault();
    lastDragId.value = null; // 每次按下都重置
    const nodePos = effPos(nodeId);
    const mouseInSvg = clientToSvg(e.clientX, e.clientY);
    dragState.value = {
        id: nodeId,
        startClientX: e.clientX, startClientY: e.clientY,
        // 节点中心与鼠标点击位置在 SVG 坐标系中的偏移，保持完全跟手
        offsetX: nodePos.x - mouseInSvg.x,
        offsetY: nodePos.y - mouseInSvg.y,
        moved: false,
    };
    document.addEventListener('mousemove', onDocMouseMove);
    document.addEventListener('mouseup', onDocMouseUp);
}
/** Convert client coords → SVG coordinate space.
 *  使用 getScreenCTM().inverse() 精确处理任意缩放/平移/DPR，
 *  比手动 rect+scale 更准确，不受 CSS width:100% 影响。 */
function clientToSvg(clientX, clientY) {
    const el = svgRef.value;
    if (!el)
        return { x: clientX, y: clientY };
    const ctm = el.getScreenCTM();
    if (!ctm) {
        // fallback: 手动换算
        const rect = el.getBoundingClientRect();
        const sx = rect.width > 0 ? svgW.value / rect.width : 1;
        const sy = rect.height > 0 ? svgH.value / rect.height : 1;
        return { x: (clientX - rect.left) * sx, y: (clientY - rect.top) * sy };
    }
    const pt = el.createSVGPoint();
    pt.x = clientX;
    pt.y = clientY;
    const r = pt.matrixTransform(ctm.inverse());
    return { x: r.x, y: r.y };
}
function onDocMouseMove(e) {
    const svgPos = clientToSvg(e.clientX, e.clientY);
    mousePos.value = svgPos;
    if (!dragState.value)
        return;
    const dx = e.clientX - dragState.value.startClientX;
    const dy = e.clientY - dragState.value.startClientY;
    if (Math.abs(dx) > 4 || Math.abs(dy) > 4)
        dragState.value.moved = true;
    if (!dragState.value.moved)
        return;
    // 直接用 SVG 坐标 + 初始偏移，完全跟手，无需缩放计算
    const newX = svgPos.x + dragState.value.offsetX;
    const newY = svgPos.y + dragState.value.offsetY;
    // 只限左/上边界，右/下不设硬墙（SVG overflow:visible 自然溢出）
    const minX = NODE_R + 4, maxX = Infinity;
    const minY = NODE_R + 4, maxY = Infinity;
    dragPositions.value = {
        ...dragPositions.value,
        [dragState.value.id]: {
            x: Math.round(Math.max(minX, Math.min(maxX, newX))),
            y: Math.round(Math.max(minY, Math.min(maxY, newY))),
        },
    };
}
function onDocMouseUp() {
    if (dragState.value?.moved) {
        lastDragId.value = dragState.value.id; // 标记刚刚拖拽结束的节点
    }
    dragState.value = null;
    document.removeEventListener('mousemove', onDocMouseMove);
    document.removeEventListener('mouseup', onDocMouseUp);
}
function onSvgMouseMove(e) {
    if (!dragState.value)
        mousePos.value = clientToSvg(e.clientX, e.clientY);
}
function onSvgBgClick() { selectedNode.value = null; }
// ── Connection creation + node edit ──────────────────────────────────────
const selectedNode = ref(null);
const editingColor = ref('#409EFF');
const savingColor = ref(false);
watch(selectedNode, (id) => {
    if (!id)
        return;
    const node = graph.value.nodes.find(n => n.id === id);
    editingColor.value = node?.avatarColor ?? nodeColor(id);
});
async function saveNodeColor() {
    if (!selectedNode.value || savingColor.value)
        return;
    savingColor.value = true;
    try {
        await agentsApi.update(selectedNode.value, { avatarColor: editingColor.value });
        ElMessage.success('头像颜色已更新');
        await loadGraph(); // 刷新图谱使颜色生效
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        savingColor.value = false;
    }
}
function onNodeClick(nodeId) {
    // dragState 在 mouseup 时已清空，用 lastDragId 判断是否为拖拽结束
    if (lastDragId.value === nodeId) {
        lastDragId.value = null;
        return; // 拖拽结束，忽略此次 click
    }
    lastDragId.value = null;
    if (!selectedNode.value) {
        selectedNode.value = nodeId;
        return;
    }
    if (selectedNode.value === nodeId) {
        selectedNode.value = null;
        return;
    }
    const from = selectedNode.value;
    selectedNode.value = null;
    openCreateRel(from, nodeId);
}
// ── Edge helpers ──────────────────────────────────────────────────────────
function edgePt(fromId, toId, end) {
    const a = effPos(fromId);
    const b = effPos(toId);
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const len = Math.sqrt(dx * dx + dy * dy) || 1;
    const r = NODE_R + 3;
    if (end === 'start')
        return { x: a.x + (dx / len) * r, y: a.y + (dy / len) * r };
    return { x: b.x - (dx / len) * r, y: b.y - (dy / len) * r };
}
function edgeWidth(strength) { return strengthWidths[strength] ?? 1.5; }
// ── Node helpers ──────────────────────────────────────────────────────────
const palette = ['#409EFF', '#67C23A', '#E6A23C', '#F56C6C', '#7C3AED', '#0891B2', '#B45309', '#64748B'];
function nodeColor(id) {
    // 优先使用成员配置的头像颜色
    const node = graph.value.nodes.find(n => n.id === id);
    if (node?.avatarColor)
        return node.avatarColor;
    // fallback: hash-based
    let h = 0;
    for (let i = 0; i < id.length; i++)
        h = id.charCodeAt(i) + ((h << 5) - h);
    return palette[Math.abs(h) % palette.length] ?? '#409EFF';
}
function nodeInitial(id) { return (id || '?').charAt(0).toUpperCase(); }
function nodeName(id) { return graph.value.nodes.find(n => n.id === id)?.name ?? id; }
// ── Auto arrange ──────────────────────────────────────────────────────────
function autoArrange() {
    dragPositions.value = {};
    ElMessage.success('已重置为自动布局');
}
// ── Create relation dialog ─────────────────────────────────────────────────
const createRelDialog = ref(false);
const relForm = reactive({ from: '', to: '', type: '平级协作', strength: '常用', desc: '' });
const savingRel = ref(false);
function openCreateRel(from, to) {
    // Check if relation already exists
    const exists = graph.value.edges.some(e => (e.from === from && e.to === to) || (e.from === to && e.to === from));
    if (exists) {
        const edge = graph.value.edges.find(e => (e.from === from && e.to === to) || (e.from === to && e.to === from));
        openEditEdge(edge);
        return;
    }
    relForm.from = from;
    relForm.to = to;
    relForm.type = '平级协作';
    relForm.strength = '常用';
    relForm.desc = '';
    createRelDialog.value = true;
}
async function saveCreateRel() {
    if (savingRel.value)
        return;
    savingRel.value = true;
    try {
        await relationsApi.putEdge(relForm.from, relForm.to, relForm.type, relForm.strength, relForm.desc);
        ElMessage.success('关系已建立');
        createRelDialog.value = false;
        await loadGraph();
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        savingRel.value = false;
    }
}
// ── Edit relation dialog ───────────────────────────────────────────────────
const editRelDialog = ref(false);
const editForm = reactive({ from: '', to: '', type: '平级协作', strength: '常用', desc: '' });
// 记录打开编辑弹窗时的原始方向，用于翻转后清除旧边
let originalEdgeFrom = '';
let originalEdgeTo = '';
function openEditEdge(edge) {
    editForm.from = edge.from;
    editForm.to = edge.to;
    editForm.type = edge.type;
    editForm.strength = edge.strength;
    editForm.desc = edge.label;
    originalEdgeFrom = edge.from;
    originalEdgeTo = edge.to;
    editRelDialog.value = true;
}
async function saveEditRel() {
    if (savingRel.value)
        return;
    savingRel.value = true;
    try {
        const directionChanged = editForm.from !== originalEdgeFrom || editForm.to !== originalEdgeTo;
        if (directionChanged) {
            // 方向翻转：先删掉原来的边，再建新边（避免两条边并存）
            await relationsApi.deleteEdge(originalEdgeFrom, originalEdgeTo);
        }
        await relationsApi.putEdge(editForm.from, editForm.to, editForm.type, editForm.strength, editForm.desc);
        ElMessage.success('关系已更新');
        editRelDialog.value = false;
        await loadGraph();
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        savingRel.value = false;
    }
}
async function confirmDeleteEdge() {
    try {
        await ElMessageBox.confirm(`删除 ${nodeName(editForm.from)} ↔ ${nodeName(editForm.to)} 的关系？`, '删除关系', {
            confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
        });
    }
    catch {
        return;
    }
    savingRel.value = true;
    try {
        await relationsApi.deleteEdge(editForm.from, editForm.to);
        ElMessage.success('关系已删除');
        editRelDialog.value = false;
        await loadGraph();
    }
    catch {
        ElMessage.error('删除失败');
    }
    finally {
        savingRel.value = false;
    }
}
// ── Load ───────────────────────────────────────────────────────────────────
async function loadGraph() {
    loading.value = true;
    try {
        const res = await relationsApi.graph();
        graph.value = res.data;
    }
    catch {
        ElMessage.error('加载图谱失败');
    }
    finally {
        loading.value = false;
    }
}
async function clearAllRelations() {
    try {
        await ElMessageBox.confirm('将清空所有成员的关系，不可恢复。确认吗？', '清空所有关系', {
            confirmButtonText: '确认清空', cancelButtonText: '取消', type: 'warning',
        });
    }
    catch {
        return;
    }
    try {
        await relationsApi.clearAll();
        ElMessage.success('已清空所有成员关系');
        await loadGraph();
    }
    catch {
        ElMessage.error('清空失败');
    }
}
const tab = ref('graph');
// Sub-tab inside Contacts tab (26.4.24v1): 联系人 vs 群聊
const subTab = ref('people');
const contacts = ref([]);
const contactsLoading = ref(false);
const contactSearch = ref('');
const contactSource = ref('');
const contactAgentFilter = ref('');
const agentNameById = computed(() => {
    const m = {};
    for (const n of graph.value.nodes)
        m[n.id] = n.name;
    return m;
});
const totalContactCount = computed(() => contacts.value.length);
const filteredContacts = computed(() => {
    const q = contactSearch.value.trim().toLowerCase();
    return contacts.value.filter(c => {
        if (contactSource.value && c.source !== contactSource.value)
            return false;
        if (contactAgentFilter.value && c.agentId !== contactAgentFilter.value)
            return false;
        if (!q)
            return true;
        const hay = ((c.displayName || '') + ' ' +
            c.id + ' ' +
            (c.tags || []).join(' ') + ' ' +
            c.source).toLowerCase();
        return hay.includes(q);
    });
});
async function loadContacts() {
    contactsLoading.value = true;
    try {
        const nodes = graph.value.nodes;
        const results = [];
        await Promise.all(nodes.map(async (n) => {
            try {
                const res = await networkApi.list(n.id);
                for (const c of (res.data?.contacts || [])) {
                    results.push({ ...c, agentId: n.id });
                }
            }
            catch { /* ignore per-agent failure */ }
        }));
        contacts.value = results;
    }
    finally {
        contactsLoading.value = false;
    }
}
async function refreshAll() {
    await loadGraph();
    if (tab.value === 'contacts') {
        if (subTab.value === 'people')
            await loadContacts();
        else
            await loadChats();
    }
}
watch(tab, (t) => {
    if (t === 'contacts') {
        if (subTab.value === 'people' && contacts.value.length === 0)
            loadContacts();
        if (subTab.value === 'chats' && chats.value.length === 0)
            loadChats();
    }
});
// ── Contact drawer ────────────────────────────────────────────────────────
const contactDrawerOpen = ref(false);
const drawerContact = ref(null);
const drawerSaving = ref(false);
const presetTags = ['家人', '同事', '客户', '合作伙伴', '朋友', 'AI 成员'];
const addingTag = ref(false);
const newTagText = ref('');
const tagInputRef = ref(null);
const drawerTitle = computed(() => {
    if (!drawerContact.value)
        return '联系人详情';
    return `✏️ ${drawerContact.value.displayName || drawerContact.value.id}`;
});
async function openContactDrawer(c) {
    try {
        const res = await networkApi.get(c.agentId, c.id);
        drawerContact.value = { ...res.data, agentId: c.agentId, tags: res.data.tags || [] };
        contactDrawerOpen.value = true;
    }
    catch {
        ElMessage.error('读取联系人失败');
    }
}
function beginAddTag() {
    addingTag.value = true;
    newTagText.value = '';
    nextTick(() => tagInputRef.value?.focus?.());
}
function commitTag() {
    const t = newTagText.value.trim();
    if (t && drawerContact.value) {
        if (!drawerContact.value.tags)
            drawerContact.value.tags = [];
        if (!drawerContact.value.tags.includes(t))
            drawerContact.value.tags.push(t);
    }
    addingTag.value = false;
    newTagText.value = '';
}
function addPresetTag(t) {
    if (!drawerContact.value)
        return;
    if (!drawerContact.value.tags)
        drawerContact.value.tags = [];
    if (!drawerContact.value.tags.includes(t))
        drawerContact.value.tags.push(t);
}
async function saveContact() {
    if (!drawerContact.value)
        return;
    const c = drawerContact.value;
    drawerSaving.value = true;
    try {
        await networkApi.update(c.agentId, c.id, {
            displayName: c.displayName,
            tags: c.tags || [],
            body: c.body,
            isOwner: !!c.isOwner,
        });
        ElMessage.success('已保存');
        contactDrawerOpen.value = false;
        await loadContacts();
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        drawerSaving.value = false;
    }
}
async function deleteContact() {
    if (!drawerContact.value)
        return;
    const c = drawerContact.value;
    try {
        await ElMessageBox.confirm(`删除 ${c.displayName || c.id}？此操作只移除该 agent 的档案。`, '确认删除', {
            confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
        });
    }
    catch {
        return;
    }
    drawerSaving.value = true;
    try {
        await networkApi.delete(c.agentId, c.id);
        ElMessage.success('已删除');
        contactDrawerOpen.value = false;
        await loadContacts();
    }
    catch {
        ElMessage.error('删除失败');
    }
    finally {
        drawerSaving.value = false;
    }
}
// ── Chats sub-tab (26.4.24v1) ────────────────────────────────────────────
const chats = ref([]);
const chatsLoading = ref(false);
const chatSearch = ref('');
const chatSource = ref('');
const chatAgentFilter = ref('');
const totalChatCount = computed(() => chats.value.length);
const filteredChats = computed(() => {
    const q = chatSearch.value.trim().toLowerCase();
    return chats.value.filter(c => {
        if (chatSource.value && c.source !== chatSource.value)
            return false;
        if (chatAgentFilter.value && c.agentId !== chatAgentFilter.value)
            return false;
        if (!q)
            return true;
        const hay = ((c.title || '') + ' ' +
            c.id + ' ' +
            (c.kind || '') + ' ' +
            (c.tags || []).join(' ') + ' ' +
            c.source).toLowerCase();
        return hay.includes(q);
    });
});
async function loadChats() {
    chatsLoading.value = true;
    try {
        const nodes = graph.value.nodes;
        const results = [];
        await Promise.all(nodes.map(async (n) => {
            try {
                const res = await networkApi.listChats(n.id);
                for (const c of (res.data?.chats || [])) {
                    results.push({ ...c, agentId: n.id });
                }
            }
            catch { /* ignore per-agent failure */ }
        }));
        chats.value = results;
    }
    finally {
        chatsLoading.value = false;
    }
}
watch(subTab, (s) => {
    if (s === 'chats' && chats.value.length === 0)
        loadChats();
});
const chatDrawerOpen = ref(false);
const drawerChat = ref(null);
const chatDrawerSaving = ref(false);
const presetChatTags = ['内部', '客户', '支持', '社区'];
const addingChatTag = ref(false);
const newChatTagText = ref('');
const chatTagInputRef = ref(null);
const chatDrawerTitle = computed(() => {
    if (!drawerChat.value)
        return '群聊详情';
    return `💬 ${drawerChat.value.title || drawerChat.value.id}`;
});
async function openChatDrawer(c) {
    try {
        const res = await networkApi.getChat(c.agentId, c.id);
        drawerChat.value = { ...res.data, agentId: c.agentId, tags: res.data.tags || [] };
        chatDrawerOpen.value = true;
    }
    catch {
        ElMessage.error('读取群档案失败');
    }
}
function beginAddChatTag() {
    addingChatTag.value = true;
    newChatTagText.value = '';
    nextTick(() => chatTagInputRef.value?.focus?.());
}
function commitChatTag() {
    const t = newChatTagText.value.trim();
    if (t && drawerChat.value) {
        if (!drawerChat.value.tags)
            drawerChat.value.tags = [];
        if (!drawerChat.value.tags.includes(t))
            drawerChat.value.tags.push(t);
    }
    addingChatTag.value = false;
    newChatTagText.value = '';
}
function addPresetChatTag(t) {
    if (!drawerChat.value)
        return;
    if (!drawerChat.value.tags)
        drawerChat.value.tags = [];
    if (!drawerChat.value.tags.includes(t))
        drawerChat.value.tags.push(t);
}
async function saveChat() {
    if (!drawerChat.value)
        return;
    const c = drawerChat.value;
    chatDrawerSaving.value = true;
    try {
        await networkApi.updateChat(c.agentId, c.id, {
            title: c.title,
            kind: c.kind,
            tags: c.tags || [],
            body: c.body,
            memberCount: c.memberCount,
        });
        ElMessage.success('已保存');
        chatDrawerOpen.value = false;
        await loadChats();
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        chatDrawerSaving.value = false;
    }
}
async function deleteChat() {
    if (!drawerChat.value)
        return;
    const c = drawerChat.value;
    try {
        await ElMessageBox.confirm(`删除群档案 ${c.title || c.id}？此操作只移除该 agent 的档案。`, '确认删除', {
            confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
        });
    }
    catch {
        return;
    }
    chatDrawerSaving.value = true;
    try {
        await networkApi.deleteChat(c.agentId, c.id);
        ElMessage.success('已删除');
        chatDrawerOpen.value = false;
        await loadChats();
    }
    catch {
        ElMessage.error('删除失败');
    }
    finally {
        chatDrawerSaving.value = false;
    }
}
function chatKindLabel(k) {
    switch (k) {
        case 'group': return '群组';
        case 'supergroup': return '超级群';
        case 'channel': return '频道';
        case 'private': return '私聊';
        case 'p2p': return '私聊';
        default: return k || '其它';
    }
}
// Helpers
function sourceLabel(s) {
    switch (s) {
        case 'feishu': return '飞书';
        case 'telegram': return 'Telegram';
        case 'web': return 'Web';
        case 'panel': return '面板';
        default: return s || '其它';
    }
}
function sourceTagType(s) {
    switch (s) {
        case 'feishu': return 'primary';
        case 'telegram': return 'success';
        case 'web': return 'warning';
        default: return 'info';
    }
}
function avatarColor(seed) {
    // Deterministic pastel color from string hash.
    let h = 0;
    for (let i = 0; i < seed.length; i++)
        h = (h * 31 + seed.charCodeAt(i)) >>> 0;
    const hue = h % 360;
    return `hsl(${hue}deg 55% 62%)`;
}
function formatLastSeen(iso) {
    try {
        const d = new Date(iso);
        const delta = Date.now() - d.getTime();
        if (delta < 60_000)
            return '刚刚';
        if (delta < 3600_000)
            return Math.floor(delta / 60_000) + '分钟前';
        if (delta < 86400_000)
            return Math.floor(delta / 3600_000) + '小时前';
        return d.toLocaleDateString();
    }
    catch {
        return iso;
    }
}
let ro = null;
onMounted(() => {
    loadGraph();
    if (graphContainerRef.value) {
        ro = new ResizeObserver(entries => {
            const w = entries[0]?.contentRect.width;
            if (w && w > 100)
                svgW.value = Math.floor(w);
            // ⚠️ Do NOT reset dragPositions here — that would cancel user drags
        });
        ro.observe(graphContainerRef.value);
    }
});
onUnmounted(() => {
    ro?.disconnect();
    document.removeEventListener('mousemove', onDocMouseMove);
    document.removeEventListener('mouseup', onDocMouseUp);
});
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['page-header']} */ ;
/** @type {__VLS_StyleScopedClasses['graph-node']} */ ;
/** @type {__VLS_StyleScopedClasses['suggest-row']} */ ;
/** @type {__VLS_StyleScopedClasses['tab-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['tab-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['tab-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['tab-count']} */ ;
/** @type {__VLS_StyleScopedClasses['sub-tab-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['sub-tab-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['sub-tab-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['sub-tab-count']} */ ;
/** @type {__VLS_StyleScopedClasses['contact-row']} */ ;
/** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
/** @type {__VLS_StyleScopedClasses['connect-banner']} */ ;
/** @type {__VLS_StyleScopedClasses['node-edit-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['graph-container']} */ ;
/** @type {__VLS_StyleScopedClasses['legend']} */ ;
/** @type {__VLS_StyleScopedClasses['rel-pair']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "team-view" },
});
/** @type {__VLS_StyleScopedClasses['team-view']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "page-header" },
});
/** @type {__VLS_StyleScopedClasses['page-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({});
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    type: "info",
    size: "small",
    ...{ style: {} },
}));
const __VLS_2 = __VLS_1({
    type: "info",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
const { default: __VLS_5 } = __VLS_3.slots;
var __VLS_3;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
if (__VLS_ctx.tab === 'graph') {
    let __VLS_6;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_8 = __VLS_7({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_7));
    let __VLS_11;
    const __VLS_12 = ({ click: {} },
        { onClick: (__VLS_ctx.autoArrange) });
    const { default: __VLS_13 } = __VLS_9.slots;
    let __VLS_14;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({}));
    const __VLS_16 = __VLS_15({}, ...__VLS_functionalComponentArgsRest(__VLS_15));
    const { default: __VLS_19 } = __VLS_17.slots;
    let __VLS_20;
    /** @ts-ignore @type { | typeof __VLS_components.Grid} */
    Grid;
    // @ts-ignore
    const __VLS_21 = __VLS_asFunctionalComponent1(__VLS_20, new __VLS_20({}));
    const __VLS_22 = __VLS_21({}, ...__VLS_functionalComponentArgsRest(__VLS_21));
    // @ts-ignore
    [tab, autoArrange,];
    var __VLS_17;
    // @ts-ignore
    [];
    var __VLS_9;
    var __VLS_10;
}
let __VLS_25;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({
    ...{ 'onClick': {} },
    size: "small",
}));
const __VLS_27 = __VLS_26({
    ...{ 'onClick': {} },
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_26));
let __VLS_30;
const __VLS_31 = ({ click: {} },
    { onClick: (__VLS_ctx.refreshAll) });
const { default: __VLS_32 } = __VLS_28.slots;
let __VLS_33;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_34 = __VLS_asFunctionalComponent1(__VLS_33, new __VLS_33({}));
const __VLS_35 = __VLS_34({}, ...__VLS_functionalComponentArgsRest(__VLS_34));
const { default: __VLS_38 } = __VLS_36.slots;
let __VLS_39;
/** @ts-ignore @type { | typeof __VLS_components.Refresh} */
Refresh;
// @ts-ignore
const __VLS_40 = __VLS_asFunctionalComponent1(__VLS_39, new __VLS_39({}));
const __VLS_41 = __VLS_40({}, ...__VLS_functionalComponentArgsRest(__VLS_40));
// @ts-ignore
[refreshAll,];
var __VLS_36;
// @ts-ignore
[];
var __VLS_28;
var __VLS_29;
if (__VLS_ctx.tab === 'graph') {
    let __VLS_44;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
        plain: true,
    }));
    const __VLS_46 = __VLS_45({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
        plain: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_45));
    let __VLS_49;
    const __VLS_50 = ({ click: {} },
        { onClick: (__VLS_ctx.clearAllRelations) });
    const { default: __VLS_51 } = __VLS_47.slots;
    let __VLS_52;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_53 = __VLS_asFunctionalComponent1(__VLS_52, new __VLS_52({}));
    const __VLS_54 = __VLS_53({}, ...__VLS_functionalComponentArgsRest(__VLS_53));
    const { default: __VLS_57 } = __VLS_55.slots;
    let __VLS_58;
    /** @ts-ignore @type { | typeof __VLS_components.Delete} */
    Delete;
    // @ts-ignore
    const __VLS_59 = __VLS_asFunctionalComponent1(__VLS_58, new __VLS_58({}));
    const __VLS_60 = __VLS_59({}, ...__VLS_functionalComponentArgsRest(__VLS_59));
    // @ts-ignore
    [tab, clearAllRelations,];
    var __VLS_55;
    // @ts-ignore
    [];
    var __VLS_47;
    var __VLS_48;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "tab-bar" },
});
/** @type {__VLS_StyleScopedClasses['tab-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.tab = 'graph';
            // @ts-ignore
            [tab,];
        } },
    ...{ class: (['tab-btn', { active: __VLS_ctx.tab === 'graph' }]) },
});
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['tab-btn']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "tab-count" },
});
/** @type {__VLS_StyleScopedClasses['tab-count']} */ ;
(__VLS_ctx.graph.nodes.length);
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.tab = 'contacts';
            // @ts-ignore
            [tab, tab, graph,];
        } },
    ...{ class: (['tab-btn', { active: __VLS_ctx.tab === 'contacts' }]) },
});
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['tab-btn']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "tab-count" },
});
/** @type {__VLS_StyleScopedClasses['tab-count']} */ ;
(__VLS_ctx.totalContactCount);
let __VLS_63;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_64 = __VLS_asFunctionalComponent1(__VLS_63, new __VLS_63({
    ...{ class: "graph-card" },
}));
const __VLS_65 = __VLS_64({
    ...{ class: "graph-card" },
}, ...__VLS_functionalComponentArgsRest(__VLS_64));
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.tab === 'graph') }, null, null);
__VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.loading) }, null, null);
/** @type {__VLS_StyleScopedClasses['graph-card']} */ ;
const { default: __VLS_68 } = __VLS_66.slots;
if (!__VLS_ctx.loading && !__VLS_ctx.graph.nodes.length) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "empty-state" },
    });
    /** @type {__VLS_StyleScopedClasses['empty-state']} */ ;
    let __VLS_69;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_70 = __VLS_asFunctionalComponent1(__VLS_69, new __VLS_69({
        ...{ style: {} },
    }));
    const __VLS_71 = __VLS_70({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_70));
    const { default: __VLS_74 } = __VLS_72.slots;
    let __VLS_75;
    /** @ts-ignore @type { | typeof __VLS_components.Share} */
    Share;
    // @ts-ignore
    const __VLS_76 = __VLS_asFunctionalComponent1(__VLS_75, new __VLS_75({}));
    const __VLS_77 = __VLS_76({}, ...__VLS_functionalComponentArgsRest(__VLS_76));
    // @ts-ignore
    [tab, tab, graph, totalContactCount, vLoading, loading, loading,];
    var __VLS_72;
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
        ...{ style: {} },
    });
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "graph-container" },
        ref: "graphContainerRef",
    });
    /** @type {__VLS_StyleScopedClasses['graph-container']} */ ;
    if (__VLS_ctx.selectedNode) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "connect-banner" },
            ...{ style: {} },
        });
        /** @type {__VLS_StyleScopedClasses['connect-banner']} */ ;
        let __VLS_80;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_81 = __VLS_asFunctionalComponent1(__VLS_80, new __VLS_80({
            ...{ style: {} },
        }));
        const __VLS_82 = __VLS_81({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_81));
        const { default: __VLS_85 } = __VLS_83.slots;
        let __VLS_86;
        /** @ts-ignore @type { | typeof __VLS_components.Link} */
        Link;
        // @ts-ignore
        const __VLS_87 = __VLS_asFunctionalComponent1(__VLS_86, new __VLS_86({}));
        const __VLS_88 = __VLS_87({}, ...__VLS_functionalComponentArgsRest(__VLS_87));
        // @ts-ignore
        [selectedNode,];
        var __VLS_83;
        __VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({
            ...{ style: {} },
        });
        (__VLS_ctx.nodeName(__VLS_ctx.selectedNode));
        let __VLS_91;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_92 = __VLS_asFunctionalComponent1(__VLS_91, new __VLS_91({
            ...{ 'onClick': {} },
            size: "small",
            text: true,
            ...{ style: {} },
        }));
        const __VLS_93 = __VLS_92({
            ...{ 'onClick': {} },
            size: "small",
            text: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_92));
        let __VLS_96;
        const __VLS_97 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.loading && !__VLS_ctx.graph.nodes.length))
                        return;
                    if (!(__VLS_ctx.selectedNode))
                        return;
                    __VLS_ctx.selectedNode = null;
                    // @ts-ignore
                    [selectedNode, selectedNode, nodeName,];
                } });
        const { default: __VLS_98 } = __VLS_94.slots;
        // @ts-ignore
        [];
        var __VLS_94;
        var __VLS_95;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "node-edit-panel" },
        });
        /** @type {__VLS_StyleScopedClasses['node-edit-panel']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.input)({
            type: "color",
            ...{ class: "color-picker-input" },
            title: "选择颜色",
        });
        (__VLS_ctx.editingColor);
        /** @type {__VLS_StyleScopedClasses['color-picker-input']} */ ;
        let __VLS_99;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_100 = __VLS_asFunctionalComponent1(__VLS_99, new __VLS_99({
            ...{ 'onClick': {} },
            size: "small",
            type: "primary",
            loading: (__VLS_ctx.savingColor),
            ...{ style: {} },
        }));
        const __VLS_101 = __VLS_100({
            ...{ 'onClick': {} },
            size: "small",
            type: "primary",
            loading: (__VLS_ctx.savingColor),
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_100));
        let __VLS_104;
        const __VLS_105 = ({ click: {} },
            { onClick: (__VLS_ctx.saveNodeColor) });
        const { default: __VLS_106 } = __VLS_102.slots;
        // @ts-ignore
        [editingColor, savingColor, saveNodeColor,];
        var __VLS_102;
        var __VLS_103;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        ...{ onMousemove: (__VLS_ctx.onSvgMouseMove) },
        ...{ onClick: (__VLS_ctx.onSvgBgClick) },
        ref: "svgRef",
        width: (__VLS_ctx.svgW),
        height: (__VLS_ctx.svgH),
        ...{ class: "graph-svg" },
        ...{ style: {} },
    });
    /** @type {__VLS_StyleScopedClasses['graph-svg']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.defs, __VLS_intrinsics.defs)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.pattern, __VLS_intrinsics.pattern)({
        id: "smallGrid",
        width: "20",
        height: "20",
        patternUnits: "userSpaceOnUse",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M 20 0 L 0 0 0 20",
        fill: "none",
        stroke: "#e8ecf0",
        'stroke-width': "0.5",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.pattern, __VLS_intrinsics.pattern)({
        id: "grid",
        width: "100",
        height: "100",
        patternUnits: "userSpaceOnUse",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.rect)({
        width: "100",
        height: "100",
        fill: "url(#smallGrid)",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M 100 0 L 0 0 0 100",
        fill: "none",
        stroke: "#dde1e7",
        'stroke-width': "1",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.marker, __VLS_intrinsics.marker)({
        id: "arrow-上下级",
        markerWidth: "10",
        markerHeight: "10",
        refX: "9",
        refY: "5",
        orient: "auto",
        markerUnits: "userSpaceOnUse",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M1,2 L1,8 L9,5 z",
        fill: "#7c3aed",
        'fill-opacity': "0.85",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.marker, __VLS_intrinsics.marker)({
        id: "arrow-上级",
        markerWidth: "10",
        markerHeight: "10",
        refX: "9",
        refY: "5",
        orient: "auto",
        markerUnits: "userSpaceOnUse",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M1,2 L1,8 L9,5 z",
        fill: "#7c3aed",
        'fill-opacity': "0.85",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.marker, __VLS_intrinsics.marker)({
        id: "arrow-下级",
        markerWidth: "10",
        markerHeight: "10",
        refX: "9",
        refY: "5",
        orient: "auto",
        markerUnits: "userSpaceOnUse",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M1,2 L1,8 L9,5 z",
        fill: "#7c3aed",
        'fill-opacity': "0.85",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.rect)({
        width: "100%",
        height: "100%",
        fill: "url(#grid)",
        rx: "0",
    });
    if (__VLS_ctx.selectedNode && !__VLS_ctx.dragState) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.line)({
            x1: (__VLS_ctx.effPos(__VLS_ctx.selectedNode).x),
            y1: (__VLS_ctx.effPos(__VLS_ctx.selectedNode).y),
            x2: (__VLS_ctx.mousePos.x),
            y2: (__VLS_ctx.mousePos.y),
            stroke: "#409eff",
            'stroke-width': "2",
            'stroke-dasharray': "6,4",
            'stroke-opacity': "0.7",
            'pointer-events': "none",
        });
    }
    for (const [edge] of __VLS_vFor((__VLS_ctx.graph.edges))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.g, __VLS_intrinsics.g)({
            key: (`${edge.from}|${edge.to}`),
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.line)({
            ...{ onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.loading && !__VLS_ctx.graph.nodes.length))
                        return;
                    __VLS_ctx.openEditEdge(edge);
                    // @ts-ignore
                    [graph, selectedNode, selectedNode, selectedNode, onSvgMouseMove, onSvgBgClick, svgW, svgH, dragState, effPos, effPos, mousePos, mousePos, openEditEdge,];
                } },
            x1: (__VLS_ctx.edgePt(edge.from, edge.to, 'start').x),
            y1: (__VLS_ctx.edgePt(edge.from, edge.to, 'start').y),
            x2: (__VLS_ctx.edgePt(edge.from, edge.to, 'end').x),
            y2: (__VLS_ctx.edgePt(edge.from, edge.to, 'end').y),
            stroke: "transparent",
            'stroke-width': "14",
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.line)({
            x1: (__VLS_ctx.edgePt(edge.from, edge.to, 'start').x),
            y1: (__VLS_ctx.edgePt(edge.from, edge.to, 'start').y),
            x2: (__VLS_ctx.edgePt(edge.from, edge.to, 'end').x),
            y2: (__VLS_ctx.edgePt(edge.from, edge.to, 'end').y),
            stroke: (__VLS_ctx.edgeColor(edge.type)),
            'stroke-width': (__VLS_ctx.edgeWidth(edge.strength)),
            'stroke-opacity': "0.7",
            'stroke-linecap': "round",
            'marker-end': (__VLS_ctx.isDirectional(edge.type) ? `url(#arrow-${edge.type})` : undefined),
            'pointer-events': "none",
            ...{ class: "graph-edge" },
        });
        /** @type {__VLS_StyleScopedClasses['graph-edge']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.text, __VLS_intrinsics.text)({
            x: ((__VLS_ctx.effPos(edge.from).x + __VLS_ctx.effPos(edge.to).x) / 2),
            y: ((__VLS_ctx.effPos(edge.from).y + __VLS_ctx.effPos(edge.to).y) / 2 - 6),
            'text-anchor': "middle",
            'font-size': "10",
            fill: (__VLS_ctx.edgeColor(edge.type)),
            'pointer-events': "none",
            'paint-order': "stroke",
            stroke: "#f5f7fa",
            'stroke-width': "3",
        });
        (edge.type);
        // @ts-ignore
        [effPos, effPos, effPos, effPos, edgePt, edgePt, edgePt, edgePt, edgePt, edgePt, edgePt, edgePt, edgeColor, edgeColor, edgeWidth, isDirectional,];
    }
    for (const [node] of __VLS_vFor((__VLS_ctx.graph.nodes))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.g, __VLS_intrinsics.g)({
            ...{ onMousedown: ((e) => __VLS_ctx.onNodeMouseDown(e, node.id)) },
            ...{ onClick: (() => __VLS_ctx.onNodeClick(node.id)) },
            key: (node.id),
            transform: (`translate(${__VLS_ctx.effPos(node.id).x}, ${__VLS_ctx.effPos(node.id).y})`),
            ...{ class: (['graph-node',
                    { 'node-selected': __VLS_ctx.selectedNode === node.id },
                    { 'node-target': !!__VLS_ctx.selectedNode && __VLS_ctx.selectedNode !== node.id }]) },
            ...{ style: {} },
        });
        /** @type {__VLS_StyleScopedClasses['graph-node']} */ ;
        /** @type {__VLS_StyleScopedClasses['node-selected']} */ ;
        /** @type {__VLS_StyleScopedClasses['node-target']} */ ;
        if (__VLS_ctx.selectedNode === node.id) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
                r: "37",
                fill: "none",
                stroke: "#409eff",
                'stroke-width': "2.5",
                'stroke-dasharray': "7,3",
                ...{ class: "selection-ring" },
            });
            /** @type {__VLS_StyleScopedClasses['selection-ring']} */ ;
        }
        else if (!!__VLS_ctx.selectedNode) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
                r: "33",
                fill: "rgba(64,158,255,0.06)",
                stroke: "#409eff",
                'stroke-width': "1.5",
                'stroke-opacity': "0.5",
            });
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
            r: "30",
            fill: "rgba(0,0,0,0.07)",
            transform: "translate(2,3)",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
            r: "28",
            fill: (__VLS_ctx.nodeColor(node.id)),
            stroke: "#fff",
            'stroke-width': "2.5",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.text, __VLS_intrinsics.text)({
            'text-anchor': "middle",
            'dominant-baseline': "central",
            fill: "#fff",
            'font-weight': "700",
            'font-size': "15",
            'font-family': "system-ui, sans-serif",
        });
        (__VLS_ctx.nodeInitial(node.id));
        __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
            cx: "20",
            cy: "-20",
            r: "6",
            fill: (node.status === 'running' ? '#67C23A' : '#c0c4cc'),
            stroke: "#fff",
            'stroke-width': "1.5",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.text, __VLS_intrinsics.text)({
            'text-anchor': "middle",
            y: "46",
            'font-size': "12",
            fill: "#303133",
            'font-family': "system-ui, sans-serif",
            'paint-order': "stroke",
            stroke: "#f5f7fa",
            'stroke-width': "3",
        });
        (node.name);
        __VLS_asFunctionalElement1(__VLS_intrinsics.text, __VLS_intrinsics.text)({
            'text-anchor': "middle",
            y: "58",
            'font-size': "10",
            fill: "#909399",
            'font-family': "system-ui, monospace",
        });
        (node.id);
        // @ts-ignore
        [graph, selectedNode, selectedNode, selectedNode, selectedNode, selectedNode, effPos, effPos, onNodeMouseDown, onNodeClick, nodeColor, nodeInitial,];
    }
    if (!__VLS_ctx.graph.edges.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "no-edge-hint" },
        });
        /** @type {__VLS_StyleScopedClasses['no-edge-hint']} */ ;
    }
}
// @ts-ignore
[graph,];
var __VLS_66;
if (__VLS_ctx.suggestions.length) {
    let __VLS_107;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_108 = __VLS_asFunctionalComponent1(__VLS_107, new __VLS_107({
        ...{ class: "suggest-card" },
    }));
    const __VLS_109 = __VLS_108({
        ...{ class: "suggest-card" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_108));
    __VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.tab === 'graph') }, null, null);
    /** @type {__VLS_StyleScopedClasses['suggest-card']} */ ;
    const { default: __VLS_112 } = __VLS_110.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.suggestions.length))
                    return;
                __VLS_ctx.suggestOpen = !__VLS_ctx.suggestOpen;
                // @ts-ignore
                [tab, suggestions, suggestOpen, suggestOpen,];
            } },
        ...{ class: "suggest-head" },
    });
    /** @type {__VLS_StyleScopedClasses['suggest-head']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "suggest-title" },
    });
    /** @type {__VLS_StyleScopedClasses['suggest-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "suggest-count" },
    });
    /** @type {__VLS_StyleScopedClasses['suggest-count']} */ ;
    (__VLS_ctx.suggestions.length);
    let __VLS_113;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_114 = __VLS_asFunctionalComponent1(__VLS_113, new __VLS_113({
        ...{ class: "suggest-toggle" },
    }));
    const __VLS_115 = __VLS_114({
        ...{ class: "suggest-toggle" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_114));
    /** @type {__VLS_StyleScopedClasses['suggest-toggle']} */ ;
    const { default: __VLS_118 } = __VLS_116.slots;
    const __VLS_119 = (__VLS_ctx.suggestOpen ? 'ArrowUp' : 'ArrowDown');
    // @ts-ignore
    const __VLS_120 = __VLS_asFunctionalComponent1(__VLS_119, new __VLS_119({}));
    const __VLS_121 = __VLS_120({}, ...__VLS_functionalComponentArgsRest(__VLS_120));
    // @ts-ignore
    [suggestions, suggestOpen,];
    var __VLS_116;
    if (__VLS_ctx.suggestOpen) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "suggest-body" },
        });
        /** @type {__VLS_StyleScopedClasses['suggest-body']} */ ;
        for (const [s, idx] of __VLS_vFor((__VLS_ctx.suggestions.slice(0, 5)))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: (idx),
                ...{ class: "suggest-row" },
            });
            /** @type {__VLS_StyleScopedClasses['suggest-row']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "suggest-pair" },
            });
            /** @type {__VLS_StyleScopedClasses['suggest-pair']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "suggest-name" },
            });
            /** @type {__VLS_StyleScopedClasses['suggest-name']} */ ;
            (s.fromName);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "suggest-arrow" },
            });
            /** @type {__VLS_StyleScopedClasses['suggest-arrow']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "suggest-name" },
            });
            /** @type {__VLS_StyleScopedClasses['suggest-name']} */ ;
            (s.toName);
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "suggest-actions" },
            });
            /** @type {__VLS_StyleScopedClasses['suggest-actions']} */ ;
            let __VLS_124;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_125 = __VLS_asFunctionalComponent1(__VLS_124, new __VLS_124({
                ...{ 'onClick': {} },
                size: "small",
            }));
            const __VLS_126 = __VLS_125({
                ...{ 'onClick': {} },
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_125));
            let __VLS_129;
            const __VLS_130 = ({ click: {} },
                { onClick: (...[$event]) => {
                        if (!(__VLS_ctx.suggestions.length))
                            return;
                        if (!(__VLS_ctx.suggestOpen))
                            return;
                        __VLS_ctx.openCreateRel(s.from, s.to);
                        // @ts-ignore
                        [suggestions, suggestOpen, openCreateRel,];
                    } });
            const { default: __VLS_131 } = __VLS_127.slots;
            // @ts-ignore
            [];
            var __VLS_127;
            var __VLS_128;
            let __VLS_132;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_133 = __VLS_asFunctionalComponent1(__VLS_132, new __VLS_132({
                ...{ 'onClick': {} },
                size: "small",
                type: "primary",
                loading: (__VLS_ctx.suggestSaving === `${s.from}|${s.to}`),
            }));
            const __VLS_134 = __VLS_133({
                ...{ 'onClick': {} },
                size: "small",
                type: "primary",
                loading: (__VLS_ctx.suggestSaving === `${s.from}|${s.to}`),
            }, ...__VLS_functionalComponentArgsRest(__VLS_133));
            let __VLS_137;
            const __VLS_138 = ({ click: {} },
                { onClick: (...[$event]) => {
                        if (!(__VLS_ctx.suggestions.length))
                            return;
                        if (!(__VLS_ctx.suggestOpen))
                            return;
                        __VLS_ctx.quickConnect(s.from, s.to);
                        // @ts-ignore
                        [suggestSaving, quickConnect,];
                    } });
            const { default: __VLS_139 } = __VLS_135.slots;
            // @ts-ignore
            [];
            var __VLS_135;
            var __VLS_136;
            // @ts-ignore
            [];
        }
        if (__VLS_ctx.suggestions.length > 5) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "suggest-more" },
            });
            /** @type {__VLS_StyleScopedClasses['suggest-more']} */ ;
            (__VLS_ctx.suggestions.length - 5);
        }
    }
    // @ts-ignore
    [suggestions, suggestions,];
    var __VLS_110;
}
if (__VLS_ctx.graph.nodes.length) {
    let __VLS_140;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_141 = __VLS_asFunctionalComponent1(__VLS_140, new __VLS_140({
        ...{ class: "legend-card" },
    }));
    const __VLS_142 = __VLS_141({
        ...{ class: "legend-card" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_141));
    __VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.tab === 'graph') }, null, null);
    /** @type {__VLS_StyleScopedClasses['legend-card']} */ ;
    const { default: __VLS_145 } = __VLS_143.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "legend" },
    });
    /** @type {__VLS_StyleScopedClasses['legend']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-title" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-item" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-item']} */ ;
    let __VLS_146;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_147 = __VLS_asFunctionalComponent1(__VLS_146, new __VLS_146({}));
    const __VLS_148 = __VLS_147({}, ...__VLS_functionalComponentArgsRest(__VLS_147));
    const { default: __VLS_151 } = __VLS_149.slots;
    let __VLS_152;
    /** @ts-ignore @type { | typeof __VLS_components.ArrowUp} */
    ArrowUp;
    // @ts-ignore
    const __VLS_153 = __VLS_asFunctionalComponent1(__VLS_152, new __VLS_152({}));
    const __VLS_154 = __VLS_153({}, ...__VLS_functionalComponentArgsRest(__VLS_153));
    // @ts-ignore
    [tab, graph,];
    var __VLS_149;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-item" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-item" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-item']} */ ;
    let __VLS_157;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_158 = __VLS_asFunctionalComponent1(__VLS_157, new __VLS_157({}));
    const __VLS_159 = __VLS_158({}, ...__VLS_functionalComponentArgsRest(__VLS_158));
    const { default: __VLS_162 } = __VLS_160.slots;
    let __VLS_163;
    /** @ts-ignore @type { | typeof __VLS_components.ArrowDown} */
    ArrowDown;
    // @ts-ignore
    const __VLS_164 = __VLS_asFunctionalComponent1(__VLS_163, new __VLS_163({}));
    const __VLS_165 = __VLS_164({}, ...__VLS_functionalComponentArgsRest(__VLS_164));
    // @ts-ignore
    [];
    var __VLS_160;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-divider" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-divider']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-title" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-title']} */ ;
    for (const [w, s] of __VLS_vFor((__VLS_ctx.strengthWidths))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            key: (s),
            ...{ class: "legend-item" },
        });
        /** @type {__VLS_StyleScopedClasses['legend-item']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
            width: "28",
            height: "8",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.line)({
            x1: "0",
            y1: "4",
            x2: "28",
            y2: "4",
            stroke: "#64748b",
            'stroke-width': (w),
            'stroke-linecap': "round",
        });
        (s);
        // @ts-ignore
        [strengthWidths,];
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-divider" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-divider']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-item" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "12",
        height: "12",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
        cx: "6",
        cy: "6",
        r: "5",
        fill: "#67C23A",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "legend-item" },
    });
    /** @type {__VLS_StyleScopedClasses['legend-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "12",
        height: "12",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
        cx: "6",
        cy: "6",
        r: "5",
        fill: "#c0c4cc",
    });
    // @ts-ignore
    [];
    var __VLS_143;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "contacts-pane" },
});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.tab === 'contacts') }, null, null);
/** @type {__VLS_StyleScopedClasses['contacts-pane']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sub-tab-bar" },
});
/** @type {__VLS_StyleScopedClasses['sub-tab-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.subTab = 'people';
            // @ts-ignore
            [tab, subTab,];
        } },
    ...{ class: (['sub-tab-btn', { active: __VLS_ctx.subTab === 'people' }]) },
});
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['sub-tab-btn']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "sub-tab-count" },
});
/** @type {__VLS_StyleScopedClasses['sub-tab-count']} */ ;
(__VLS_ctx.totalContactCount);
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.subTab = 'chats';
            // @ts-ignore
            [totalContactCount, subTab, subTab,];
        } },
    ...{ class: (['sub-tab-btn', { active: __VLS_ctx.subTab === 'chats' }]) },
});
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['sub-tab-btn']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "sub-tab-count" },
});
/** @type {__VLS_StyleScopedClasses['sub-tab-count']} */ ;
(__VLS_ctx.totalChatCount);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.subTab === 'people') }, null, null);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "contact-filter-bar" },
});
/** @type {__VLS_StyleScopedClasses['contact-filter-bar']} */ ;
let __VLS_168;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input'] | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_169 = __VLS_asFunctionalComponent1(__VLS_168, new __VLS_168({
    modelValue: (__VLS_ctx.contactSearch),
    placeholder: "搜索 姓名 / ID / 标签 / 来源",
    size: "default",
    ...{ style: {} },
    clearable: true,
}));
const __VLS_170 = __VLS_169({
    modelValue: (__VLS_ctx.contactSearch),
    placeholder: "搜索 姓名 / ID / 标签 / 来源",
    size: "default",
    ...{ style: {} },
    clearable: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_169));
const { default: __VLS_173 } = __VLS_171.slots;
{
    const { prefix: __VLS_174 } = __VLS_171.slots;
    let __VLS_175;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_176 = __VLS_asFunctionalComponent1(__VLS_175, new __VLS_175({}));
    const __VLS_177 = __VLS_176({}, ...__VLS_functionalComponentArgsRest(__VLS_176));
    const { default: __VLS_180 } = __VLS_178.slots;
    let __VLS_181;
    /** @ts-ignore @type { | typeof __VLS_components.Search} */
    Search;
    // @ts-ignore
    const __VLS_182 = __VLS_asFunctionalComponent1(__VLS_181, new __VLS_181({}));
    const __VLS_183 = __VLS_182({}, ...__VLS_functionalComponentArgsRest(__VLS_182));
    // @ts-ignore
    [subTab, subTab, totalChatCount, contactSearch,];
    var __VLS_178;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_171;
let __VLS_186;
/** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
elRadioGroup;
// @ts-ignore
const __VLS_187 = __VLS_asFunctionalComponent1(__VLS_186, new __VLS_186({
    modelValue: (__VLS_ctx.contactSource),
    size: "small",
}));
const __VLS_188 = __VLS_187({
    modelValue: (__VLS_ctx.contactSource),
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_187));
const { default: __VLS_191 } = __VLS_189.slots;
let __VLS_192;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_193 = __VLS_asFunctionalComponent1(__VLS_192, new __VLS_192({
    value: "",
}));
const __VLS_194 = __VLS_193({
    value: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_193));
const { default: __VLS_197 } = __VLS_195.slots;
// @ts-ignore
[contactSource,];
var __VLS_195;
let __VLS_198;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_199 = __VLS_asFunctionalComponent1(__VLS_198, new __VLS_198({
    value: "feishu",
}));
const __VLS_200 = __VLS_199({
    value: "feishu",
}, ...__VLS_functionalComponentArgsRest(__VLS_199));
const { default: __VLS_203 } = __VLS_201.slots;
// @ts-ignore
[];
var __VLS_201;
let __VLS_204;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_205 = __VLS_asFunctionalComponent1(__VLS_204, new __VLS_204({
    value: "telegram",
}));
const __VLS_206 = __VLS_205({
    value: "telegram",
}, ...__VLS_functionalComponentArgsRest(__VLS_205));
const { default: __VLS_209 } = __VLS_207.slots;
// @ts-ignore
[];
var __VLS_207;
let __VLS_210;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_211 = __VLS_asFunctionalComponent1(__VLS_210, new __VLS_210({
    value: "web",
}));
const __VLS_212 = __VLS_211({
    value: "web",
}, ...__VLS_functionalComponentArgsRest(__VLS_211));
const { default: __VLS_215 } = __VLS_213.slots;
// @ts-ignore
[];
var __VLS_213;
// @ts-ignore
[];
var __VLS_189;
let __VLS_216;
/** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
elRadioGroup;
// @ts-ignore
const __VLS_217 = __VLS_asFunctionalComponent1(__VLS_216, new __VLS_216({
    modelValue: (__VLS_ctx.contactAgentFilter),
    size: "small",
}));
const __VLS_218 = __VLS_217({
    modelValue: (__VLS_ctx.contactAgentFilter),
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_217));
const { default: __VLS_221 } = __VLS_219.slots;
let __VLS_222;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_223 = __VLS_asFunctionalComponent1(__VLS_222, new __VLS_222({
    value: "",
}));
const __VLS_224 = __VLS_223({
    value: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_223));
const { default: __VLS_227 } = __VLS_225.slots;
// @ts-ignore
[contactAgentFilter,];
var __VLS_225;
for (const [ag] of __VLS_vFor((__VLS_ctx.graph.nodes))) {
    let __VLS_228;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_229 = __VLS_asFunctionalComponent1(__VLS_228, new __VLS_228({
        key: (ag.id),
        value: (ag.id),
    }));
    const __VLS_230 = __VLS_229({
        key: (ag.id),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_229));
    const { default: __VLS_233 } = __VLS_231.slots;
    (ag.name);
    // @ts-ignore
    [graph,];
    var __VLS_231;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_219;
if (!__VLS_ctx.contactsLoading && !__VLS_ctx.filteredContacts.length) {
    let __VLS_234;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_235 = __VLS_asFunctionalComponent1(__VLS_234, new __VLS_234({
        ...{ class: "contacts-empty" },
    }));
    const __VLS_236 = __VLS_235({
        ...{ class: "contacts-empty" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_235));
    /** @type {__VLS_StyleScopedClasses['contacts-empty']} */ ;
    const { default: __VLS_239 } = __VLS_237.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    (__VLS_ctx.contacts.length ? '当前筛选无结果' : '还没有联系人。对话一次就会自动出现。');
    // @ts-ignore
    [contactsLoading, filteredContacts, contacts,];
    var __VLS_237;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "contact-list" },
    });
    __VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.contactsLoading) }, null, null);
    /** @type {__VLS_StyleScopedClasses['contact-list']} */ ;
    for (const [c] of __VLS_vFor((__VLS_ctx.filteredContacts))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.contactsLoading && !__VLS_ctx.filteredContacts.length))
                        return;
                    __VLS_ctx.openContactDrawer(c);
                    // @ts-ignore
                    [vLoading, contactsLoading, filteredContacts, openContactDrawer,];
                } },
            key: (c.agentId + '|' + c.id),
            ...{ class: "contact-row" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-avatar" },
            ...{ style: ({ background: __VLS_ctx.avatarColor(c.displayName || c.id) }) },
        });
        /** @type {__VLS_StyleScopedClasses['contact-avatar']} */ ;
        ((c.displayName || c.id).slice(0, 1));
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-main" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-main']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-name-row" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-name-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "contact-name" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-name']} */ ;
        (c.displayName || c.id);
        let __VLS_240;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_241 = __VLS_asFunctionalComponent1(__VLS_240, new __VLS_240({
            size: "small",
            type: (__VLS_ctx.sourceTagType(c.source)),
            effect: "plain",
            ...{ style: {} },
        }));
        const __VLS_242 = __VLS_241({
            size: "small",
            type: (__VLS_ctx.sourceTagType(c.source)),
            effect: "plain",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_241));
        const { default: __VLS_245 } = __VLS_243.slots;
        (__VLS_ctx.sourceLabel(c.source));
        // @ts-ignore
        [avatarColor, sourceTagType, sourceLabel,];
        var __VLS_243;
        if (c.isOwner) {
            let __VLS_246;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_247 = __VLS_asFunctionalComponent1(__VLS_246, new __VLS_246({
                size: "small",
                type: "warning",
                ...{ style: {} },
            }));
            const __VLS_248 = __VLS_247({
                size: "small",
                type: "warning",
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_247));
            const { default: __VLS_251 } = __VLS_249.slots;
            // @ts-ignore
            [];
            var __VLS_249;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-meta" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-meta']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "contact-id" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-id']} */ ;
        (c.id);
        if (c.tags && c.tags.length) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "contact-tags" },
            });
            /** @type {__VLS_StyleScopedClasses['contact-tags']} */ ;
            for (const [t] of __VLS_vFor((c.tags))) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    key: (t),
                    ...{ class: "contact-tag" },
                });
                /** @type {__VLS_StyleScopedClasses['contact-tag']} */ ;
                (t);
                // @ts-ignore
                [];
            }
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "contact-msgcount" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-msgcount']} */ ;
        (c.msgCount);
        if (c.lastSeenAt) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "contact-lastseen" },
            });
            /** @type {__VLS_StyleScopedClasses['contact-lastseen']} */ ;
            (__VLS_ctx.formatLastSeen(c.lastSeenAt));
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-agent-chip" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-agent-chip']} */ ;
        let __VLS_252;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_253 = __VLS_asFunctionalComponent1(__VLS_252, new __VLS_252({
            type: "info",
            size: "small",
        }));
        const __VLS_254 = __VLS_253({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_253));
        const { default: __VLS_257 } = __VLS_255.slots;
        (__VLS_ctx.agentNameById[c.agentId] || c.agentId);
        // @ts-ignore
        [formatLastSeen, agentNameById,];
        var __VLS_255;
        // @ts-ignore
        [];
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.subTab === 'chats') }, null, null);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "contact-filter-bar" },
});
/** @type {__VLS_StyleScopedClasses['contact-filter-bar']} */ ;
let __VLS_258;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input'] | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_259 = __VLS_asFunctionalComponent1(__VLS_258, new __VLS_258({
    modelValue: (__VLS_ctx.chatSearch),
    placeholder: "搜索 群名 / ID / 类型 / 标签 / 来源",
    size: "default",
    ...{ style: {} },
    clearable: true,
}));
const __VLS_260 = __VLS_259({
    modelValue: (__VLS_ctx.chatSearch),
    placeholder: "搜索 群名 / ID / 类型 / 标签 / 来源",
    size: "default",
    ...{ style: {} },
    clearable: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_259));
const { default: __VLS_263 } = __VLS_261.slots;
{
    const { prefix: __VLS_264 } = __VLS_261.slots;
    let __VLS_265;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_266 = __VLS_asFunctionalComponent1(__VLS_265, new __VLS_265({}));
    const __VLS_267 = __VLS_266({}, ...__VLS_functionalComponentArgsRest(__VLS_266));
    const { default: __VLS_270 } = __VLS_268.slots;
    let __VLS_271;
    /** @ts-ignore @type { | typeof __VLS_components.Search} */
    Search;
    // @ts-ignore
    const __VLS_272 = __VLS_asFunctionalComponent1(__VLS_271, new __VLS_271({}));
    const __VLS_273 = __VLS_272({}, ...__VLS_functionalComponentArgsRest(__VLS_272));
    // @ts-ignore
    [subTab, chatSearch,];
    var __VLS_268;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_261;
let __VLS_276;
/** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
elRadioGroup;
// @ts-ignore
const __VLS_277 = __VLS_asFunctionalComponent1(__VLS_276, new __VLS_276({
    modelValue: (__VLS_ctx.chatSource),
    size: "small",
}));
const __VLS_278 = __VLS_277({
    modelValue: (__VLS_ctx.chatSource),
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_277));
const { default: __VLS_281 } = __VLS_279.slots;
let __VLS_282;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_283 = __VLS_asFunctionalComponent1(__VLS_282, new __VLS_282({
    value: "",
}));
const __VLS_284 = __VLS_283({
    value: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_283));
const { default: __VLS_287 } = __VLS_285.slots;
// @ts-ignore
[chatSource,];
var __VLS_285;
let __VLS_288;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_289 = __VLS_asFunctionalComponent1(__VLS_288, new __VLS_288({
    value: "feishu",
}));
const __VLS_290 = __VLS_289({
    value: "feishu",
}, ...__VLS_functionalComponentArgsRest(__VLS_289));
const { default: __VLS_293 } = __VLS_291.slots;
// @ts-ignore
[];
var __VLS_291;
let __VLS_294;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_295 = __VLS_asFunctionalComponent1(__VLS_294, new __VLS_294({
    value: "telegram",
}));
const __VLS_296 = __VLS_295({
    value: "telegram",
}, ...__VLS_functionalComponentArgsRest(__VLS_295));
const { default: __VLS_299 } = __VLS_297.slots;
// @ts-ignore
[];
var __VLS_297;
// @ts-ignore
[];
var __VLS_279;
let __VLS_300;
/** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
elRadioGroup;
// @ts-ignore
const __VLS_301 = __VLS_asFunctionalComponent1(__VLS_300, new __VLS_300({
    modelValue: (__VLS_ctx.chatAgentFilter),
    size: "small",
}));
const __VLS_302 = __VLS_301({
    modelValue: (__VLS_ctx.chatAgentFilter),
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_301));
const { default: __VLS_305 } = __VLS_303.slots;
let __VLS_306;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_307 = __VLS_asFunctionalComponent1(__VLS_306, new __VLS_306({
    value: "",
}));
const __VLS_308 = __VLS_307({
    value: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_307));
const { default: __VLS_311 } = __VLS_309.slots;
// @ts-ignore
[chatAgentFilter,];
var __VLS_309;
for (const [ag] of __VLS_vFor((__VLS_ctx.graph.nodes))) {
    let __VLS_312;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_313 = __VLS_asFunctionalComponent1(__VLS_312, new __VLS_312({
        key: (ag.id),
        value: (ag.id),
    }));
    const __VLS_314 = __VLS_313({
        key: (ag.id),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_313));
    const { default: __VLS_317 } = __VLS_315.slots;
    (ag.name);
    // @ts-ignore
    [graph,];
    var __VLS_315;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_303;
if (!__VLS_ctx.chatsLoading && !__VLS_ctx.filteredChats.length) {
    let __VLS_318;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_319 = __VLS_asFunctionalComponent1(__VLS_318, new __VLS_318({
        ...{ class: "contacts-empty" },
    }));
    const __VLS_320 = __VLS_319({
        ...{ class: "contacts-empty" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_319));
    /** @type {__VLS_StyleScopedClasses['contacts-empty']} */ ;
    const { default: __VLS_323 } = __VLS_321.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    (__VLS_ctx.chats.length ? '当前筛选无结果' : '还没有群档案。AI 在飞书/TG 群聊中收到第一条消息时自动创建。');
    // @ts-ignore
    [chatsLoading, filteredChats, chats,];
    var __VLS_321;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "contact-list" },
    });
    __VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.chatsLoading) }, null, null);
    /** @type {__VLS_StyleScopedClasses['contact-list']} */ ;
    for (const [c] of __VLS_vFor((__VLS_ctx.filteredChats))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.chatsLoading && !__VLS_ctx.filteredChats.length))
                        return;
                    __VLS_ctx.openChatDrawer(c);
                    // @ts-ignore
                    [vLoading, chatsLoading, filteredChats, openChatDrawer,];
                } },
            key: (c.agentId + '|' + c.id),
            ...{ class: "contact-row" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-avatar" },
            ...{ style: ({ background: __VLS_ctx.avatarColor(c.title || c.id) }) },
        });
        /** @type {__VLS_StyleScopedClasses['contact-avatar']} */ ;
        ((c.title || c.id).slice(0, 1));
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-main" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-main']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-name-row" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-name-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "contact-name" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-name']} */ ;
        (c.title || c.id);
        let __VLS_324;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_325 = __VLS_asFunctionalComponent1(__VLS_324, new __VLS_324({
            size: "small",
            type: (__VLS_ctx.sourceTagType(c.source)),
            effect: "plain",
            ...{ style: {} },
        }));
        const __VLS_326 = __VLS_325({
            size: "small",
            type: (__VLS_ctx.sourceTagType(c.source)),
            effect: "plain",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_325));
        const { default: __VLS_329 } = __VLS_327.slots;
        (__VLS_ctx.sourceLabel(c.source));
        // @ts-ignore
        [avatarColor, sourceTagType, sourceLabel,];
        var __VLS_327;
        if (c.kind) {
            let __VLS_330;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_331 = __VLS_asFunctionalComponent1(__VLS_330, new __VLS_330({
                size: "small",
                type: "info",
                effect: "plain",
                ...{ style: {} },
            }));
            const __VLS_332 = __VLS_331({
                size: "small",
                type: "info",
                effect: "plain",
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_331));
            const { default: __VLS_335 } = __VLS_333.slots;
            (__VLS_ctx.chatKindLabel(c.kind));
            // @ts-ignore
            [chatKindLabel,];
            var __VLS_333;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-meta" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-meta']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "contact-id" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-id']} */ ;
        (c.id);
        if (c.tags && c.tags.length) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "contact-tags" },
            });
            /** @type {__VLS_StyleScopedClasses['contact-tags']} */ ;
            for (const [t] of __VLS_vFor((c.tags))) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    key: (t),
                    ...{ class: "contact-tag" },
                });
                /** @type {__VLS_StyleScopedClasses['contact-tag']} */ ;
                (t);
                // @ts-ignore
                [];
            }
        }
        if (c.memberCount && c.memberCount > 0) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "contact-msgcount" },
            });
            /** @type {__VLS_StyleScopedClasses['contact-msgcount']} */ ;
            (c.memberCount);
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "contact-msgcount" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-msgcount']} */ ;
        (c.msgCount);
        if (c.lastSeenAt) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "contact-lastseen" },
            });
            /** @type {__VLS_StyleScopedClasses['contact-lastseen']} */ ;
            (__VLS_ctx.formatLastSeen(c.lastSeenAt));
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "contact-agent-chip" },
        });
        /** @type {__VLS_StyleScopedClasses['contact-agent-chip']} */ ;
        let __VLS_336;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_337 = __VLS_asFunctionalComponent1(__VLS_336, new __VLS_336({
            type: "info",
            size: "small",
        }));
        const __VLS_338 = __VLS_337({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_337));
        const { default: __VLS_341 } = __VLS_339.slots;
        (__VLS_ctx.agentNameById[c.agentId] || c.agentId);
        // @ts-ignore
        [formatLastSeen, agentNameById,];
        var __VLS_339;
        // @ts-ignore
        [];
    }
}
let __VLS_342;
/** @ts-ignore @type { | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer'] | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer']} */
elDrawer;
// @ts-ignore
const __VLS_343 = __VLS_asFunctionalComponent1(__VLS_342, new __VLS_342({
    modelValue: (__VLS_ctx.contactDrawerOpen),
    title: (__VLS_ctx.drawerTitle),
    direction: "rtl",
    size: "540px",
    destroyOnClose: true,
}));
const __VLS_344 = __VLS_343({
    modelValue: (__VLS_ctx.contactDrawerOpen),
    title: (__VLS_ctx.drawerTitle),
    direction: "rtl",
    size: "540px",
    destroyOnClose: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_343));
const { default: __VLS_347 } = __VLS_345.slots;
if (__VLS_ctx.drawerContact) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "contact-drawer" },
    });
    /** @type {__VLS_StyleScopedClasses['contact-drawer']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-head" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-head']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-avatar" },
        ...{ style: ({ background: __VLS_ctx.avatarColor(__VLS_ctx.drawerContact.displayName || __VLS_ctx.drawerContact.id) }) },
    });
    /** @type {__VLS_StyleScopedClasses['cd-avatar']} */ ;
    ((__VLS_ctx.drawerContact.displayName || __VLS_ctx.drawerContact.id).slice(0, 1));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-title" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-title']} */ ;
    let __VLS_348;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_349 = __VLS_asFunctionalComponent1(__VLS_348, new __VLS_348({
        modelValue: (__VLS_ctx.drawerContact.displayName),
        placeholder: "显示名",
        size: "default",
        ...{ style: {} },
    }));
    const __VLS_350 = __VLS_349({
        modelValue: (__VLS_ctx.drawerContact.displayName),
        placeholder: "显示名",
        size: "default",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_349));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-sub" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-sub']} */ ;
    let __VLS_353;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_354 = __VLS_asFunctionalComponent1(__VLS_353, new __VLS_353({
        size: "small",
        type: (__VLS_ctx.sourceTagType(__VLS_ctx.drawerContact.source)),
        effect: "plain",
    }));
    const __VLS_355 = __VLS_354({
        size: "small",
        type: (__VLS_ctx.sourceTagType(__VLS_ctx.drawerContact.source)),
        effect: "plain",
    }, ...__VLS_functionalComponentArgsRest(__VLS_354));
    const { default: __VLS_358 } = __VLS_356.slots;
    (__VLS_ctx.sourceLabel(__VLS_ctx.drawerContact.source));
    // @ts-ignore
    [avatarColor, sourceTagType, sourceLabel, contactDrawerOpen, drawerTitle, drawerContact, drawerContact, drawerContact, drawerContact, drawerContact, drawerContact, drawerContact, drawerContact,];
    var __VLS_356;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "cd-id" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-id']} */ ;
    (__VLS_ctx.drawerContact.id);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-field" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-tags" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-tags']} */ ;
    for (const [t, i] of __VLS_vFor(((__VLS_ctx.drawerContact.tags || [])))) {
        let __VLS_359;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_360 = __VLS_asFunctionalComponent1(__VLS_359, new __VLS_359({
            ...{ 'onClose': {} },
            key: (t + i),
            closable: true,
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_361 = __VLS_360({
            ...{ 'onClose': {} },
            key: (t + i),
            closable: true,
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_360));
        let __VLS_364;
        const __VLS_365 = ({ close: {} },
            { onClose: (...[$event]) => {
                    if (!(__VLS_ctx.drawerContact))
                        return;
                    (__VLS_ctx.drawerContact.tags || []).splice(i, 1);
                    // @ts-ignore
                    [drawerContact, drawerContact, drawerContact,];
                } });
        const { default: __VLS_366 } = __VLS_362.slots;
        (t);
        // @ts-ignore
        [];
        var __VLS_362;
        var __VLS_363;
        // @ts-ignore
        [];
    }
    if (__VLS_ctx.addingTag) {
        let __VLS_367;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_368 = __VLS_asFunctionalComponent1(__VLS_367, new __VLS_367({
            ...{ 'onKeyup': {} },
            ...{ 'onBlur': {} },
            modelValue: (__VLS_ctx.newTagText),
            size: "small",
            ...{ style: {} },
            placeholder: "回车确认",
            ref: "tagInputRef",
        }));
        const __VLS_369 = __VLS_368({
            ...{ 'onKeyup': {} },
            ...{ 'onBlur': {} },
            modelValue: (__VLS_ctx.newTagText),
            size: "small",
            ...{ style: {} },
            placeholder: "回车确认",
            ref: "tagInputRef",
        }, ...__VLS_functionalComponentArgsRest(__VLS_368));
        let __VLS_372;
        const __VLS_373 = ({ keyup: {} },
            { onKeyup: (__VLS_ctx.commitTag) });
        const __VLS_374 = ({ blur: {} },
            { onBlur: (__VLS_ctx.commitTag) });
        var __VLS_375 = {};
        var __VLS_370;
        var __VLS_371;
    }
    else {
        let __VLS_377;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_378 = __VLS_asFunctionalComponent1(__VLS_377, new __VLS_377({
            ...{ 'onClick': {} },
            size: "small",
        }));
        const __VLS_379 = __VLS_378({
            ...{ 'onClick': {} },
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_378));
        let __VLS_382;
        const __VLS_383 = ({ click: {} },
            { onClick: (__VLS_ctx.beginAddTag) });
        const { default: __VLS_384 } = __VLS_380.slots;
        // @ts-ignore
        [addingTag, newTagText, commitTag, commitTag, beginAddTag,];
        var __VLS_380;
        var __VLS_381;
    }
    for (const [preset] of __VLS_vFor((__VLS_ctx.presetTags))) {
        let __VLS_385;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_386 = __VLS_asFunctionalComponent1(__VLS_385, new __VLS_385({
            ...{ 'onClick': {} },
            key: (preset),
            size: "small",
            plain: true,
            ...{ style: {} },
        }));
        const __VLS_387 = __VLS_386({
            ...{ 'onClick': {} },
            key: (preset),
            size: "small",
            plain: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_386));
        let __VLS_390;
        const __VLS_391 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.drawerContact))
                        return;
                    __VLS_ctx.addPresetTag(preset);
                    // @ts-ignore
                    [presetTags, addPresetTag,];
                } });
        const { default: __VLS_392 } = __VLS_388.slots;
        (preset);
        // @ts-ignore
        [];
        var __VLS_388;
        var __VLS_389;
        // @ts-ignore
        [];
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-field" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
    let __VLS_393;
    /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox'] | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
    elCheckbox;
    // @ts-ignore
    const __VLS_394 = __VLS_asFunctionalComponent1(__VLS_393, new __VLS_393({
        modelValue: (__VLS_ctx.drawerContact.isOwner),
    }));
    const __VLS_395 = __VLS_394({
        modelValue: (__VLS_ctx.drawerContact.isOwner),
    }, ...__VLS_functionalComponentArgsRest(__VLS_394));
    const { default: __VLS_398 } = __VLS_396.slots;
    // @ts-ignore
    [drawerContact,];
    var __VLS_396;
    let __VLS_399;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_400 = __VLS_asFunctionalComponent1(__VLS_399, new __VLS_399({
        type: "info",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_401 = __VLS_400({
        type: "info",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_400));
    const { default: __VLS_404 } = __VLS_402.slots;
    // @ts-ignore
    [];
    var __VLS_402;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-field" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({});
    let __VLS_405;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_406 = __VLS_asFunctionalComponent1(__VLS_405, new __VLS_405({
        modelValue: (__VLS_ctx.drawerContact.body),
        type: "textarea",
        rows: (12),
        placeholder: "# 姓名&#10;&#10;## 事实&#10;- 公司/角色&#10;- ...&#10;&#10;## 偏好（AI 观察）&#10;-&#10;&#10;## 待跟进&#10;-",
        ...{ style: {} },
    }));
    const __VLS_407 = __VLS_406({
        modelValue: (__VLS_ctx.drawerContact.body),
        type: "textarea",
        rows: (12),
        placeholder: "# 姓名&#10;&#10;## 事实&#10;- 公司/角色&#10;- ...&#10;&#10;## 偏好（AI 观察）&#10;-&#10;&#10;## 待跟进&#10;-",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_406));
    let __VLS_410;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_411 = __VLS_asFunctionalComponent1(__VLS_410, new __VLS_410({
        type: "info",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_412 = __VLS_411({
        type: "info",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_411));
    const { default: __VLS_415 } = __VLS_413.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
    // @ts-ignore
    [drawerContact,];
    var __VLS_413;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-actions" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-actions']} */ ;
    let __VLS_416;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_417 = __VLS_asFunctionalComponent1(__VLS_416, new __VLS_416({
        ...{ 'onClick': {} },
    }));
    const __VLS_418 = __VLS_417({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_417));
    let __VLS_421;
    const __VLS_422 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.drawerContact))
                    return;
                __VLS_ctx.contactDrawerOpen = false;
                // @ts-ignore
                [contactDrawerOpen,];
            } });
    const { default: __VLS_423 } = __VLS_419.slots;
    // @ts-ignore
    [];
    var __VLS_419;
    var __VLS_420;
    let __VLS_424;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_425 = __VLS_asFunctionalComponent1(__VLS_424, new __VLS_424({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
        loading: (__VLS_ctx.drawerSaving),
    }));
    const __VLS_426 = __VLS_425({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
        loading: (__VLS_ctx.drawerSaving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_425));
    let __VLS_429;
    const __VLS_430 = ({ click: {} },
        { onClick: (__VLS_ctx.deleteContact) });
    const { default: __VLS_431 } = __VLS_427.slots;
    // @ts-ignore
    [drawerSaving, deleteContact,];
    var __VLS_427;
    var __VLS_428;
    let __VLS_432;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_433 = __VLS_asFunctionalComponent1(__VLS_432, new __VLS_432({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.drawerSaving),
    }));
    const __VLS_434 = __VLS_433({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.drawerSaving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_433));
    let __VLS_437;
    const __VLS_438 = ({ click: {} },
        { onClick: (__VLS_ctx.saveContact) });
    const { default: __VLS_439 } = __VLS_435.slots;
    // @ts-ignore
    [drawerSaving, saveContact,];
    var __VLS_435;
    var __VLS_436;
}
// @ts-ignore
[];
var __VLS_345;
let __VLS_440;
/** @ts-ignore @type { | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer'] | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer']} */
elDrawer;
// @ts-ignore
const __VLS_441 = __VLS_asFunctionalComponent1(__VLS_440, new __VLS_440({
    modelValue: (__VLS_ctx.chatDrawerOpen),
    title: (__VLS_ctx.chatDrawerTitle),
    direction: "rtl",
    size: "540px",
    destroyOnClose: true,
}));
const __VLS_442 = __VLS_441({
    modelValue: (__VLS_ctx.chatDrawerOpen),
    title: (__VLS_ctx.chatDrawerTitle),
    direction: "rtl",
    size: "540px",
    destroyOnClose: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_441));
const { default: __VLS_445 } = __VLS_443.slots;
if (__VLS_ctx.drawerChat) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "contact-drawer" },
    });
    /** @type {__VLS_StyleScopedClasses['contact-drawer']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-head" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-head']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-avatar" },
        ...{ style: ({ background: __VLS_ctx.avatarColor(__VLS_ctx.drawerChat.title || __VLS_ctx.drawerChat.id) }) },
    });
    /** @type {__VLS_StyleScopedClasses['cd-avatar']} */ ;
    ((__VLS_ctx.drawerChat.title || __VLS_ctx.drawerChat.id).slice(0, 1));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-title" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-title']} */ ;
    let __VLS_446;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_447 = __VLS_asFunctionalComponent1(__VLS_446, new __VLS_446({
        modelValue: (__VLS_ctx.drawerChat.title),
        placeholder: "群名（飞书消息事件不带群名时为空，可在此填）",
        size: "default",
        ...{ style: {} },
    }));
    const __VLS_448 = __VLS_447({
        modelValue: (__VLS_ctx.drawerChat.title),
        placeholder: "群名（飞书消息事件不带群名时为空，可在此填）",
        size: "default",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_447));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-sub" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-sub']} */ ;
    let __VLS_451;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_452 = __VLS_asFunctionalComponent1(__VLS_451, new __VLS_451({
        size: "small",
        type: (__VLS_ctx.sourceTagType(__VLS_ctx.drawerChat.source)),
        effect: "plain",
    }));
    const __VLS_453 = __VLS_452({
        size: "small",
        type: (__VLS_ctx.sourceTagType(__VLS_ctx.drawerChat.source)),
        effect: "plain",
    }, ...__VLS_functionalComponentArgsRest(__VLS_452));
    const { default: __VLS_456 } = __VLS_454.slots;
    (__VLS_ctx.sourceLabel(__VLS_ctx.drawerChat.source));
    // @ts-ignore
    [avatarColor, sourceTagType, sourceLabel, chatDrawerOpen, chatDrawerTitle, drawerChat, drawerChat, drawerChat, drawerChat, drawerChat, drawerChat, drawerChat, drawerChat,];
    var __VLS_454;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "cd-id" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-id']} */ ;
    (__VLS_ctx.drawerChat.id);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-field" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({});
    let __VLS_457;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_458 = __VLS_asFunctionalComponent1(__VLS_457, new __VLS_457({
        modelValue: (__VLS_ctx.drawerChat.kind),
        placeholder: "选择群类型",
        ...{ style: {} },
    }));
    const __VLS_459 = __VLS_458({
        modelValue: (__VLS_ctx.drawerChat.kind),
        placeholder: "选择群类型",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_458));
    const { default: __VLS_462 } = __VLS_460.slots;
    let __VLS_463;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_464 = __VLS_asFunctionalComponent1(__VLS_463, new __VLS_463({
        label: "群组 (group)",
        value: "group",
    }));
    const __VLS_465 = __VLS_464({
        label: "群组 (group)",
        value: "group",
    }, ...__VLS_functionalComponentArgsRest(__VLS_464));
    let __VLS_468;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_469 = __VLS_asFunctionalComponent1(__VLS_468, new __VLS_468({
        label: "超级群 (supergroup)",
        value: "supergroup",
    }));
    const __VLS_470 = __VLS_469({
        label: "超级群 (supergroup)",
        value: "supergroup",
    }, ...__VLS_functionalComponentArgsRest(__VLS_469));
    let __VLS_473;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_474 = __VLS_asFunctionalComponent1(__VLS_473, new __VLS_473({
        label: "频道 (channel)",
        value: "channel",
    }));
    const __VLS_475 = __VLS_474({
        label: "频道 (channel)",
        value: "channel",
    }, ...__VLS_functionalComponentArgsRest(__VLS_474));
    let __VLS_478;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_479 = __VLS_asFunctionalComponent1(__VLS_478, new __VLS_478({
        label: "私聊 (private)",
        value: "private",
    }));
    const __VLS_480 = __VLS_479({
        label: "私聊 (private)",
        value: "private",
    }, ...__VLS_functionalComponentArgsRest(__VLS_479));
    // @ts-ignore
    [drawerChat, drawerChat,];
    var __VLS_460;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-field" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({});
    let __VLS_483;
    /** @ts-ignore @type { | typeof __VLS_components.elInputNumber | typeof __VLS_components.ElInputNumber | typeof __VLS_components['el-input-number']} */
    elInputNumber;
    // @ts-ignore
    const __VLS_484 = __VLS_asFunctionalComponent1(__VLS_483, new __VLS_483({
        modelValue: (__VLS_ctx.drawerChat.memberCount),
        min: (0),
        max: (100000),
        size: "small",
    }));
    const __VLS_485 = __VLS_484({
        modelValue: (__VLS_ctx.drawerChat.memberCount),
        min: (0),
        max: (100000),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_484));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-field" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-tags" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-tags']} */ ;
    for (const [t, i] of __VLS_vFor(((__VLS_ctx.drawerChat.tags || [])))) {
        let __VLS_488;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_489 = __VLS_asFunctionalComponent1(__VLS_488, new __VLS_488({
            ...{ 'onClose': {} },
            key: (t + i),
            closable: true,
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_490 = __VLS_489({
            ...{ 'onClose': {} },
            key: (t + i),
            closable: true,
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_489));
        let __VLS_493;
        const __VLS_494 = ({ close: {} },
            { onClose: (...[$event]) => {
                    if (!(__VLS_ctx.drawerChat))
                        return;
                    (__VLS_ctx.drawerChat.tags || []).splice(i, 1);
                    // @ts-ignore
                    [drawerChat, drawerChat, drawerChat,];
                } });
        const { default: __VLS_495 } = __VLS_491.slots;
        (t);
        // @ts-ignore
        [];
        var __VLS_491;
        var __VLS_492;
        // @ts-ignore
        [];
    }
    if (__VLS_ctx.addingChatTag) {
        let __VLS_496;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_497 = __VLS_asFunctionalComponent1(__VLS_496, new __VLS_496({
            ...{ 'onKeyup': {} },
            ...{ 'onBlur': {} },
            modelValue: (__VLS_ctx.newChatTagText),
            size: "small",
            ...{ style: {} },
            placeholder: "回车确认",
            ref: "chatTagInputRef",
        }));
        const __VLS_498 = __VLS_497({
            ...{ 'onKeyup': {} },
            ...{ 'onBlur': {} },
            modelValue: (__VLS_ctx.newChatTagText),
            size: "small",
            ...{ style: {} },
            placeholder: "回车确认",
            ref: "chatTagInputRef",
        }, ...__VLS_functionalComponentArgsRest(__VLS_497));
        let __VLS_501;
        const __VLS_502 = ({ keyup: {} },
            { onKeyup: (__VLS_ctx.commitChatTag) });
        const __VLS_503 = ({ blur: {} },
            { onBlur: (__VLS_ctx.commitChatTag) });
        var __VLS_504 = {};
        var __VLS_499;
        var __VLS_500;
    }
    else {
        let __VLS_506;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_507 = __VLS_asFunctionalComponent1(__VLS_506, new __VLS_506({
            ...{ 'onClick': {} },
            size: "small",
        }));
        const __VLS_508 = __VLS_507({
            ...{ 'onClick': {} },
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_507));
        let __VLS_511;
        const __VLS_512 = ({ click: {} },
            { onClick: (__VLS_ctx.beginAddChatTag) });
        const { default: __VLS_513 } = __VLS_509.slots;
        // @ts-ignore
        [addingChatTag, newChatTagText, commitChatTag, commitChatTag, beginAddChatTag,];
        var __VLS_509;
        var __VLS_510;
    }
    for (const [preset] of __VLS_vFor((__VLS_ctx.presetChatTags))) {
        let __VLS_514;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_515 = __VLS_asFunctionalComponent1(__VLS_514, new __VLS_514({
            ...{ 'onClick': {} },
            key: (preset),
            size: "small",
            plain: true,
            ...{ style: {} },
        }));
        const __VLS_516 = __VLS_515({
            ...{ 'onClick': {} },
            key: (preset),
            size: "small",
            plain: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_515));
        let __VLS_519;
        const __VLS_520 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.drawerChat))
                        return;
                    __VLS_ctx.addPresetChatTag(preset);
                    // @ts-ignore
                    [presetChatTags, addPresetChatTag,];
                } });
        const { default: __VLS_521 } = __VLS_517.slots;
        (preset);
        // @ts-ignore
        [];
        var __VLS_517;
        var __VLS_518;
        // @ts-ignore
        [];
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-field" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-field']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({});
    let __VLS_522;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_523 = __VLS_asFunctionalComponent1(__VLS_522, new __VLS_522({
        modelValue: (__VLS_ctx.drawerChat.body),
        type: "textarea",
        rows: (14),
        placeholder: "# 群名&#10;&#10;## 基础信息&#10;- 群创建于 ...&#10;- 群主：...&#10;&#10;## 群规则&#10;-&#10;&#10;## 重要议题&#10;-&#10;&#10;## 待跟进&#10;-",
        ...{ style: {} },
    }));
    const __VLS_524 = __VLS_523({
        modelValue: (__VLS_ctx.drawerChat.body),
        type: "textarea",
        rows: (14),
        placeholder: "# 群名&#10;&#10;## 基础信息&#10;- 群创建于 ...&#10;- 群主：...&#10;&#10;## 群规则&#10;-&#10;&#10;## 重要议题&#10;-&#10;&#10;## 待跟进&#10;-",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_523));
    let __VLS_527;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_528 = __VLS_asFunctionalComponent1(__VLS_527, new __VLS_527({
        type: "info",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_529 = __VLS_528({
        type: "info",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_528));
    const { default: __VLS_532 } = __VLS_530.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
    // @ts-ignore
    [drawerChat,];
    var __VLS_530;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "cd-actions" },
    });
    /** @type {__VLS_StyleScopedClasses['cd-actions']} */ ;
    let __VLS_533;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_534 = __VLS_asFunctionalComponent1(__VLS_533, new __VLS_533({
        ...{ 'onClick': {} },
    }));
    const __VLS_535 = __VLS_534({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_534));
    let __VLS_538;
    const __VLS_539 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.drawerChat))
                    return;
                __VLS_ctx.chatDrawerOpen = false;
                // @ts-ignore
                [chatDrawerOpen,];
            } });
    const { default: __VLS_540 } = __VLS_536.slots;
    // @ts-ignore
    [];
    var __VLS_536;
    var __VLS_537;
    let __VLS_541;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_542 = __VLS_asFunctionalComponent1(__VLS_541, new __VLS_541({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
        loading: (__VLS_ctx.chatDrawerSaving),
    }));
    const __VLS_543 = __VLS_542({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
        loading: (__VLS_ctx.chatDrawerSaving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_542));
    let __VLS_546;
    const __VLS_547 = ({ click: {} },
        { onClick: (__VLS_ctx.deleteChat) });
    const { default: __VLS_548 } = __VLS_544.slots;
    // @ts-ignore
    [chatDrawerSaving, deleteChat,];
    var __VLS_544;
    var __VLS_545;
    let __VLS_549;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_550 = __VLS_asFunctionalComponent1(__VLS_549, new __VLS_549({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.chatDrawerSaving),
    }));
    const __VLS_551 = __VLS_550({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.chatDrawerSaving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_550));
    let __VLS_554;
    const __VLS_555 = ({ click: {} },
        { onClick: (__VLS_ctx.saveChat) });
    const { default: __VLS_556 } = __VLS_552.slots;
    // @ts-ignore
    [chatDrawerSaving, saveChat,];
    var __VLS_552;
    var __VLS_553;
}
// @ts-ignore
[];
var __VLS_443;
let __VLS_557;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_558 = __VLS_asFunctionalComponent1(__VLS_557, new __VLS_557({
    modelValue: (__VLS_ctx.createRelDialog),
    title: "建立关系",
    width: "460px",
    closeOnClickModal: (false),
}));
const __VLS_559 = __VLS_558({
    modelValue: (__VLS_ctx.createRelDialog),
    title: "建立关系",
    width: "460px",
    closeOnClickModal: (false),
}, ...__VLS_functionalComponentArgsRest(__VLS_558));
const { default: __VLS_562 } = __VLS_560.slots;
const __VLS_563 = RelTypeForm;
// @ts-ignore
const __VLS_564 = __VLS_asFunctionalComponent1(__VLS_563, new __VLS_563({
    ...{ 'onSwap': {} },
    fromName: (__VLS_ctx.nodeName(__VLS_ctx.relForm.from)),
    toName: (__VLS_ctx.nodeName(__VLS_ctx.relForm.to)),
    type: (__VLS_ctx.relForm.type),
    strength: (__VLS_ctx.relForm.strength),
    desc: (__VLS_ctx.relForm.desc),
}));
const __VLS_565 = __VLS_564({
    ...{ 'onSwap': {} },
    fromName: (__VLS_ctx.nodeName(__VLS_ctx.relForm.from)),
    toName: (__VLS_ctx.nodeName(__VLS_ctx.relForm.to)),
    type: (__VLS_ctx.relForm.type),
    strength: (__VLS_ctx.relForm.strength),
    desc: (__VLS_ctx.relForm.desc),
}, ...__VLS_functionalComponentArgsRest(__VLS_564));
let __VLS_568;
const __VLS_569 = ({ swap: {} },
    { onSwap: (() => { const t = __VLS_ctx.relForm.from; __VLS_ctx.relForm.from = __VLS_ctx.relForm.to; __VLS_ctx.relForm.to = t; }) });
var __VLS_566;
var __VLS_567;
{
    const { footer: __VLS_570 } = __VLS_560.slots;
    let __VLS_571;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_572 = __VLS_asFunctionalComponent1(__VLS_571, new __VLS_571({
        ...{ 'onClick': {} },
    }));
    const __VLS_573 = __VLS_572({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_572));
    let __VLS_576;
    const __VLS_577 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.createRelDialog = false;
                // @ts-ignore
                [nodeName, nodeName, createRelDialog, createRelDialog, relForm, relForm, relForm, relForm, relForm, relForm, relForm, relForm, relForm,];
            } });
    const { default: __VLS_578 } = __VLS_574.slots;
    // @ts-ignore
    [];
    var __VLS_574;
    var __VLS_575;
    let __VLS_579;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_580 = __VLS_asFunctionalComponent1(__VLS_579, new __VLS_579({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.savingRel),
    }));
    const __VLS_581 = __VLS_580({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.savingRel),
    }, ...__VLS_functionalComponentArgsRest(__VLS_580));
    let __VLS_584;
    const __VLS_585 = ({ click: {} },
        { onClick: (__VLS_ctx.saveCreateRel) });
    const { default: __VLS_586 } = __VLS_582.slots;
    // @ts-ignore
    [savingRel, saveCreateRel,];
    var __VLS_582;
    var __VLS_583;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_560;
let __VLS_587;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_588 = __VLS_asFunctionalComponent1(__VLS_587, new __VLS_587({
    modelValue: (__VLS_ctx.editRelDialog),
    title: "编辑关系",
    width: "460px",
    closeOnClickModal: (false),
}));
const __VLS_589 = __VLS_588({
    modelValue: (__VLS_ctx.editRelDialog),
    title: "编辑关系",
    width: "460px",
    closeOnClickModal: (false),
}, ...__VLS_functionalComponentArgsRest(__VLS_588));
const { default: __VLS_592 } = __VLS_590.slots;
const __VLS_593 = RelTypeForm;
// @ts-ignore
const __VLS_594 = __VLS_asFunctionalComponent1(__VLS_593, new __VLS_593({
    ...{ 'onSwap': {} },
    fromName: (__VLS_ctx.nodeName(__VLS_ctx.editForm.from)),
    toName: (__VLS_ctx.nodeName(__VLS_ctx.editForm.to)),
    type: (__VLS_ctx.editForm.type),
    strength: (__VLS_ctx.editForm.strength),
    desc: (__VLS_ctx.editForm.desc),
}));
const __VLS_595 = __VLS_594({
    ...{ 'onSwap': {} },
    fromName: (__VLS_ctx.nodeName(__VLS_ctx.editForm.from)),
    toName: (__VLS_ctx.nodeName(__VLS_ctx.editForm.to)),
    type: (__VLS_ctx.editForm.type),
    strength: (__VLS_ctx.editForm.strength),
    desc: (__VLS_ctx.editForm.desc),
}, ...__VLS_functionalComponentArgsRest(__VLS_594));
let __VLS_598;
const __VLS_599 = ({ swap: {} },
    { onSwap: (() => { const t = __VLS_ctx.editForm.from; __VLS_ctx.editForm.from = __VLS_ctx.editForm.to; __VLS_ctx.editForm.to = t; }) });
var __VLS_596;
var __VLS_597;
{
    const { footer: __VLS_600 } = __VLS_590.slots;
    let __VLS_601;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_602 = __VLS_asFunctionalComponent1(__VLS_601, new __VLS_601({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
        loading: (__VLS_ctx.savingRel),
    }));
    const __VLS_603 = __VLS_602({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
        loading: (__VLS_ctx.savingRel),
    }, ...__VLS_functionalComponentArgsRest(__VLS_602));
    let __VLS_606;
    const __VLS_607 = ({ click: {} },
        { onClick: (__VLS_ctx.confirmDeleteEdge) });
    const { default: __VLS_608 } = __VLS_604.slots;
    // @ts-ignore
    [nodeName, nodeName, savingRel, editRelDialog, editForm, editForm, editForm, editForm, editForm, editForm, editForm, editForm, editForm, confirmDeleteEdge,];
    var __VLS_604;
    var __VLS_605;
    let __VLS_609;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_610 = __VLS_asFunctionalComponent1(__VLS_609, new __VLS_609({
        ...{ 'onClick': {} },
    }));
    const __VLS_611 = __VLS_610({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_610));
    let __VLS_614;
    const __VLS_615 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.editRelDialog = false;
                // @ts-ignore
                [editRelDialog,];
            } });
    const { default: __VLS_616 } = __VLS_612.slots;
    // @ts-ignore
    [];
    var __VLS_612;
    var __VLS_613;
    let __VLS_617;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_618 = __VLS_asFunctionalComponent1(__VLS_617, new __VLS_617({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.savingRel),
    }));
    const __VLS_619 = __VLS_618({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.savingRel),
    }, ...__VLS_functionalComponentArgsRest(__VLS_618));
    let __VLS_622;
    const __VLS_623 = ({ click: {} },
        { onClick: (__VLS_ctx.saveEditRel) });
    const { default: __VLS_624 } = __VLS_620.slots;
    // @ts-ignore
    [savingRel, saveEditRel,];
    var __VLS_620;
    var __VLS_621;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_590;
// @ts-ignore
var __VLS_376 = __VLS_375, __VLS_505 = __VLS_504;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
