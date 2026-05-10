/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, reactive, nextTick, onMounted, onUnmounted, watch } from 'vue';
import { chatSSE, resumeSSE, getSessionStatus, sessions as sessionsApi, tasks as tasksApi } from '../api';
import DispatchPanel from './DispatchPanel.vue';
const props = withDefaults(defineProps(), {
    examples: () => [],
    showThinking: false,
    compact: false,
    applyable: false,
});
const emit = defineEmits();
// ── State ─────────────────────────────────────────────────────────────────
const messages = ref(props.initialMessages ? [...props.initialMessages] : []);
const inputText = ref('');
const pendingImages = ref([]);
const pendingFiles = ref([]);
// ── 档位 chip（极简版：5 个硬编码，追加 hashtag 到输入末尾；system prompt 约定识别） ──
const modeChips = [
    { tag: '#简答', hint: '只给结论，不啰嗦' },
    { tag: '#深思考', hint: '多步推理，展示思路' },
    { tag: '#写代码', hint: '聚焦代码实现' },
    { tag: '#闲聊', hint: '轻松聊天，不必严谨' },
    { tag: '#急', hint: '优先给最快的解决方案' },
];
function appendModeChip(tag) {
    inputText.value = (inputText.value ? inputText.value.trim() + ' ' : '') + tag + ' ';
    nextTick(() => {
        const el = inputRef.value;
        if (el) {
            el.focus();
            el.setSelectionRange(el.value.length, el.value.length);
            autoGrow();
        }
    });
}
const streaming = ref(false);
watch(streaming, (v) => emit('streaming-change', v));
// #14 fix: track user scroll intention during streaming
const userScrolledUp = ref(false);
const streamText = ref('');
const streamThinking = ref('');
const streamToolCalls = ref([]); // active tool calls during streaming
const activeFence = ref(null);
function abortActiveStream(reason = 'aborted') {
    const f = activeFence.value;
    if (!f || f.aborted)
        return;
    f.aborted = true;
    try {
        f.ctrl?.abort();
    }
    catch { /* already aborted */ }
    void reason;
}
// ── Background task tracking (agent_spawn) ─────────────────────────────────
// Maps toolCallId → background taskId for live status polling (current session)
const spawnedTaskMap = reactive(new Map());
// Tasks re-attached after page reload (no tool call card, just status tracking)
const resumedTasks = ref([]);
let taskPollTimer = null;
// Elapsed time ticker — incremented every second while tasks are running
const elapsedTick = ref(0);
let elapsedTimer = null;
const runningTaskCount = computed(() => {
    let count = 0;
    for (const msg of messages.value) {
        for (const tc of msg.toolCalls ?? []) {
            if (tc.taskId && tc.taskStatus && !['done', 'error', 'killed'].includes(tc.taskStatus))
                count++;
        }
    }
    count += resumedTasks.value.filter(t => !['done', 'error', 'killed'].includes(t.status)).length;
    return count;
});
const isDragOver = ref(false);
let _dragDepth = 0; // counter to handle child element drag enter/leave
const copied = ref(null);
const previewSrc = ref('');
// Session management — server-side persistent history
// Once set, subsequent requests use sessionId instead of sending full history[]
const currentSessionId = ref(props.sessionId);
const historyLoading = ref(false);
const msgListRef = ref();
const inputRef = ref();
const dispatchPanelRef = ref(null);
// ── Computed ──────────────────────────────────────────────────────────────
const rootStyle = computed(() => ({
    height: props.height ?? '100%',
    '--bg': props.bgColor ?? 'transparent',
}));
// ── Helpers ───────────────────────────────────────────────────────────────
// ── System signal detector ─────────────────────────────────────────────────
// Coordinator 模式下，后端会把 <task-notification> XML 以 role=user 写入 session，
// 让 LLM 在下一轮感知到子任务完成。但用户不应在聊天界面看到这种内部协议气泡。
// 命中条件：消息文本 trim 后以 <task-notification> 开头。
function isSystemSignalMsg(text) {
    if (!text)
        return false;
    const t = text.trim();
    return t.startsWith('<task-notification>') || t.startsWith('&lt;task-notification&gt;');
}
// isNetworkLayerError — distinguish transport-level failures (recoverable via
// Broadcaster resume) from business errors (LLM returned 400, tool failed, etc).
// We intentionally match only the most unambiguous network patterns to avoid
// auto-retrying actual LLM-side errors.
function isNetworkLayerError(err) {
    if (!err)
        return false;
    const s = err.toLowerCase();
    // Fetch AbortController → typed as AbortError; we never reach here for those.
    // TypeError: Failed to fetch  (connection dropped / offline)
    if (s.includes('failed to fetch'))
        return true;
    if (s.includes('network error'))
        return true;
    if (s.includes('load failed'))
        return true; // Safari offline
    if (s.includes('err_network_changed'))
        return true;
    if (s.includes('err_internet_disconnected'))
        return true;
    if (s.includes('connection refused'))
        return true;
    if (s.includes('connection reset'))
        return true;
    if (s.includes('socket hang up'))
        return true;
    // HTTP-layer transient: 502/503/504 returned by reverse proxy or LB during restart
    if (/\b50[234]\b/.test(err))
        return true;
    return false;
}
// Maximum reconnect attempts per sendSessionId, then give up and show error.
const MAX_SSE_RECONNECTS = 3;
const sseReconnectCount = new Map();
// appendReconnectNotice — show a transient info system bubble informing the
// user the connection was interrupted and is being restored. Later events from
// the resumed stream will continue to fill the same assistant bubble.
function appendReconnectNotice() {
    // Avoid spamming notices if multiple disconnect events fire in a row.
    const last = messages.value[messages.value.length - 1];
    if (last && last.role === 'system' && last.sysKind === 'info' && last.text.includes('重新连接')) {
        return;
    }
    messages.value.push({
        role: 'system',
        text: '🔄 连接中断，正在尝试重新连接…',
        sysKind: 'info',
    });
}
// reconnectAndResume — re-subscribe to the session Broadcaster and continue
// routing events into the *same* existing assistant bubble (by msgIdx).
// Uses exponential backoff: 1s / 3s / 7s. Gives up after MAX_SSE_RECONNECTS.
function reconnectAndResume(sessionId, msgIdx, sendSessionId) {
    const attempts = (sseReconnectCount.get(sendSessionId) || 0) + 1;
    sseReconnectCount.set(sendSessionId, attempts);
    if (attempts > MAX_SSE_RECONNECTS) {
        const cur = messages.value[msgIdx];
        if (cur) {
            cur.truncatedByError = true;
            messages.value.push({
                role: 'system',
                text: `⚠️ 连接多次中断 (${MAX_SSE_RECONNECTS} 次后放弃)，可重新发送消息继续。`,
                sysKind: 'error',
            });
        }
        streaming.value = false;
        streamText.value = '';
        streamThinking.value = '';
        streamToolCalls.value = [];
        return;
    }
    const backoffMs = [1000, 3000, 7000][attempts - 1] || 7000;
    setTimeout(() => {
        // User may have switched session while waiting → abort
        if (currentSessionId.value !== sendSessionId) {
            streaming.value = false;
            return;
        }
        resumeSSE(props.agentId, sessionId, (ev) => {
            // Session-switch guard
            if (currentSessionId.value !== sendSessionId) {
                streaming.value = false;
                return;
            }
            switch (ev.type) {
                case 'idle':
                    // Server has no active worker for this session — it finished while
                    // we were offline. Treat as silent success; no error surfaced.
                    streaming.value = false;
                    streamText.value = '';
                    streamThinking.value = '';
                    streamToolCalls.value = [];
                    // Remove any stray reconnect-info bubble
                    const lastA = messages.value[messages.value.length - 1];
                    if (lastA?.role === 'system' && lastA.sysKind === 'info') {
                        messages.value.pop();
                    }
                    sseReconnectCount.delete(sendSessionId);
                    break;
                case 'text_delta':
                    streamText.value += ev.text;
                    scrollBottom();
                    break;
                case 'thinking_delta':
                    streamThinking.value += ev.text;
                    scrollBottom();
                    break;
                case 'tool_call':
                case 'tool_result':
                    // Resume path: fold into streamToolCalls same as initial path
                    if (ev.type === 'tool_call') {
                        streamToolCalls.value.push({
                            id: ev.tool_call_id || ev.id,
                            name: ev.name || '',
                            input: ev.input || '',
                            result: '',
                            status: 'running',
                            _expanded: false,
                        });
                    }
                    else {
                        const tc = streamToolCalls.value.find((t) => t.id === (ev.tool_call_id || ev.id));
                        if (tc) {
                            tc.result = ev.result || '';
                            tc.status = 'done';
                        }
                    }
                    scrollBottom();
                    break;
                case 'done':
                case 'error': {
                    // Flush into the existing assistant bubble
                    const cur = messages.value[msgIdx];
                    if (cur) {
                        cur.text = streamText.value;
                        cur.thinking = streamThinking.value || undefined;
                        if (ev.type === 'error') {
                            if (isNetworkLayerError(ev.error)) {
                                // Still flaky — retry
                                appendReconnectNotice();
                                reconnectAndResume(sessionId, msgIdx, sendSessionId);
                                return;
                            }
                            cur.truncatedByError = true;
                            messages.value.push({
                                role: 'system',
                                text: formatErrorMessage(ev.error || '未知错误'),
                                sysKind: 'error',
                            });
                        }
                    }
                    streaming.value = false;
                    streamText.value = '';
                    streamThinking.value = '';
                    streamToolCalls.value = [];
                    sseReconnectCount.delete(sendSessionId);
                    scrollBottom(true);
                    break;
                }
            }
        });
    }, backoffMs);
}
// formatErrorMessage — turn a raw LLM/runner error string into a user-facing
// system bubble. Collapses HTTP status codes to intuitive Chinese phrases,
// trims overly long provider traces, and strips tool-call noise.
// Keep it concise: the user is already looking at the partial answer above.
function formatErrorMessage(raw) {
    const s = (raw || '').trim();
    if (!s)
        return '请求失败';
    const lower = s.toLowerCase();
    if (lower.includes('429') || lower.includes('rate limit') || lower.includes('too many requests')) {
        return '⚠️ 模型请求频率受限 (429)。请稍等片刻再试。';
    }
    if (lower.includes('401') || lower.includes('unauthorized') || lower.includes('invalid api key')) {
        return '⚠️ Provider 认证失败 — 请在「模型」页检查对应 API Key。';
    }
    if (lower.includes('503') || lower.includes('502') || lower.includes('bad gateway') || lower.includes('service unavailable')) {
        return '⚠️ 模型服务暂时不可用 (Provider 端临时故障)。可稍后重试。';
    }
    if (lower.includes('timeout') || lower.includes('deadline exceeded') || lower.includes('timed out')) {
        return '⚠️ 请求超时 — 网络或模型响应过慢。稍后重试即可。';
    }
    if (lower.includes('context length') || lower.includes('context_length_exceeded') || lower.includes('maximum context')) {
        return '⚠️ 对话已超出模型上下文长度，请开启新会话或在设置里启用压缩。';
    }
    if (lower.includes('content filter') || lower.includes('safety') || lower.includes('content_policy')) {
        return '⚠️ 内容被 Provider 安全策略拦截。请调整问法后重试。';
    }
    // Fallback: show the raw message but cap it to 240 chars to avoid a wall-of-text.
    const trimmed = s.length > 240 ? s.slice(0, 240) + '…' : s;
    return `⚠️ 请求失败：${trimmed}`;
}
// ── agent_spawn task polling ────────────────────────────────────────────────
function fmtElapsed(startMs) {
    // depend on elapsedTick so Vue re-renders every second
    void elapsedTick.value;
    const s = Math.floor((Date.now() - startMs) / 1000);
    if (s < 60)
        return `${s}s`;
    return `${Math.floor(s / 60)}m${s % 60}s`;
}
function startTaskPolling() {
    if (taskPollTimer)
        return;
    taskPollTimer = setInterval(pollTasks, 3000);
    if (!elapsedTimer) {
        elapsedTimer = setInterval(() => { elapsedTick.value++; }, 1000);
    }
}
async function pollTasks() {
    const allIdle = spawnedTaskMap.size === 0 &&
        resumedTasks.value.every(t => ['done', 'error', 'killed'].includes(t.status));
    if (allIdle) {
        if (taskPollTimer) {
            clearInterval(taskPollTimer);
            taskPollTimer = null;
        }
        return;
    }
    // Poll tool-call-linked tasks
    const doneIds = [];
    let spawnedJustCompleted = false;
    for (const [tcId, taskId] of spawnedTaskMap) {
        try {
            const res = await tasksApi.get(taskId);
            const info = res.data;
            for (const msg of messages.value) {
                const tc = msg.toolCalls?.find(t => t.id === tcId);
                if (tc) {
                    const wasRunning = !['done', 'error', 'killed'].includes(tc.taskStatus ?? '');
                    const prevStatus = tc.taskStatus;
                    tc.taskStatus = info.status;
                    // Record when task first becomes running
                    if (info.status === 'running' && prevStatus !== 'running' && !tc.taskStartedAt) {
                        tc.taskStartedAt = Date.now();
                    }
                    if (['done', 'error', 'killed'].includes(info.status)) {
                        doneIds.push(tcId);
                        if (wasRunning)
                            spawnedJustCompleted = true;
                    }
                }
            }
        }
        catch {
            doneIds.push(tcId);
            spawnedJustCompleted = true;
        }
    }
    for (const id of doneIds)
        spawnedTaskMap.delete(id);
    // Poll resumed tasks (page-reload re-attached)
    let anyJustCompleted = false;
    for (const rt of resumedTasks.value) {
        if (['done', 'error', 'killed'].includes(rt.status))
            continue;
        try {
            const res = await tasksApi.get(rt.id);
            const prevStatus = rt.status;
            rt.status = res.data.status;
            if (['done', 'error', 'killed'].includes(rt.status) && prevStatus !== rt.status) {
                anyJustCompleted = true;
            }
        }
        catch {
            rt.status = 'error';
            anyJustCompleted = true;
        }
    }
    const stillRunning = spawnedTaskMap.size > 0 ||
        resumedTasks.value.some(t => !['done', 'error', 'killed'].includes(t.status));
    if (!stillRunning && taskPollTimer) {
        clearInterval(taskPollTimer);
        taskPollTimer = null;
    }
    if (!stillRunning && elapsedTimer) {
        clearInterval(elapsedTimer);
        elapsedTimer = null;
    }
    // When any task just completed, reload session messages to pick up the [后台任务完成] notification
    if ((anyJustCompleted || spawnedJustCompleted) && currentSessionId.value && !streaming.value) {
        const sid = currentSessionId.value;
        setTimeout(async () => {
            if (streaming.value)
                return; // don't overwrite mid-stream
            if (currentSessionId.value !== sid)
                return; // stale
            try {
                const res = await sessionsApi.get(props.agentId, sid);
                if (streaming.value)
                    return; // streaming may have started while awaiting
                if (currentSessionId.value !== sid)
                    return;
                const parsed = res.data.messages ?? [];
                const loaded = [];
                if (parsed.some((m) => m.isCompact || m.role === 'compaction')) {
                    loaded.push({ role: 'system', text: '更早的内容已压缩' });
                }
                for (const m of parsed) {
                    if (m.role === 'compaction')
                        continue;
                    if ((m.text?.trim() || (m.toolCalls && m.toolCalls.length > 0)) && !isSystemSignalMsg(m.text))
                        loaded.push({ role: m.role, text: m.text, toolCalls: m.toolCalls?.map((tc) => ({ id: tc.id, name: tc.name, input: tc.input, result: tc.result, status: 'done', _expanded: false, ...processToolResult(tc.result ?? '') })) });
                }
                messages.value = loaded;
                scrollBottom();
                // After reload, trigger LLM to summarize the completed task result
                await nextTick();
                if (!streaming.value && currentSessionId.value === sid) {
                    await sendContinueAfterSpawn();
                }
            }
            catch { }
        }, 1500); // small delay to let server write the notification first
    }
}
// Trigger LLM to report back on just-completed background task
async function sendContinueAfterSpawn() {
    if (streaming.value || !currentSessionId.value)
        return;
    // Check last assistant message — if it already looks like a completion report, skip
    const lastMsg = [...messages.value].reverse().find(m => m.role === 'assistant');
    if (lastMsg?.text && (lastMsg.text.includes('任务完成') ||
        lastMsg.text.includes('已完成') ||
        lastMsg.text.includes('执行完毕') ||
        lastMsg.text.includes('完成了')))
        return;
    // Use runChat in silent mode (no user bubble) with a hidden continue prompt
    runChat('派遣的后台任务已完成，请根据任务结果向我汇报。', [], true);
}
onUnmounted(() => {
    if (taskPollTimer) {
        clearInterval(taskPollTimer);
        taskPollTimer = null;
    }
    if (elapsedTimer) {
        clearInterval(elapsedTimer);
        elapsedTimer = null;
    }
    // P0.4 Abort fence — tear down in-flight SSE on unmount
    abortActiveStream('unmount');
});
// After page reload, re-attach any still-running tasks spawned in this session
async function reattachSessionTasks(sessionId) {
    try {
        const res = await tasksApi.list({ sessionId });
        const all = res.data;
        const active = all.filter(t => !['done', 'error', 'killed'].includes(t.status));
        const justDone = all.filter(t => ['done', 'error', 'killed'].includes(t.status));
        // If some tasks already completed but we don't have their notifications yet
        // (e.g. page was closed while subagent was running), do a reload to catch up.
        if (justDone.length > 0) {
            setTimeout(async () => {
                if (streaming.value)
                    return; // don't overwrite mid-stream
                if (currentSessionId.value !== sessionId)
                    return;
                try {
                    const r = await sessionsApi.get(props.agentId, sessionId);
                    if (streaming.value)
                        return; // streaming may have started while awaiting
                    if (currentSessionId.value !== sessionId)
                        return;
                    const parsed = r.data.messages ?? [];
                    const loaded = [];
                    if (parsed.some((m) => m.isCompact || m.role === 'compaction')) {
                        loaded.push({ role: 'system', text: '更早的内容已压缩' });
                    }
                    for (const m of parsed) {
                        if (m.role === 'compaction')
                            continue;
                        if ((m.text?.trim() || (m.toolCalls && m.toolCalls.length > 0)) && !isSystemSignalMsg(m.text))
                            loaded.push({ role: m.role, text: m.text, toolCalls: m.toolCalls?.map((tc) => ({ id: tc.id, name: tc.name, input: tc.input, result: tc.result, status: 'done', _expanded: false, ...processToolResult(tc.result ?? '') })) });
                    }
                    messages.value = loaded;
                    scrollBottom();
                }
                catch { }
            }, 500);
        }
        if (active.length === 0)
            return;
        resumedTasks.value = active.map(t => ({
            id: t.id,
            label: t.label || t.id.slice(0, 8),
            status: t.status,
        }));
        startTaskPolling();
    }
    catch { /* ignore */ }
}
function scrollBottom(force = false) {
    nextTick(() => {
        if (!msgListRef.value)
            return;
        // #14 fix: if user scrolled up during streaming, don't force scroll down
        if (!force && userScrolledUp.value)
            return;
        msgListRef.value.scrollTop = msgListRef.value.scrollHeight;
    });
}
// #14 fix: detect user scroll during streaming
function onMsgListScroll() {
    if (!msgListRef.value || !streaming.value)
        return;
    const el = msgListRef.value;
    const distFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    userScrolledUp.value = distFromBottom > 80;
}
function autoGrow() {
    if (!inputRef.value)
        return;
    inputRef.value.style.height = 'auto';
    const maxH = 200;
    const newH = Math.min(inputRef.value.scrollHeight, maxH);
    inputRef.value.style.height = newH + 'px';
    inputRef.value.style.overflowY = inputRef.value.scrollHeight > maxH ? 'auto' : 'hidden';
}
function fillInput(text) {
    inputText.value = text;
    nextTick(() => inputRef.value?.focus());
}
function fmtJson(raw) {
    try {
        return JSON.stringify(JSON.parse(raw), null, 2);
    }
    catch {
        return raw;
    }
}
function copyMsg(text) {
    navigator.clipboard?.writeText(text);
    const idx = messages.value.findIndex(m => m.text === text);
    copied.value = idx;
    setTimeout(() => { copied.value = null; }, 1500);
}
function retryMsg(idx) {
    for (let i = idx - 1; i >= 0; i--) {
        const m = messages.value[i];
        if (m && m.role === 'user') {
            const text = m.text;
            const imgs = m.images ?? [];
            messages.value.splice(i, messages.value.length - i);
            runChat(text, imgs);
            return;
        }
    }
}
function previewImg(src) { previewSrc.value = src; }
// processToolResult detects special markers in a tool result string and returns
// extra fields to merge into the ToolCall object (mediaUrl, fileCard).
// Used both during streaming and when loading history.
function processToolResult(result) {
    const extra = {};
    if (!result)
        return extra;
    const mediaMatch = result.match(/\[media:([^\]]+)\]/);
    if (mediaMatch && mediaMatch[1]) {
        const token = localStorage.getItem('aipanel_token') ?? '';
        extra.mediaUrl = `/api/media?path=${encodeURIComponent(mediaMatch[1])}&token=${encodeURIComponent(token)}`;
    }
    const fileCardMatch = result.match(/\[file_card:([^|]+)\|([^|]+)\|([^\]]+)\]/);
    if (fileCardMatch && fileCardMatch[1] && fileCardMatch[2] && fileCardMatch[3]) {
        extra.fileCard = { url: fileCardMatch[1], name: fileCardMatch[2], size: fileCardMatch[3] };
    }
    return extra;
}
// ── Markdown renderer (lightweight) ──────────────────────────────────────
/**
 * 过滤 skill-studio action JSON 块，避免原始协议 JSON 显示给用户。
 * 匹配 ```json {...} ``` 和裸 JSON 对象（action: edit_file / fill_skill）。
 */
function filterActionBlocks(text) {
    // Remove ```json...``` blocks with action keys
    text = text.replace(/```(?:json)?\s*\{[^`]*?"action"\s*:\s*"(?:edit_file|fill_skill)"[^`]*?\}\s*```/gs, '');
    // Remove bare JSON objects with action keys (allow multi-line)
    text = text.replace(/\{\s*"action"\s*:\s*"(?:edit_file|fill_skill)"[\s\S]*?\n?\}/g, '');
    return text.trim();
}
// 极简 syntax highlight：只做通用关键字/字符串/数字/注释染色，不依赖第三方库
function highlightCode(code, lang) {
    const l = (lang || '').toLowerCase();
    // 先转义 HTML（code 已经是 escape 后的字符串，但为保险再做一次 <>&）
    let src = code;
    // 通用 token：字符串 / 数字 / 注释 / 关键字
    const keywordsByLang = {
        js: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'class', 'new', 'async', 'await', 'import', 'export', 'from', 'default', 'try', 'catch', 'finally', 'throw', 'typeof', 'instanceof', 'this', 'null', 'true', 'false', 'undefined'],
        ts: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'class', 'new', 'async', 'await', 'import', 'export', 'from', 'default', 'try', 'catch', 'finally', 'throw', 'typeof', 'instanceof', 'this', 'null', 'true', 'false', 'undefined', 'interface', 'type', 'enum', 'as', 'readonly', 'public', 'private', 'protected'],
        go: ['func', 'package', 'import', 'var', 'const', 'type', 'struct', 'interface', 'return', 'if', 'else', 'for', 'range', 'switch', 'case', 'default', 'break', 'continue', 'go', 'defer', 'chan', 'map', 'true', 'false', 'nil'],
        py: ['def', 'class', 'import', 'from', 'return', 'if', 'elif', 'else', 'for', 'while', 'try', 'except', 'finally', 'raise', 'with', 'as', 'lambda', 'None', 'True', 'False', 'and', 'or', 'not', 'in', 'is', 'pass', 'yield', 'async', 'await', 'self'],
        sh: ['if', 'then', 'else', 'elif', 'fi', 'for', 'do', 'done', 'while', 'case', 'esac', 'function', 'return', 'export', 'echo', 'cd', 'ls', 'pwd', 'local', 'readonly'],
        bash: ['if', 'then', 'else', 'elif', 'fi', 'for', 'do', 'done', 'while', 'case', 'esac', 'function', 'return', 'export', 'echo', 'cd', 'ls', 'pwd', 'local', 'readonly'],
        rust: ['fn', 'let', 'mut', 'pub', 'use', 'mod', 'struct', 'enum', 'impl', 'trait', 'match', 'if', 'else', 'for', 'while', 'loop', 'return', 'self', 'Self', 'true', 'false', 'as'],
        json: [],
    };
    const base = l.split('-')[0] || '';
    const kwList = keywordsByLang[l] || (base ? keywordsByLang[base] : undefined) || [];
    // 注意顺序：先注释 → 字符串 → 数字 → 关键字（避免关键字替换破坏字符串）
    // 注释: // ...\n  |  # ...\n  |  /* ... */
    src = src.replace(/(\/\/[^\n]*|#[^\n]*|\/\*[\s\S]*?\*\/)/g, '<span class="tok-c">$1</span>');
    // 字符串: "..." / '...' / `...`  (避免跨越已经高亮的 span)
    src = src.replace(/("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`)/g, '<span class="tok-s">$1</span>');
    // 数字
    src = src.replace(/\b(\d+\.?\d*)\b/g, '<span class="tok-n">$1</span>');
    // 关键字
    if (kwList.length) {
        const re = new RegExp('\\b(' + kwList.join('|') + ')\\b', 'g');
        src = src.replace(re, '<span class="tok-k">$1</span>');
    }
    return src;
}
function renderMd(text) {
    if (!text)
        return '';
    // In skill-studio, hide protocol JSON from the user — actions are handled silently
    if (props.scenario === 'skill-studio')
        text = filterActionBlocks(text);
    // 1. 预抽取代码块（避免内层 markdown 干扰）
    const codeBlocks = [];
    let html = text
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => {
        const highlighted = highlightCode(code, lang);
        const header = lang ? `<div class="code-lang">${lang}</div>` : '';
        const i = codeBlocks.length;
        codeBlocks.push(`<div class="code-wrap${lang ? ' lang-' + lang : ''}">${header}<pre class="code-block"><code>${highlighted}</code></pre></div>`);
        return `\x00CODEBLOCK${i}\x00`;
    });
    // 2. 表格（GFM 样式）: 先用多行正则识别 |a|b| + |---|---| + 数据行
    html = html.replace(/(^\|[^\n]+\|\n\|[\s\-:|]+\|\n(?:\|[^\n]*\|\n?)+)/gm, (block) => {
        const lines = block.trim().split('\n');
        if (lines.length < 2)
            return block;
        const parseRow = (line) => line.replace(/^\||\|$/g, '').split('|').map(c => c.trim());
        const headers = parseRow(lines[0]);
        const rows = lines.slice(2).map(parseRow);
        const thead = '<tr>' + headers.map(h => `<th>${h}</th>`).join('') + '</tr>';
        const tbody = rows.map(r => '<tr>' + r.map(c => `<td>${c}</td>`).join('') + '</tr>').join('');
        return `<div class="md-table-wrap"><table class="md-table"><thead>${thead}</thead><tbody>${tbody}</tbody></table></div>`;
    });
    // 3. 标题 / 引用块 / 列表 / 行内元素
    html = html
        // 引用块 (> ...)
        .replace(/^&gt; (.+)$/gm, '<blockquote>$1</blockquote>')
        // 合并相邻 blockquote
        .replace(/(<\/blockquote>\n<blockquote>)/g, '<br>')
        // Inline code
        .replace(/`([^`\n]+)`/g, '<code class="inline-code">$1</code>')
        // Bold
        .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
        // Italic (避开 bold 的 **)
        .replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, '$1<em>$2</em>')
        // Links
        .replace(/\[(.+?)\]\((https?:\/\/[^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
        // Headings
        .replace(/^###### (.+)$/gm, '<h4 class="md-h6">$1</h4>')
        .replace(/^##### (.+)$/gm, '<h4 class="md-h5">$1</h4>')
        .replace(/^#### (.+)$/gm, '<h4>$1</h4>')
        .replace(/^### (.+)$/gm, '<h3>$1</h3>')
        .replace(/^## (.+)$/gm, '<h2>$1</h2>')
        .replace(/^# (.+)$/gm, '<h1>$1</h1>')
        // 水平线
        .replace(/^---+$/gm, '<hr />')
        // 有序列表
        .replace(/^(\d+)\. (.+)$/gm, '<li data-ol="1">$2</li>')
        // 无序列表
        .replace(/^[-*] (.+)$/gm, '<li>$1</li>')
        // 将连续 li 包起来（区分有序/无序）
        .replace(/(<li data-ol="1">[\s\S]*?<\/li>)(?=\n?(?!<li data-ol="1">))/g, (m) => `<ol>${m.replace(/ data-ol="1"/g, '')}</ol>`)
        .replace(/(<li>(?:(?!data-ol="1").)*?<\/li>(?:\n<li>(?:(?!data-ol="1").)*?<\/li>)*)/g, (m) => `<ul>${m}</ul>`)
        // 普通换行 → <br>
        .replace(/([^>\n])\n([^<\n])/g, '$1<br>$2');
    // 4. 恢复代码块
    html = html.replace(/\x00CODEBLOCK(\d+)\x00/g, (_, i) => codeBlocks[parseInt(i)] || '');
    return html;
}
// ── Apply data extractor (robust, multi-strategy) ────────────────────────
/**
 * Returns precomputed applyData from msg, OR tries to extract a JSON object
 * from the message text using multiple fallback strategies.
 * Returns null if nothing parseable found, or if not applyable mode.
 */
/**
 * 从 AI 回复中提取选项行，变成 quick-reply chips。
 * 检测模式：以 emoji 开头 + 空格 + 中文描述 的行（如 "🎙 想要更偏向英超"）
 */
function extractOptions(text) {
    const lines = text.split('\n');
    const opts = [];
    const emojiLineRe = /^([🎙😄🌐🛎📚🎨💼🤖⚽🎯✅❌🔥💡🎁🚀🌟💎🎪🎭🎬🎤]|[\u{1F300}-\u{1FFFF}]|[\u{2600}-\u{27BF}])\s+(.{4,40})/u;
    for (const line of lines) {
        const trimmed = line.replace(/^[-*•]\s*/, '').trim();
        const m = trimmed.match(emojiLineRe);
        if (m) {
            // 去掉末尾标点
            const opt = trimmed.replace(/[：:。，,]$/, '').trim();
            if (opt.length >= 5)
                opts.push(opt);
        }
    }
    // 最多返回 5 个选项
    return opts.slice(0, 5);
}
/** 判断文本中是否含有 JSON 块（快速检测，不解析） */
function hasJsonBlock(text) {
    if (!text)
        return false;
    return /\{[\s\S]{30,}\}/.test(text) &&
        (text.includes('"name"') || text.includes('"identity"') ||
            text.includes('"soul"') || text.includes('"IDENTITY"') || text.includes('"SOUL"'));
}
/** 用户手动触发解析并 emit apply */
function manualApply(msg) {
    const data = tryExtractJson(msg.text);
    if (data) {
        // Clear previous apply cards — only show the one being applied
        messages.value.forEach(m => { if (m !== msg && m.applyData)
            delete m.applyData; });
        msg.applyData = data;
        nextTick(() => emit('apply', data));
    }
    else {
        alert('未能从消息中提取到配置 JSON，请手动复制');
    }
}
/**
 * 用括号平衡计数从文本中提取第一个合法 JSON 对象字符串。
 * 比正则更可靠：能正确处理值中含 `}` 的情况。
 */
function extractBalancedJson(text, fromIndex = 0) {
    const start = text.indexOf('{', fromIndex);
    if (start === -1)
        return null;
    let depth = 0;
    let inStr = false;
    let esc = false;
    for (let i = start; i < text.length; i++) {
        const c = text[i];
        if (esc) {
            esc = false;
            continue;
        }
        if (c === '\\' && inStr) {
            esc = true;
            continue;
        }
        if (c === '"') {
            inStr = !inStr;
            continue;
        }
        if (!inStr) {
            if (c === '{')
                depth++;
            else if (c === '}') {
                depth--;
                if (depth === 0)
                    return { raw: text.slice(start, i + 1), end: i + 1 };
            }
        }
    }
    return null;
}
function tryExtractJson(text) {
    // Strategy 1: all ```(json)? ... ``` fence blocks — try LAST one first (most likely final config)
    const fenceRe = /```(?:json)?\s*\n?([\s\S]*?)```/g;
    const fenceBlocks = [];
    let fm;
    while ((fm = fenceRe.exec(text)) !== null) {
        const inner = (fm[1] ?? '').trim();
        if (inner.startsWith('{'))
            fenceBlocks.push(inner);
    }
    for (let i = fenceBlocks.length - 1; i >= 0; i--) {
        const raw = fenceBlocks[i];
        const balanced = extractBalancedJson(raw);
        if (!balanced)
            continue;
        const r = safeParse(balanced.raw) ?? safeParse(escapeJsonNewlines(balanced.raw));
        if (r)
            return r;
    }
    // Strategy 2: balanced brace scan over full text — collect all, try last first
    const candidates = [];
    let pos = 0;
    while (pos < text.length) {
        const found = extractBalancedJson(text, pos);
        if (!found)
            break;
        candidates.push(found.raw);
        pos = found.end;
    }
    for (let i = candidates.length - 1; i >= 0; i--) {
        const raw = candidates[i];
        const r = safeParse(raw) ?? safeParse(escapeJsonNewlines(raw));
        if (r)
            return r;
    }
    return null;
}
function safeParse(raw) {
    try {
        const obj = JSON.parse(raw);
        if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
            // Only return if it has at least one string-valued known field
            const knownKeys = ['name', 'id', 'description', 'identity', 'soul', 'IDENTITY', 'SOUL', 'NAME', 'DESCRIPTION'];
            if (Object.keys(obj).some(k => knownKeys.includes(k)))
                return obj;
        }
    }
    catch { /* ignore */ }
    return null;
}
function escapeJsonNewlines(raw) {
    // Replace actual newlines inside JSON string values only
    // Split by quote pairs and only escape within strings
    let result = '';
    let inString = false;
    let escape = false;
    for (let i = 0; i < raw.length; i++) {
        const c = raw[i];
        if (escape) {
            result += c;
            escape = false;
            continue;
        }
        if (c === '\\' && inString) {
            result += c;
            escape = true;
            continue;
        }
        if (c === '"') {
            inString = !inString;
            result += c;
            continue;
        }
        if (inString && c === '\n') {
            result += '\\n';
            continue;
        }
        if (inString && c === '\r') {
            result += '\\r';
            continue;
        }
        result += c;
    }
    return result;
}
// ── Tool helpers ──────────────────────────────────────────────────────────
const TOOL_ICONS = {
    exec: '⚡', bash: '⚡',
    read: '📖', write: '✏️', edit: '✏️',
    web_search: '🌐', web_fetch: '🌐', browser: '🌐',
    agent_spawn: '🚀', agent_tasks: '📋', agent_kill: '🛑', agent_result: '📊',
    project_read: '📁', project_write: '📁', project_list: '📁', project_create: '📁', project_glob: '📁',
    memory_search: '🧠', memory_get: '🧠',
    image: '🖼️', tts: '🔊', show_image: '🖼️',
    cron: '⏱️',
};
function toolIcon(name) {
    return TOOL_ICONS[name] ?? '⚙️';
}
function toolSummary(name, rawInput) {
    try {
        const inp = JSON.parse(rawInput);
        if (name === 'exec' || name === 'bash')
            return (inp.command ?? '').slice(0, 60);
        if (name === 'read')
            return inp.file_path ?? inp.path ?? '';
        if (name === 'write')
            return (inp.file_path ?? inp.path ?? '') + (inp.content ? ` (${inp.content.length}B)` : '');
        if (name === 'edit')
            return inp.file_path ?? inp.path ?? '';
        if (name === 'web_search')
            return inp.query ?? '';
        if (name === 'web_fetch')
            return inp.url ?? '';
        if (name === 'agent_spawn')
            return `→ ${inp.agentId}: ${(inp.task ?? '').slice(0, 40)}`;
        if (name === 'project_read')
            return inp.path ?? '';
        if (name === 'project_write')
            return inp.path ?? '';
        if (name === 'memory_search')
            return inp.query ?? '';
        if (name === 'show_image')
            return (inp.path ?? '').split('/').pop() ?? '';
    }
    catch { }
    return '';
}
// ── File type helpers ──────────────────────────────────────────────────────
const TEXT_EXTS = new Set([
    'txt', 'md', 'markdown', 'js', 'ts', 'jsx', 'tsx', 'vue', 'go', 'py', 'rs', 'java', 'kt', 'swift',
    'html', 'css', 'scss', 'less', 'json', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'env',
    'sh', 'bash', 'zsh', 'fish', 'ps1', 'bat', 'cmd', 'dockerfile', 'makefile',
    'sql', 'graphql', 'proto', 'xml', 'svg', 'gitignore', 'gitattributes',
    'csv', 'tsv', 'log', 'conf', 'properties', 'r', 'rb', 'php',
]);
// 二进制文件：上传到 workspace/uploads/，消息里携带路径引用
const BINARY_EXTS = new Set([
    'xlsx', 'xls', 'xlsm', 'xlsb',
    'docx', 'doc', 'rtf',
    'pptx', 'ppt',
    'pdf',
    'zip', 'tar', 'gz',
    'mp3', 'mp4', 'mov', 'avi',
]);
function isTextFile(name) {
    const ext = name.split('.').pop()?.toLowerCase() ?? '';
    return TEXT_EXTS.has(ext);
}
function fileTypeIcon(name) {
    const ext = name.split('.').pop()?.toLowerCase() ?? '';
    const icons = {
        js: '🟨', ts: '🔵', vue: '💚', go: '🐹', py: '🐍', rs: '🦀',
        html: '🌐', css: '🎨', json: '📋', md: '📝', sh: '⚡',
        sql: '🗄️', yaml: '⚙️', yml: '⚙️', dockerfile: '🐳',
        csv: '📊', tsv: '📊',
        xlsx: '📗', xls: '📗', xlsm: '📗', xlsb: '📗',
        docx: '📘', doc: '📘', rtf: '📘',
        pptx: '📙', ppt: '📙',
        pdf: '📕',
        zip: '🗜️', tar: '🗜️', gz: '🗜️',
        mp3: '🎵', mp4: '🎬', mov: '🎬', avi: '🎬',
    };
    return icons[ext] ?? '📄';
}
function formatFileSize(bytes) {
    if (bytes < 1024)
        return `${bytes}B`;
    if (bytes < 1024 * 1024)
        return `${(bytes / 1024).toFixed(1)}KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
}
// ── Image handling ────────────────────────────────────────────────────────
function handlePaste(e) {
    const items = e.clipboardData?.items;
    if (!items)
        return;
    for (const item of Array.from(items)) {
        if (item.type.startsWith('image/')) {
            e.preventDefault();
            const file = item.getAsFile();
            if (file)
                readImageFile(file);
        }
    }
}
// Use depth counter to avoid flicker when dragging over child elements
function onDragEnter(e) {
    e.preventDefault();
    _dragDepth++;
    isDragOver.value = true;
}
function onDragLeave(e) {
    e.preventDefault();
    _dragDepth--;
    if (_dragDepth <= 0) {
        _dragDepth = 0;
        isDragOver.value = false;
    }
}
function isBinaryFile(name) {
    const ext = name.split('.').pop()?.toLowerCase() ?? '';
    return BINARY_EXTS.has(ext);
}
function handleGlobalDrop(e) {
    _dragDepth = 0;
    isDragOver.value = false;
    const files = e.dataTransfer?.files;
    if (!files)
        return;
    for (const file of Array.from(files)) {
        if (file.type.startsWith('image/')) {
            readImageFile(file);
        }
        else if (isTextFile(file.name)) {
            readTextFile(file);
        }
        else if (isBinaryFile(file.name)) {
            uploadBinaryFile(file);
        }
        // else: truly unsupported, silently ignore
    }
}
function handleFileSelect(e) {
    const files = e.target.files;
    if (!files)
        return;
    for (const file of Array.from(files)) {
        if (file.type.startsWith('image/')) {
            readImageFile(file);
        }
        else if (isTextFile(file.name)) {
            readTextFile(file);
        }
        else if (isBinaryFile(file.name)) {
            uploadBinaryFile(file);
        }
    }
    ;
    e.target.value = '';
}
function readTextFile(file) {
    const reader = new FileReader();
    reader.onload = () => {
        if (typeof reader.result === 'string') {
            pendingFiles.value.push({ name: file.name, content: reader.result, size: file.size });
        }
    };
    reader.readAsText(file, 'utf-8');
}
async function uploadBinaryFile(file) {
    const uploadPath = `uploads/${Date.now()}_${file.name}`;
    const entry = { name: file.name, content: '', uploadPath, uploading: true, size: file.size };
    pendingFiles.value.push(entry);
    const idx = pendingFiles.value.length - 1;
    try {
        const buf = await file.arrayBuffer();
        const bytes = new Uint8Array(buf);
        const base = (localStorage.getItem('aipanel_url') || '').replace(/\/$/, '');
        const token = localStorage.getItem('aipanel_token') || '';
        // Chunk size: 50KB raw bytes → ~67KB base64 JSON — works through any proxy/VPN
        const CHUNK = 50 * 1024;
        const total = Math.ceil(bytes.byteLength / CHUNK) || 1;
        for (let i = 0; i < total; i++) {
            const slice = bytes.slice(i * CHUNK, (i + 1) * CHUNK);
            // Convert chunk to base64
            let binary = '';
            for (let j = 0; j < slice.length; j++)
                binary += String.fromCharCode(slice[j]);
            const b64 = btoa(binary);
            const res = await fetch(`${base}/api/agents/${props.agentId}/files/${uploadPath}?chunk=${i}&total=${total}`, {
                method: 'PUT',
                headers: { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
                body: JSON.stringify({ content: 'base64:' + b64 }),
            });
            if (!res.ok) {
                const text = await res.text().catch(() => '');
                throw new Error(`chunk ${i} failed: HTTP ${res.status} ${text.slice(0, 80)}`);
            }
        }
        pendingFiles.value[idx] = { ...entry, uploading: false };
    }
    catch (e) {
        pendingFiles.value[idx] = { ...entry, uploading: false, uploadError: e.message || '上传失败' };
    }
}
function readImageFile(file) {
    const reader = new FileReader();
    reader.onload = () => {
        if (typeof reader.result === 'string')
            pendingImages.value.push(reader.result);
    };
    reader.readAsDataURL(file);
}
function removeImage(i) { pendingImages.value.splice(i, 1); }
// ── Keyboard handling ─────────────────────────────────────────────────────
// Enter = 发送 | Shift+Enter = 换行 | IME 组词期间 (isComposing) 不拦截
function onTextareaKeydown(e) {
    if (e.key !== 'Enter')
        return;
    // Shift / Ctrl / Meta / Alt + Enter = 允许原生换行行为
    if (e.shiftKey || e.ctrlKey || e.metaKey || e.altKey)
        return;
    // 中文输入法组词过程中不拦截 Enter (用 isComposing + keyCode 229 双保险)
    if (e.isComposing || e.keyCode === 229)
        return;
    e.preventDefault();
    send();
}
// ── Send ──────────────────────────────────────────────────────────────────
function send() {
    // 只读模式：不允许发送（多重保险，模板里输入区本就被隐藏）
    if (props.readOnly)
        return;
    const text = inputText.value.trim();
    const imgs = [...pendingImages.value];
    const files = [...pendingFiles.value];
    if (!text && !imgs.length && !files.length)
        return;
    if (streaming.value)
        return;
    // Don't send if any binary file is still uploading
    if (files.some(f => f.uploading))
        return;
    // Build final message text: append file contents as code blocks
    let finalText = text;
    if (files.length > 0) {
        const fileBlocks = files.map(f => {
            if (f.uploadPath) {
                // Binary file: include path reference so agent can process it with tools
                const sizeStr = f.size ? ` (${formatFileSize(f.size)})` : '';
                return `\n\n📎 **${f.name}**${sizeStr}\n文件已上传到工作区路径 \`${f.uploadPath}\`，可用 read/exec/bash 工具处理。`;
            }
            const ext = f.name.split('.').pop() ?? 'text';
            // Truncate very large text files to avoid token overflow
            const MAX = 80000;
            const content = f.content.length > MAX
                ? f.content.slice(0, MAX) + `\n\n…（文件过大，已截断，完整内容共 ${f.content.length} 字符）`
                : f.content;
            return `\n\n📎 **${f.name}**\n\`\`\`${ext}\n${content}\n\`\`\``;
        }).join('');
        finalText = (text ? text + fileBlocks : fileBlocks.trimStart());
    }
    inputText.value = '';
    pendingImages.value = [];
    pendingFiles.value = [];
    nextTick(() => {
        if (inputRef.value) {
            inputRef.value.style.height = 'auto';
        }
    });
    emit('message', finalText, imgs);
    runChat(finalText, imgs);
}
function runChat(text, imgs, silent = false) {
    if (!silent) {
        messages.value.push({ role: 'user', text, images: imgs.length ? imgs : undefined });
        scrollBottom();
    }
    streaming.value = true;
    streamText.value = '';
    streamThinking.value = '';
    streamToolCalls.value = [];
    // Current assistant message being built
    const assistantMsg = { role: 'assistant', text: '', toolCalls: [] };
    messages.value.push(assistantMsg);
    if (silent)
        scrollBottom();
    const msgIdx = messages.value.length - 1;
    // Track active tool call
    let activeToolId = '';
    // Session-aware history:
    //   - sessionId exists → server already owns full history; never send history[] to avoid duplication.
    //   - no sessionId    → legacy mode: build client-side history (capped at 20 turns).
    let historyParam;
    if (currentSessionId.value) {
        historyParam = undefined; // server owns history — explicit, not sent
    }
    else {
        const historyMsgs = messages.value
            .slice(0, -1)
            .filter(m => (m.role === 'user' || m.role === 'assistant') && m.text)
            .slice(-20)
            .map(m => ({ role: m.role, content: m.text }));
        historyParam = historyMsgs.length > 0 ? historyMsgs : undefined;
    }
    const params = {
        sessionId: currentSessionId.value,
        context: props.context,
        scenario: props.scenario,
        skillId: props.skillId,
        images: imgs.length ? imgs : undefined,
        history: historyParam,
    };
    // #13 fix: capture session at send time; discard events if session changed
    const sendSessionId = currentSessionId.value || '';
    // P0.4 Abort fence: register fence so external paths can stop the stream.
    // If there was a previous active fence (e.g. user sent while one was still
    // closing), abort it first to avoid zombie HTTP streams.
    abortActiveStream('new-send');
    const fence = { aborted: false, ctrl: null };
    activeFence.value = fence;
    let compactionBubbleIdx = -1;
    fence.ctrl = chatSSE(props.agentId, text, (ev) => {
        // Fence guard — drop anything that lands after abort() was called
        if (fence.aborted)
            return;
        // #13 fix: if user switched session, discard this response
        if (currentSessionId.value !== sendSessionId) {
            fence.aborted = true;
            try {
                fence.ctrl?.abort();
            }
            catch { /* already aborted */ }
            streaming.value = false;
            return;
        }
        switch (ev.type) {
            case 'compaction_start': {
                // P0.6: show a transient info bubble while compaction runs.
                // Insert BEFORE the pending assistant bubble (which is the last item)
                // so it's visible during the streaming phase (template filters the
                // last message when streaming).
                const kb = Math.round((ev.tokens_before || 0) / 1000);
                const insertAt = Math.max(0, messages.value.length - 1);
                messages.value.splice(insertAt, 0, {
                    role: 'system',
                    sysKind: 'info',
                    text: `🗜️ 正在压缩历史上下文 (~${kb}k tokens)…`,
                });
                compactionBubbleIdx = insertAt;
                scrollBottom();
                break;
            }
            case 'compaction_end': {
                // Update the placeholder bubble with before→after token deltas.
                const msg = compactionBubbleIdx >= 0 ? messages.value[compactionBubbleIdx] : undefined;
                if (msg) {
                    if (ev.error) {
                        msg.text = `⚠️ 历史压缩失败：${ev.error}（本轮对话将使用原始历史）`;
                        msg.sysKind = 'error';
                    }
                    else {
                        const kb = Math.round((ev.tokens_before || 0) / 1000);
                        const ka = Math.round((ev.tokens_after || 0) / 1000);
                        msg.text = `✓ 历史已压缩 ${kb}k → ${ka}k tokens`;
                    }
                }
                compactionBubbleIdx = -1;
                scrollBottom();
                break;
            }
            case 'thinking_delta':
                streamThinking.value += ev.text;
                scrollBottom();
                break;
            case 'text':
            case 'text_delta':
                streamText.value += ev.text;
                scrollBottom();
                break;
            case 'tool_call': {
                const tc = {
                    id: ev.tool_call?.id ?? String(Date.now()),
                    name: ev.tool_call?.name ?? 'tool',
                    input: ev.tool_call?.input ? JSON.stringify(ev.tool_call.input) : undefined,
                    status: 'running',
                    _startedAt: Date.now(),
                    _expanded: false,
                };
                messages.value[msgIdx].toolCalls.push(tc);
                streamToolCalls.value.push(tc);
                activeToolId = tc.id;
                scrollBottom();
                break;
            }
            case 'tool_result': {
                // 关键: 按 tool_call_id 精准匹配 (并行 tool 场景下不能靠 activeToolId)
                // 兼容老后端: 如果没给 tool_call_id, 退回到最近的 running 的 tool
                const matchId = ev.tool_call_id || activeToolId;
                let tc = messages.value[msgIdx].toolCalls?.find(t => t.id === matchId);
                if (!tc) {
                    // 最后兜底: 找第一个还在 running 的 tool
                    tc = messages.value[msgIdx].toolCalls?.find(t => t.status === 'running');
                }
                if (tc) {
                    tc.result = ev.text;
                    tc.status = 'done';
                    if (tc._startedAt) {
                        const ms = Date.now() - tc._startedAt;
                        tc.duration = ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
                    }
                    // Detect special markers ([media:path], [file_card:URL|NAME|SIZE]) in tool result
                    if (ev.text) {
                        Object.assign(tc, processToolResult(ev.text));
                    }
                    // agent_spawn: extract task ID from result and start polling
                    if (tc.name === 'agent_spawn' && ev.text) {
                        const m = ev.text.match(/任务\s*ID[：:]\s*([a-f0-9-]{8,})/i);
                        if (m) {
                            tc.taskId = m[1];
                            tc.taskStatus = 'pending';
                            spawnedTaskMap.set(tc.id, m[1]);
                            startTaskPolling();
                            try {
                                const inp = tc.input ? JSON.parse(tc.input) : {};
                                const spawnedId = inp.agentId ?? '';
                                const spawnedName = inp.agentId ?? '';
                                emit('dispatch', spawnedId, spawnedName, '#409eff', m[1]);
                            }
                            catch { }
                        }
                    }
                    if (tc.name === 'agent_result' && ev.text) {
                        const m = ev.text.match(/[a-f0-9]{8}-[a-f0-9]{3,}/i) ?? ev.text.match(/([a-f0-9-]{8,})/i);
                        try {
                            const inp = tc.input ? JSON.parse(tc.input) : {};
                            const tid = inp.taskId ?? inp.task_id ?? inp.id ?? (m ? m[1] : null);
                            if (tid)
                                emit('task-handled', tid);
                        }
                        catch { }
                    }
                    // Sync into streamToolCalls
                    const stc = streamToolCalls.value.find(t => t.id === tc.id);
                    if (stc) {
                        stc.result = tc.result;
                        stc.status = 'done';
                        stc.duration = tc.duration;
                    }
                }
                scrollBottom();
                break;
            }
            // ── Token 使用量 ────────────────────────────────────────────────────────
            case 'usage': {
                if (ev.input_tokens != null || ev.output_tokens != null) {
                    const cur = messages.value[msgIdx];
                    if (cur) {
                        cur.tokenUsage = {
                            input: ev.input_tokens ?? 0,
                            output: ev.output_tokens ?? 0,
                        };
                    }
                }
                break;
            }
            // ── 派遣面板事件：透传给 DispatchPanel ──────────────────────────────────
            case 'subagent_spawn':
            case 'subagent_report':
            case 'subagent_done':
            case 'subagent_error':
                dispatchPanelRef.value?.handleEvent(ev);
                break;
            case 'done':
            case 'error': {
                // Capture server-side sessionId for subsequent requests
                if (ev.type === 'done' && ev.sessionId) {
                    const isNew = !currentSessionId.value;
                    currentSessionId.value = ev.sessionId;
                    if (isNew)
                        emit('session-change', ev.sessionId);
                }
                const cur = messages.value[msgIdx];
                cur.text = streamText.value;
                cur.thinking = streamThinking.value || undefined;
                // Save token usage from done event if available
                if (ev.type === 'done' && (ev.input_tokens != null || ev.output_tokens != null)) {
                    cur.tokenUsage = {
                        input: ev.input_tokens ?? 0,
                        output: ev.output_tokens ?? 0,
                    };
                }
                if (props.applyable) {
                    const extracted = tryExtractJson(streamText.value);
                    if (extracted) {
                        // Clear apply cards from all previous messages — only the latest should show
                        messages.value.forEach(m => { if (m !== cur && m.applyData)
                            delete m.applyData; });
                        cur.applyData = extracted;
                    }
                }
                // Extract quick-reply options from the response
                const opts = extractOptions(streamText.value);
                if (opts.length >= 2)
                    cur.options = opts;
                if (ev.type === 'error') {
                    if (ev.error?.includes('no model configured')) {
                        // Unique case: the bubble becomes a "please configure model" card,
                        // no partial text to preserve because nothing streamed.
                        cur.noModelError = true;
                        cur.text = '';
                    }
                    else if (isNetworkLayerError(ev.error) && currentSessionId.value) {
                        // P0.2 SSE auto-reconnect:
                        // The HTTP stream was cut by network issue (switched WiFi, locked
                        // phone, flaky link) but the agent generation is probably still
                        // running on the server. Re-subscribe to the same session's
                        // Broadcaster instead of giving up.
                        console.log('[AiChat] SSE network cut, attempting auto-reconnect…');
                        appendReconnectNotice();
                        reconnectAndResume(currentSessionId.value, msgIdx, sendSessionId);
                        // Do NOT fall through to "streaming = false"; keep UI in streaming
                        // state while reconnect is in flight.
                        return;
                    }
                    else {
                        // P0.3 Error isolation:
                        //   - Keep the already-streamed text in the original assistant bubble
                        //   - Mark it as truncated (adds "（因错误中断）" footer)
                        //   - Emit an independent system bubble with the actual error
                        //   This replaces the old behavior of overwriting the entire assistant
                        //   text with "[错误] ...", which destroyed partial answers.
                        cur.truncatedByError = true;
                        messages.value.push({
                            role: 'system',
                            text: formatErrorMessage(ev.error || '未知错误'),
                            sysKind: 'error',
                        });
                    }
                    const tc = cur.toolCalls?.find(t => t.status === 'running');
                    if (tc)
                        tc.status = 'error';
                }
                streaming.value = false;
                streamText.value = '';
                streamThinking.value = '';
                streamToolCalls.value = [];
                userScrolledUp.value = false; // #14 fix: reset on stream end
                // Clear active fence: stream ended naturally, no abort needed
                if (activeFence.value === fence)
                    activeFence.value = null;
                emit('response', cur.text);
                scrollBottom(true); // force scroll to bottom when done
                break;
            }
        }
    }, params);
}
// ── Public API (expose for parent use) ───────────────────────────────────
function clearMessages() { messages.value = []; }
function appendMessage(msg) { messages.value.push(msg); scrollBottom(); }
/** Resume an existing session — immediately loads history from server */
async function resumeSession(sessionId) {
    // P0.4: abort any in-flight stream from the previous session before
    // loading new history.
    abortActiveStream('session-switch');
    currentSessionId.value = sessionId;
    messages.value = [];
    historyLoading.value = true;
    // Snapshot the sessionId at call time so we can detect stale closures
    const mySessionId = sessionId;
    try {
        const res = await sessionsApi.get(props.agentId, sessionId);
        // Guard: user may have switched sessions while waiting for response
        if (currentSessionId.value !== mySessionId)
            return;
        const parsed = res.data.messages ?? [];
        const loaded = [];
        // Insert a compaction marker if any compaction entry exists
        const hasCompaction = parsed.some(m => m.isCompact || m.role === 'compaction');
        if (hasCompaction) {
            loaded.push({ role: 'system', text: '更早的内容已压缩' });
        }
        for (const m of parsed) {
            if (m.role === 'compaction')
                continue; // skip raw compaction entries
            if (!m.text?.trim() && !(m.toolCalls?.length))
                continue; // skip empty messages
            if (isSystemSignalMsg(m.text))
                continue; // skip <task-notification> internal signals
            loaded.push({
                role: m.role,
                text: m.text,
                toolCalls: m.toolCalls?.map((tc) => ({
                    id: tc.id,
                    name: tc.name,
                    input: tc.input,
                    result: tc.result,
                    status: 'done',
                    _expanded: false,
                })),
            });
        }
        messages.value = loaded;
        scrollBottom();
        // Re-attach any still-running background tasks from this session
        reattachSessionTasks(sessionId);
        // Check if a generation is still running in the background → reconnect
        reconnectIfGenerating(sessionId);
    }
    catch (e) {
        // 404 = 新 session，正常情况，直接留空
        if (e?.response?.status === 404) {
            messages.value = [];
        }
        else {
            console.error('[AiChat] resumeSession failed', e);
            messages.value = [{ role: 'system', text: '历史加载失败，继续对话仍可接续' }];
        }
    }
    finally {
        historyLoading.value = false;
    }
}
/**
 * Check if a session has an in-progress generation in the background.
 * If so, attach to the broadcaster and show the streaming response.
 * Called automatically on page load / tab refocus when a sessionId is known.
 */
async function reconnectIfGenerating(sessionId) {
    if (streaming.value)
        return; // already streaming
    const status = await getSessionStatus(props.agentId, sessionId);
    // Stale-closure guard: user may have switched sessions while we were waiting for status.
    // If currentSessionId changed, our update would overwrite the wrong session's UI.
    if (currentSessionId.value !== sessionId)
        return;
    if (!status.hasWorker)
        return; // no active worker at all
    if (status.status !== 'generating') {
        // Worker exists but is idle — generation just finished (or just became idle).
        // Reload history once now, then again after a short delay in case the runner
        // saved to disk just as we were checking (race between AppendMessage and IsBusy).
        const doReload = async () => {
            if (streaming.value)
                return; // don't overwrite mid-stream
            try {
                const res = await sessionsApi.get(props.agentId, sessionId);
                if (streaming.value)
                    return; // streaming may have started while awaiting
                if (currentSessionId.value !== sessionId)
                    return;
                const parsed = res.data.messages ?? [];
                const loaded = [];
                if (parsed.some((m) => m.isCompact || m.role === 'compaction')) {
                    loaded.push({ role: 'system', text: '更早的内容已压缩' });
                }
                for (const m of parsed) {
                    if (m.role === 'compaction')
                        continue;
                    if ((m.text?.trim() || (m.toolCalls && m.toolCalls.length > 0)) && !isSystemSignalMsg(m.text))
                        loaded.push({ role: m.role, text: m.text, toolCalls: m.toolCalls?.map((tc) => ({ id: tc.id, name: tc.name, input: tc.input, result: tc.result, status: 'done', _expanded: false, ...processToolResult(tc.result ?? '') })) });
                }
                messages.value = loaded;
                scrollBottom();
            }
            catch { }
        };
        await doReload();
        // Second reload after 1s — catches the case where the runner saved just after our first reload
        setTimeout(async () => {
            if (streaming.value)
                return; // don't overwrite mid-stream
            if (currentSessionId.value !== sessionId)
                return;
            await doReload();
        }, 1000);
        return;
    }
    // Worker is actively generating — subscribe to live stream.
    // Guard: only proceed if still on the same session
    if (currentSessionId.value !== sessionId)
        return;
    streaming.value = true;
    streamText.value = '';
    streamThinking.value = '';
    streamToolCalls.value = [];
    const assistantMsg = { role: 'assistant', text: '', toolCalls: [] };
    messages.value.push(assistantMsg);
    const msgIdx = messages.value.length - 1;
    scrollBottom();
    let activeToolId = '';
    const ctrl = resumeSSE(props.agentId, sessionId, (ev) => {
        switch (ev.type) {
            case 'idle':
                // Generation already finished before we connected — nothing to do
                messages.value.splice(msgIdx, 1); // remove empty bubble
                streaming.value = false;
                break;
            case 'thinking_delta':
                streamThinking.value += ev.text;
                scrollBottom();
                break;
            case 'text':
            case 'text_delta':
                streamText.value += ev.text;
                scrollBottom();
                break;
            case 'tool_call': {
                const tc = {
                    id: ev.tool_call?.id ?? String(Date.now()),
                    name: ev.tool_call?.name ?? 'tool',
                    input: ev.tool_call?.input ? JSON.stringify(ev.tool_call.input) : undefined,
                    status: 'running',
                    _startedAt: Date.now(),
                    _expanded: false,
                };
                messages.value[msgIdx].toolCalls.push(tc);
                streamToolCalls.value.push(tc);
                activeToolId = tc.id;
                scrollBottom();
                break;
            }
            case 'tool_result': {
                const matchId = ev.tool_call_id || activeToolId;
                let tc = messages.value[msgIdx].toolCalls?.find(t => t.id === matchId);
                if (!tc) {
                    tc = messages.value[msgIdx].toolCalls?.find(t => t.status === 'running');
                }
                if (tc) {
                    tc.result = ev.text;
                    tc.status = 'done';
                    if (tc._startedAt) {
                        const ms = Date.now() - tc._startedAt;
                        tc.duration = ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
                    }
                    if (ev.text)
                        Object.assign(tc, processToolResult(ev.text));
                    const stc = streamToolCalls.value.find(t => t.id === tc.id);
                    if (stc) {
                        stc.result = tc.result;
                        stc.status = 'done';
                        stc.duration = tc.duration;
                        Object.assign(stc, processToolResult(ev.text ?? ''));
                    }
                }
                scrollBottom();
                break;
            }
            // ── 派遣面板事件（reconnect 时）────────────────────────────────────────
            case 'subagent_spawn':
            case 'subagent_report':
            case 'subagent_done':
            case 'subagent_error':
                dispatchPanelRef.value?.handleEvent(ev);
                break;
            case 'done':
            case 'error': {
                if (ev.type === 'done' && ev.sessionId) {
                    const isNew = !currentSessionId.value;
                    currentSessionId.value = ev.sessionId;
                    if (isNew)
                        emit('session-change', ev.sessionId);
                }
                const cur = messages.value[msgIdx];
                cur.text = streamText.value;
                cur.thinking = streamThinking.value || undefined;
                if (ev.type === 'error') {
                    if (ev.error?.includes('no model configured')) {
                        cur.noModelError = true;
                        cur.text = '';
                    }
                    else {
                        // P0.3 Error isolation (resume path — same logic as above)
                        cur.truncatedByError = true;
                        messages.value.push({
                            role: 'system',
                            text: formatErrorMessage(ev.error || '未知错误'),
                            sysKind: 'error',
                        });
                    }
                }
                streaming.value = false;
                streamText.value = '';
                streamThinking.value = '';
                streamToolCalls.value = [];
                scrollBottom();
                break;
            }
        }
    });
    // Store abort controller so it can be cancelled if needed
    // (reuse the existing abortCtrl pattern if present, otherwise just store locally)
    onUnmounted(() => ctrl.abort());
}
/** Start a brand new session (clears sessionId + messages) */
function startNewSession() {
    // P0.4: abort any in-flight stream before dropping session context.
    abortActiveStream('new-session');
    currentSessionId.value = undefined;
    messages.value = [];
}
function sendText(text) { fillInput(text); nextTick(send); }
/** 静默发送：只显示 AI 回复，不在聊天中添加用户消息（用于自动触发场景） */
function sendSilent(text) { runChat(text, [], true); }
/**
 * 子任务完成回调：注入系统提示气泡，再静默触发主助手流式汇报结果。
 * 若当前正在流式生成，等待本轮结束后再触发，避免被 streaming 守卫拦截。
 */
function continueAfterSpawn(agentName, label, output) {
    const doIt = () => {
        appendMessage({ role: 'system', text: `✅ ${agentName} 完成了任务「${label}」` });
        const prompt = `[系统通知] ${agentName} 已完成你派遣的任务「${label}」，以下是执行结果：\n\n${output}\n\n请基于以上结果，向用户做一个自然的汇报。`;
        nextTick(() => runChat(prompt, [], true));
    };
    if (!streaming.value) {
        doIt();
        return;
    }
    // 主助手正在回复——等本轮流式结束后再触发
    const stop = watch(streaming, (val) => {
        if (!val) {
            stop();
            doIt();
        }
    });
}
/**
 * 显式装入一组历史消息并强制滚到底部（用于 AgentDetailView 点击渠道会话时，
 * 把 convlog 数据渲染进 AiChat 的只读气泡流里）。
 */
function loadHistoryMessages(msgs) {
    currentSessionId.value = undefined; // 不启任何 session 订阅
    messages.value = msgs;
    streaming.value = false;
    streamText.value = '';
    streamThinking.value = '';
    streamToolCalls.value = [];
    userScrolledUp.value = false;
    nextTick(() => scrollBottom(true));
}
const __VLS_exposed = { clearMessages, appendMessage, sendText, sendSilent, fillInput, messages, streaming, currentSessionId, resumeSession, startNewSession, continueAfterSpawn, loadHistoryMessages };
defineExpose(__VLS_exposed);
// ── Init ─────────────────────────────────────────────────────────────────
onMounted(() => {
    scrollBottom();
    // On page load: if a session is already active, load messages and check ongoing background generation
    if (currentSessionId.value) {
        if (props.initialMessages && props.initialMessages.length > 0) {
            // initialMessages provided externally — skip fetch, just reconnect
            reconnectIfGenerating(currentSessionId.value);
        }
        else {
            // Load full message history for this session
            resumeSession(currentSessionId.value);
        }
    }
});
const __VLS_defaults = {
    examples: () => [],
    showThinking: false,
    compact: false,
    applyable: false,
};
const __VLS_ctx = {
    ...{},
    ...{},
    ...{},
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['chat-messages']} */ ;
/** @type {__VLS_StyleScopedClasses['example-chip']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['assistant']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-col']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-system']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-system']} */ ;
/** @type {__VLS_StyleScopedClasses['truncated-footer']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['user']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['assistant']} */ ;
/** @type {__VLS_StyleScopedClasses['no-model-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['no-model-onboard-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-col']} */ ;
/** @type {__VLS_StyleScopedClasses['thinking-summary']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step']} */ ;
/** @type {__VLS_StyleScopedClasses['done']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step-dot']} */ ;
/** @type {__VLS_StyleScopedClasses['running']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step-dot']} */ ;
/** @type {__VLS_StyleScopedClasses['done']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step-dot']} */ ;
/** @type {__VLS_StyleScopedClasses['error']} */ ;
/** @type {__VLS_StyleScopedClasses['task-badge']} */ ;
/** @type {__VLS_StyleScopedClasses['task-badge']} */ ;
/** @type {__VLS_StyleScopedClasses['running']} */ ;
/** @type {__VLS_StyleScopedClasses['task-badge']} */ ;
/** @type {__VLS_StyleScopedClasses['done']} */ ;
/** @type {__VLS_StyleScopedClasses['task-badge']} */ ;
/** @type {__VLS_StyleScopedClasses['error']} */ ;
/** @type {__VLS_StyleScopedClasses['task-badge']} */ ;
/** @type {__VLS_StyleScopedClasses['mub-action']} */ ;
/** @type {__VLS_StyleScopedClasses['resumed-chip']} */ ;
/** @type {__VLS_StyleScopedClasses['running']} */ ;
/** @type {__VLS_StyleScopedClasses['resumed-chip']} */ ;
/** @type {__VLS_StyleScopedClasses['pending']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-pre']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-file-link']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['user']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['inline-code']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['md-table']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['md-table']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['md-table']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['md-table']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
/** @type {__VLS_StyleScopedClasses['apply-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['assistant']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-actions']} */ ;
/** @type {__VLS_StyleScopedClasses['act-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['option-chip']} */ ;
/** @type {__VLS_StyleScopedClasses['typing-dots']} */ ;
/** @type {__VLS_StyleScopedClasses['typing-dots']} */ ;
/** @type {__VLS_StyleScopedClasses['typing-dots']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-input-area']} */ ;
/** @type {__VLS_StyleScopedClasses['ai-chat']} */ ;
/** @type {__VLS_StyleScopedClasses['attach-thumb']} */ ;
/** @type {__VLS_StyleScopedClasses['attach-file-remove']} */ ;
/** @type {__VLS_StyleScopedClasses['input-row']} */ ;
/** @type {__VLS_StyleScopedClasses['input-row']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-textarea']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-textarea']} */ ;
/** @type {__VLS_StyleScopedClasses['icon-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['mode-chip']} */ ;
/** @type {__VLS_StyleScopedClasses['history-loading-dots']} */ ;
/** @type {__VLS_StyleScopedClasses['history-loading-dots']} */ ;
/** @type {__VLS_StyleScopedClasses['history-loading-dots']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-messages']} */ ;
/** @type {__VLS_StyleScopedClasses['compact']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['compact']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-input-area']} */ ;
/** @type {__VLS_StyleScopedClasses['compact']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-textarea']} */ ;
/** @type {__VLS_StyleScopedClasses['compact']} */ ;
/** @type {__VLS_StyleScopedClasses['input-hint']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-input-area']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-textarea']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['input-hint']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['user']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['assistant']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-col']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-messages']} */ ;
/** @type {__VLS_StyleScopedClasses['tool-step-summary']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onDragenter: (__VLS_ctx.onDragEnter) },
    ...{ onDragover: () => { } },
    ...{ onDragleave: (__VLS_ctx.onDragLeave) },
    ...{ onDrop: (__VLS_ctx.handleGlobalDrop) },
    ...{ class: "ai-chat" },
    ...{ class: ({ compact: __VLS_ctx.compact, 'has-bg': __VLS_ctx.bgColor, 'drag-active': __VLS_ctx.isDragOver }) },
    ...{ style: (__VLS_ctx.rootStyle) },
});
/** @type {__VLS_StyleScopedClasses['ai-chat']} */ ;
/** @type {__VLS_StyleScopedClasses['compact']} */ ;
/** @type {__VLS_StyleScopedClasses['has-bg']} */ ;
/** @type {__VLS_StyleScopedClasses['drag-active']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.Transition | typeof __VLS_components.Transition} */
Transition;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    name: "drag-fade",
}));
const __VLS_2 = __VLS_1({
    name: "drag-fade",
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
const { default: __VLS_5 } = __VLS_3.slots;
if (__VLS_ctx.isDragOver) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drag-overlay" },
    });
    /** @type {__VLS_StyleScopedClasses['drag-overlay']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drag-overlay-content" },
    });
    /** @type {__VLS_StyleScopedClasses['drag-overlay-content']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drag-overlay-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['drag-overlay-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drag-overlay-title" },
    });
    /** @type {__VLS_StyleScopedClasses['drag-overlay-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drag-overlay-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['drag-overlay-hint']} */ ;
}
// @ts-ignore
[onDragEnter, onDragLeave, handleGlobalDrop, compact, bgColor, isDragOver, isDragOver, rootStyle,];
var __VLS_3;
if (__VLS_ctx.currentSessionId) {
    const __VLS_6 = DispatchPanel;
    // @ts-ignore
    const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({
        sessionId: (__VLS_ctx.currentSessionId),
        ref: "dispatchPanelRef",
    }));
    const __VLS_8 = __VLS_7({
        sessionId: (__VLS_ctx.currentSessionId),
        ref: "dispatchPanelRef",
    }, ...__VLS_functionalComponentArgsRest(__VLS_7));
    var __VLS_11 = {};
    var __VLS_9;
}
if (props.modelUnavailable) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "model-unavail-banner" },
    });
    /** @type {__VLS_StyleScopedClasses['model-unavail-banner']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "mub-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['mub-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "14",
        height: "14",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        'stroke-width': "2",
        'stroke-linecap': "round",
        'stroke-linejoin': "round",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M12 9v4",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
        cx: "12",
        cy: "17",
        r: ".5",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M10.3 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (props.modelUnavailable);
    let __VLS_13;
    /** @ts-ignore @type { | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link'] | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link']} */
    routerLink;
    // @ts-ignore
    const __VLS_14 = __VLS_asFunctionalComponent1(__VLS_13, new __VLS_13({
        to: "/config/models",
        ...{ class: "mub-action" },
    }));
    const __VLS_15 = __VLS_14({
        to: "/config/models",
        ...{ class: "mub-action" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_14));
    /** @type {__VLS_StyleScopedClasses['mub-action']} */ ;
    const { default: __VLS_18 } = __VLS_16.slots;
    // @ts-ignore
    [currentSessionId, currentSessionId,];
    var __VLS_16;
}
if (__VLS_ctx.runningTaskCount > 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "running-tasks-banner" },
    });
    /** @type {__VLS_StyleScopedClasses['running-tasks-banner']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
        ...{ class: "running-dot" },
    });
    /** @type {__VLS_StyleScopedClasses['running-dot']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (__VLS_ctx.runningTaskCount);
    if (__VLS_ctx.resumedTasks.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "resumed-list" },
        });
        /** @type {__VLS_StyleScopedClasses['resumed-list']} */ ;
        for (const [rt] of __VLS_vFor((__VLS_ctx.resumedTasks.filter(t => !['done', 'error', 'killed'].includes(t.status))))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                key: (rt.id),
                ...{ class: "resumed-chip" },
                ...{ class: (rt.status) },
            });
            /** @type {__VLS_StyleScopedClasses['resumed-chip']} */ ;
            (rt.status === 'running' ? '⟳' : '🟡');
            (rt.label);
            // @ts-ignore
            [runningTaskCount, runningTaskCount, resumedTasks, resumedTasks,];
        }
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onScroll: (__VLS_ctx.onMsgListScroll) },
    ...{ class: "chat-messages" },
    ref: "msgListRef",
});
/** @type {__VLS_StyleScopedClasses['chat-messages']} */ ;
if (__VLS_ctx.historyLoading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "history-loading" },
    });
    /** @type {__VLS_StyleScopedClasses['history-loading']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "history-loading-dots" },
    });
    /** @type {__VLS_StyleScopedClasses['history-loading-dots']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "history-loading-text" },
    });
    /** @type {__VLS_StyleScopedClasses['history-loading-text']} */ ;
}
if (!__VLS_ctx.messages.length && !__VLS_ctx.historyLoading && props.noModel) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-onboard" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-onboard']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-onboard-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-onboard-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-onboard-title" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-onboard-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-onboard-desc" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-onboard-desc']} */ ;
    let __VLS_19;
    /** @ts-ignore @type { | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link'] | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link']} */
    routerLink;
    // @ts-ignore
    const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
        to: "/config/models",
        ...{ class: "no-model-onboard-btn" },
    }));
    const __VLS_21 = __VLS_20({
        to: "/config/models",
        ...{ class: "no-model-onboard-btn" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_20));
    /** @type {__VLS_StyleScopedClasses['no-model-onboard-btn']} */ ;
    const { default: __VLS_24 } = __VLS_22.slots;
    // @ts-ignore
    [onMsgListScroll, historyLoading, historyLoading, messages,];
    var __VLS_22;
}
else if (!__VLS_ctx.messages.length && !__VLS_ctx.historyLoading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-empty']} */ ;
    if (__VLS_ctx.welcomeMessage) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "welcome-msg" },
        });
        /** @type {__VLS_StyleScopedClasses['welcome-msg']} */ ;
        (__VLS_ctx.welcomeMessage);
    }
    if (__VLS_ctx.examples.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "examples" },
        });
        /** @type {__VLS_StyleScopedClasses['examples']} */ ;
        for (const [ex, i] of __VLS_vFor((__VLS_ctx.examples))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ onClick: (...[$event]) => {
                        if (!!(!__VLS_ctx.messages.length && !__VLS_ctx.historyLoading && props.noModel))
                            return;
                        if (!(!__VLS_ctx.messages.length && !__VLS_ctx.historyLoading))
                            return;
                        if (!(__VLS_ctx.examples.length))
                            return;
                        __VLS_ctx.fillInput(ex);
                        // @ts-ignore
                        [historyLoading, messages, welcomeMessage, welcomeMessage, examples, examples, fillInput,];
                    } },
                key: (i),
                ...{ class: "example-chip" },
            });
            /** @type {__VLS_StyleScopedClasses['example-chip']} */ ;
            (ex);
            // @ts-ignore
            [];
        }
    }
}
for (const [msg, i] of __VLS_vFor(((__VLS_ctx.streaming ? __VLS_ctx.messages.slice(0, -1) : __VLS_ctx.messages).filter(m => (m.text?.trim() || m.images?.length || m.toolCalls?.length || m.options?.length || m.noModelError) && !__VLS_ctx.isSystemSignalMsg(m.text))))) {
    (i);
    if (msg.role === 'user' && (msg.text?.trim() || msg.images?.length)) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-row user" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
        /** @type {__VLS_StyleScopedClasses['user']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-bubble user" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
        /** @type {__VLS_StyleScopedClasses['user']} */ ;
        if (msg.images?.length) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "msg-images" },
            });
            /** @type {__VLS_StyleScopedClasses['msg-images']} */ ;
            for (const [src, j] of __VLS_vFor((msg.images))) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
                    ...{ onClick: (...[$event]) => {
                            if (!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                                return;
                            if (!(msg.images?.length))
                                return;
                            __VLS_ctx.previewImg(src);
                            // @ts-ignore
                            [messages, messages, streaming, isSystemSignalMsg, previewImg,];
                        } },
                    key: (j),
                    src: (src),
                    ...{ class: "msg-img" },
                });
                /** @type {__VLS_StyleScopedClasses['msg-img']} */ ;
                // @ts-ignore
                [];
            }
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-text" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
        (msg.text);
    }
    else if (msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-row assistant" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
        /** @type {__VLS_StyleScopedClasses['assistant']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-col" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-col']} */ ;
        if (msg.thinking) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.details, __VLS_intrinsics.details)({
                ...{ class: "thinking-block" },
                open: (__VLS_ctx.showThinking),
            });
            /** @type {__VLS_StyleScopedClasses['thinking-block']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.summary, __VLS_intrinsics.summary)({
                ...{ class: "thinking-summary" },
            });
            /** @type {__VLS_StyleScopedClasses['thinking-summary']} */ ;
            let __VLS_25;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({
                ...{ class: "thinking-icon" },
            }));
            const __VLS_27 = __VLS_26({
                ...{ class: "thinking-icon" },
            }, ...__VLS_functionalComponentArgsRest(__VLS_26));
            /** @type {__VLS_StyleScopedClasses['thinking-icon']} */ ;
            const { default: __VLS_30 } = __VLS_28.slots;
            let __VLS_31;
            /** @ts-ignore @type { | typeof __VLS_components.ChatRound} */
            ChatRound;
            // @ts-ignore
            const __VLS_32 = __VLS_asFunctionalComponent1(__VLS_31, new __VLS_31({}));
            const __VLS_33 = __VLS_32({}, ...__VLS_functionalComponentArgsRest(__VLS_32));
            // @ts-ignore
            [showThinking,];
            var __VLS_28;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "thinking-len" },
            });
            /** @type {__VLS_StyleScopedClasses['thinking-len']} */ ;
            (msg.thinking.length);
            __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
                ...{ class: "thinking-content" },
            });
            /** @type {__VLS_StyleScopedClasses['thinking-content']} */ ;
            (msg.thinking);
        }
        if (msg.toolCalls?.length) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "tool-timeline" },
            });
            /** @type {__VLS_StyleScopedClasses['tool-timeline']} */ ;
            for (const [tc, ti] of __VLS_vFor((msg.toolCalls))) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ onClick: (...[$event]) => {
                            if (!!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                                return;
                            if (!(msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)))
                                return;
                            if (!(msg.toolCalls?.length))
                                return;
                            tc._expanded = !tc._expanded;
                            // @ts-ignore
                            [];
                        } },
                    key: (ti),
                    ...{ class: "tool-step" },
                    ...{ class: (tc.status) },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "tool-step-header" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-header']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-step-dot" },
                    ...{ class: (tc.status) },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-dot']} */ ;
                if (tc.status === 'running') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-spin" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-spin']} */ ;
                }
                else if (tc.status === 'done') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                }
                else if (tc.status === 'error') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                }
                else {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                }
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-step-icon" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-icon']} */ ;
                (__VLS_ctx.toolIcon(tc.name));
                __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
                    ...{ class: "tool-step-name" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-name']} */ ;
                (tc.name);
                if (tc.input) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-step-summary" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-step-summary']} */ ;
                    (__VLS_ctx.toolSummary(tc.name, tc.input));
                }
                __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
                    ...{ class: "tool-step-flex" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-flex']} */ ;
                if (tc.duration) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-step-dur" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-step-dur']} */ ;
                    (tc.duration);
                }
                if (tc.taskId) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "task-badge" },
                        ...{ class: (tc.taskStatus) },
                    });
                    /** @type {__VLS_StyleScopedClasses['task-badge']} */ ;
                    if (tc.taskStatus === 'pending') {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                    }
                    else if (tc.taskStatus === 'running') {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                            ...{ class: "tool-spin" },
                        });
                        /** @type {__VLS_StyleScopedClasses['tool-spin']} */ ;
                        if (tc.taskStartedAt) {
                            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                                ...{ class: "task-elapsed" },
                            });
                            /** @type {__VLS_StyleScopedClasses['task-elapsed']} */ ;
                            (__VLS_ctx.fmtElapsed(tc.taskStartedAt));
                        }
                    }
                    else if (tc.taskStatus === 'done') {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                    }
                    else if (tc.taskStatus === 'error') {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                    }
                    else if (tc.taskStatus === 'killed') {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                    }
                }
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-step-chevron" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-chevron']} */ ;
                (tc._expanded ? '▲' : '▼');
                if (tc._expanded) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ onClick: () => { } },
                        ...{ class: "tool-step-body" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-step-body']} */ ;
                    if (tc.input) {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                            ...{ class: "tool-section" },
                        });
                        /** @type {__VLS_StyleScopedClasses['tool-section']} */ ;
                        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                            ...{ class: "tool-label" },
                        });
                        /** @type {__VLS_StyleScopedClasses['tool-label']} */ ;
                        __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
                            ...{ class: "tool-pre" },
                        });
                        /** @type {__VLS_StyleScopedClasses['tool-pre']} */ ;
                        (__VLS_ctx.fmtJson(tc.input));
                    }
                    if (tc.result) {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                            ...{ class: "tool-section" },
                        });
                        /** @type {__VLS_StyleScopedClasses['tool-section']} */ ;
                        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                            ...{ class: "tool-label" },
                        });
                        /** @type {__VLS_StyleScopedClasses['tool-label']} */ ;
                        __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
                            ...{ class: "tool-pre result" },
                        });
                        /** @type {__VLS_StyleScopedClasses['tool-pre']} */ ;
                        /** @type {__VLS_StyleScopedClasses['result']} */ ;
                        (tc.result.slice(0, 3000));
                        (tc.result.length > 3000 ? '\n… (截断)' : '');
                    }
                }
                if (tc.mediaUrl) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ onClick: () => { } },
                        ...{ class: "tool-media-preview" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-media-preview']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
                        ...{ onClick: (...[$event]) => {
                                if (!!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                                    return;
                                if (!(msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)))
                                    return;
                                if (!(msg.toolCalls?.length))
                                    return;
                                if (!(tc.mediaUrl))
                                    return;
                                __VLS_ctx.previewImg(tc.mediaUrl);
                                // @ts-ignore
                                [previewImg, toolIcon, toolSummary, fmtElapsed, fmtJson,];
                            } },
                        src: (tc.mediaUrl),
                        ...{ class: "tool-media-img" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-media-img']} */ ;
                }
                if (tc.fileCard) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ onClick: () => { } },
                        ...{ class: "tool-file-card" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-file-card']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
                        href: (tc.fileCard.url),
                        target: "_blank",
                        download: true,
                        ...{ class: "tool-file-link" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-file-link']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-file-icon" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-file-icon']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-file-name" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-file-name']} */ ;
                    (tc.fileCard.name);
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-file-size" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-file-size']} */ ;
                    (tc.fileCard.size);
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-file-dl" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-file-dl']} */ ;
                }
                // @ts-ignore
                [];
            }
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-bubble assistant" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
        /** @type {__VLS_StyleScopedClasses['assistant']} */ ;
        if (msg.noModelError) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "no-model-card" },
            });
            /** @type {__VLS_StyleScopedClasses['no-model-card']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "no-model-icon" },
            });
            /** @type {__VLS_StyleScopedClasses['no-model-icon']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "no-model-body" },
            });
            /** @type {__VLS_StyleScopedClasses['no-model-body']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "no-model-title" },
            });
            /** @type {__VLS_StyleScopedClasses['no-model-title']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "no-model-desc" },
            });
            /** @type {__VLS_StyleScopedClasses['no-model-desc']} */ ;
            let __VLS_36;
            /** @ts-ignore @type { | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link'] | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link']} */
            routerLink;
            // @ts-ignore
            const __VLS_37 = __VLS_asFunctionalComponent1(__VLS_36, new __VLS_36({
                to: "/config/models",
                ...{ class: "no-model-btn" },
            }));
            const __VLS_38 = __VLS_37({
                to: "/config/models",
                ...{ class: "no-model-btn" },
            }, ...__VLS_functionalComponentArgsRest(__VLS_37));
            /** @type {__VLS_StyleScopedClasses['no-model-btn']} */ ;
            const { default: __VLS_41 } = __VLS_39.slots;
            // @ts-ignore
            [];
            var __VLS_39;
        }
        else if (msg.text) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
                ...{ class: "msg-text" },
            });
            __VLS_asFunctionalDirective(__VLS_directives.vHtml, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.renderMd(msg.text)) }, null, null);
            /** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
        }
        if (msg.truncatedByError) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "truncated-footer" },
            });
            /** @type {__VLS_StyleScopedClasses['truncated-footer']} */ ;
            let __VLS_42;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_43 = __VLS_asFunctionalComponent1(__VLS_42, new __VLS_42({}));
            const __VLS_44 = __VLS_43({}, ...__VLS_functionalComponentArgsRest(__VLS_43));
            const { default: __VLS_47 } = __VLS_45.slots;
            let __VLS_48;
            /** @ts-ignore @type { | typeof __VLS_components.Warning} */
            Warning;
            // @ts-ignore
            const __VLS_49 = __VLS_asFunctionalComponent1(__VLS_48, new __VLS_48({}));
            const __VLS_50 = __VLS_49({}, ...__VLS_functionalComponentArgsRest(__VLS_49));
            // @ts-ignore
            [renderMd,];
            var __VLS_45;
        }
        if (msg.applyData && props.applyable) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "apply-card" },
            });
            /** @type {__VLS_StyleScopedClasses['apply-card']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "apply-preview" },
            });
            /** @type {__VLS_StyleScopedClasses['apply-preview']} */ ;
            for (const [val, key] of __VLS_vFor((msg.applyData))) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    key: (key),
                    ...{ class: "apply-row" },
                });
                /** @type {__VLS_StyleScopedClasses['apply-row']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "apply-key" },
                });
                /** @type {__VLS_StyleScopedClasses['apply-key']} */ ;
                (key);
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "apply-val" },
                });
                /** @type {__VLS_StyleScopedClasses['apply-val']} */ ;
                (String(val).slice(0, 60));
                (String(val).length > 60 ? '…' : '');
                // @ts-ignore
                [];
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                ...{ onClick: (...[$event]) => {
                        if (!!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                            return;
                        if (!(msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)))
                            return;
                        if (!(msg.applyData && props.applyable))
                            return;
                        __VLS_ctx.$emit('apply', msg.applyData);
                        // @ts-ignore
                        [$emit,];
                    } },
                ...{ class: "apply-btn" },
            });
            /** @type {__VLS_StyleScopedClasses['apply-btn']} */ ;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-actions" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-actions']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
            ...{ onClick: (...[$event]) => {
                    if (!!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                        return;
                    if (!(msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)))
                        return;
                    __VLS_ctx.copyMsg(msg.text);
                    // @ts-ignore
                    [copyMsg,];
                } },
            ...{ class: "act-btn" },
            title: (__VLS_ctx.copied === i ? '已复制' : '复制'),
        });
        /** @type {__VLS_StyleScopedClasses['act-btn']} */ ;
        if (__VLS_ctx.copied === i) {
            let __VLS_53;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_54 = __VLS_asFunctionalComponent1(__VLS_53, new __VLS_53({}));
            const __VLS_55 = __VLS_54({}, ...__VLS_functionalComponentArgsRest(__VLS_54));
            const { default: __VLS_58 } = __VLS_56.slots;
            let __VLS_59;
            /** @ts-ignore @type { | typeof __VLS_components.Check} */
            Check;
            // @ts-ignore
            const __VLS_60 = __VLS_asFunctionalComponent1(__VLS_59, new __VLS_59({}));
            const __VLS_61 = __VLS_60({}, ...__VLS_functionalComponentArgsRest(__VLS_60));
            // @ts-ignore
            [copied, copied,];
            var __VLS_56;
        }
        else {
            let __VLS_64;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_65 = __VLS_asFunctionalComponent1(__VLS_64, new __VLS_64({}));
            const __VLS_66 = __VLS_65({}, ...__VLS_functionalComponentArgsRest(__VLS_65));
            const { default: __VLS_69 } = __VLS_67.slots;
            let __VLS_70;
            /** @ts-ignore @type { | typeof __VLS_components.CopyDocument} */
            CopyDocument;
            // @ts-ignore
            const __VLS_71 = __VLS_asFunctionalComponent1(__VLS_70, new __VLS_70({}));
            const __VLS_72 = __VLS_71({}, ...__VLS_functionalComponentArgsRest(__VLS_71));
            // @ts-ignore
            [];
            var __VLS_67;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
            ...{ onClick: (...[$event]) => {
                    if (!!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                        return;
                    if (!(msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)))
                        return;
                    __VLS_ctx.retryMsg(i);
                    // @ts-ignore
                    [retryMsg,];
                } },
            ...{ class: "act-btn" },
            title: "重试",
        });
        /** @type {__VLS_StyleScopedClasses['act-btn']} */ ;
        if (props.applyable && !msg.applyData && __VLS_ctx.hasJsonBlock(msg.text)) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                ...{ onClick: (...[$event]) => {
                        if (!!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                            return;
                        if (!(msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)))
                            return;
                        if (!(props.applyable && !msg.applyData && __VLS_ctx.hasJsonBlock(msg.text)))
                            return;
                        __VLS_ctx.manualApply(msg);
                        // @ts-ignore
                        [hasJsonBlock, manualApply,];
                    } },
                ...{ class: "act-btn apply-manual-btn" },
                title: "检测到配置 JSON，点击应用",
            });
            /** @type {__VLS_StyleScopedClasses['act-btn']} */ ;
            /** @type {__VLS_StyleScopedClasses['apply-manual-btn']} */ ;
            let __VLS_75;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_76 = __VLS_asFunctionalComponent1(__VLS_75, new __VLS_75({}));
            const __VLS_77 = __VLS_76({}, ...__VLS_functionalComponentArgsRest(__VLS_76));
            const { default: __VLS_80 } = __VLS_78.slots;
            let __VLS_81;
            /** @ts-ignore @type { | typeof __VLS_components.Setting} */
            Setting;
            // @ts-ignore
            const __VLS_82 = __VLS_asFunctionalComponent1(__VLS_81, new __VLS_81({}));
            const __VLS_83 = __VLS_82({}, ...__VLS_functionalComponentArgsRest(__VLS_82));
            // @ts-ignore
            [];
            var __VLS_78;
        }
        if (msg.tokenUsage) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "msg-token-usage" },
            });
            /** @type {__VLS_StyleScopedClasses['msg-token-usage']} */ ;
            (msg.tokenUsage.input.toLocaleString());
            (msg.tokenUsage.output.toLocaleString());
        }
        if (msg.options && msg.options.length) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "option-chips" },
            });
            /** @type {__VLS_StyleScopedClasses['option-chips']} */ ;
            for (const [opt, oi] of __VLS_vFor((msg.options))) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                    ...{ onClick: (...[$event]) => {
                            if (!!(msg.role === 'user' && (msg.text?.trim() || msg.images?.length)))
                                return;
                            if (!(msg.role === 'assistant' && (msg.text?.trim() || msg.toolCalls?.length || msg.thinking)))
                                return;
                            if (!(msg.options && msg.options.length))
                                return;
                            __VLS_ctx.fillInput(opt);
                            // @ts-ignore
                            [fillInput,];
                        } },
                    key: (oi),
                    ...{ class: "option-chip" },
                });
                /** @type {__VLS_StyleScopedClasses['option-chip']} */ ;
                (opt);
                // @ts-ignore
                [];
            }
        }
    }
    else if (msg.role === 'system') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-row system" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
        /** @type {__VLS_StyleScopedClasses['system']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: (['msg-system', msg.sysKind === 'error' ? 'is-error' : '']) },
        });
        /** @type {__VLS_StyleScopedClasses['msg-system']} */ ;
        if (msg.sysKind === 'error') {
            let __VLS_86;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_87 = __VLS_asFunctionalComponent1(__VLS_86, new __VLS_86({
                ...{ class: "sys-icon" },
            }));
            const __VLS_88 = __VLS_87({
                ...{ class: "sys-icon" },
            }, ...__VLS_functionalComponentArgsRest(__VLS_87));
            /** @type {__VLS_StyleScopedClasses['sys-icon']} */ ;
            const { default: __VLS_91 } = __VLS_89.slots;
            let __VLS_92;
            /** @ts-ignore @type { | typeof __VLS_components.CircleCloseFilled} */
            CircleCloseFilled;
            // @ts-ignore
            const __VLS_93 = __VLS_asFunctionalComponent1(__VLS_92, new __VLS_92({}));
            const __VLS_94 = __VLS_93({}, ...__VLS_functionalComponentArgsRest(__VLS_93));
            // @ts-ignore
            [];
            var __VLS_89;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        (msg.text);
    }
    // @ts-ignore
    [];
}
if (__VLS_ctx.streaming) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "msg-row assistant" },
    });
    /** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
    /** @type {__VLS_StyleScopedClasses['assistant']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "msg-col" },
    });
    /** @type {__VLS_StyleScopedClasses['msg-col']} */ ;
    if (__VLS_ctx.streamThinking && __VLS_ctx.showThinking) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.details, __VLS_intrinsics.details)({
            ...{ class: "thinking-block" },
            open: true,
        });
        /** @type {__VLS_StyleScopedClasses['thinking-block']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.summary, __VLS_intrinsics.summary)({
            ...{ class: "thinking-summary" },
        });
        /** @type {__VLS_StyleScopedClasses['thinking-summary']} */ ;
        let __VLS_97;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_98 = __VLS_asFunctionalComponent1(__VLS_97, new __VLS_97({
            ...{ class: "thinking-icon" },
        }));
        const __VLS_99 = __VLS_98({
            ...{ class: "thinking-icon" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_98));
        /** @type {__VLS_StyleScopedClasses['thinking-icon']} */ ;
        const { default: __VLS_102 } = __VLS_100.slots;
        let __VLS_103;
        /** @ts-ignore @type { | typeof __VLS_components.ChatRound} */
        ChatRound;
        // @ts-ignore
        const __VLS_104 = __VLS_asFunctionalComponent1(__VLS_103, new __VLS_103({}));
        const __VLS_105 = __VLS_104({}, ...__VLS_functionalComponentArgsRest(__VLS_104));
        // @ts-ignore
        [streaming, showThinking, streamThinking,];
        var __VLS_100;
        __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
            ...{ class: "thinking-content" },
        });
        /** @type {__VLS_StyleScopedClasses['thinking-content']} */ ;
        (__VLS_ctx.streamThinking);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "blink" },
        });
        /** @type {__VLS_StyleScopedClasses['blink']} */ ;
    }
    if (__VLS_ctx.streamToolCalls.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "tool-timeline" },
        });
        /** @type {__VLS_StyleScopedClasses['tool-timeline']} */ ;
        for (const [tc, ti] of __VLS_vFor((__VLS_ctx.streamToolCalls))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ onClick: (...[$event]) => {
                        if (!(__VLS_ctx.streaming))
                            return;
                        if (!(__VLS_ctx.streamToolCalls.length))
                            return;
                        tc._expanded = !tc._expanded;
                        // @ts-ignore
                        [streamThinking, streamToolCalls, streamToolCalls,];
                    } },
                key: (ti),
                ...{ class: "tool-step" },
                ...{ class: (tc.status) },
            });
            /** @type {__VLS_StyleScopedClasses['tool-step']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "tool-step-header" },
            });
            /** @type {__VLS_StyleScopedClasses['tool-step-header']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "tool-step-dot" },
                ...{ class: (tc.status) },
            });
            /** @type {__VLS_StyleScopedClasses['tool-step-dot']} */ ;
            if (tc.status === 'running') {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-spin" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-spin']} */ ;
            }
            else if (tc.status === 'done') {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
            }
            else if (tc.status === 'error') {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "tool-step-icon" },
            });
            /** @type {__VLS_StyleScopedClasses['tool-step-icon']} */ ;
            (__VLS_ctx.toolIcon(tc.name));
            __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
                ...{ class: "tool-step-name" },
            });
            /** @type {__VLS_StyleScopedClasses['tool-step-name']} */ ;
            (tc.name);
            if (tc.input) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-step-summary" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-summary']} */ ;
                (__VLS_ctx.toolSummary(tc.name, tc.input));
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
                ...{ class: "tool-step-flex" },
            });
            /** @type {__VLS_StyleScopedClasses['tool-step-flex']} */ ;
            if (tc.duration) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-step-dur" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-dur']} */ ;
                (tc.duration);
            }
            if (tc.taskId) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "task-badge" },
                    ...{ class: (tc.taskStatus) },
                });
                /** @type {__VLS_StyleScopedClasses['task-badge']} */ ;
                if (tc.taskStatus === 'pending') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                }
                else if (tc.taskStatus === 'running') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "tool-spin" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-spin']} */ ;
                    if (tc.taskStartedAt) {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                            ...{ class: "task-elapsed" },
                        });
                        /** @type {__VLS_StyleScopedClasses['task-elapsed']} */ ;
                        (__VLS_ctx.fmtElapsed(tc.taskStartedAt));
                    }
                }
                else if (tc.taskStatus === 'done') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                }
                else if (tc.taskStatus === 'error') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                }
                else if (tc.taskStatus === 'killed') {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
                }
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "tool-step-chevron" },
            });
            /** @type {__VLS_StyleScopedClasses['tool-step-chevron']} */ ;
            (tc._expanded ? '▲' : '▼');
            if (tc._expanded) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ onClick: () => { } },
                    ...{ class: "tool-step-body" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-step-body']} */ ;
                if (tc.input) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ class: "tool-section" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-section']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ class: "tool-label" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-label']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
                        ...{ class: "tool-pre" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-pre']} */ ;
                    (__VLS_ctx.fmtJson(tc.input));
                }
                if (tc.result) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ class: "tool-section" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-section']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ class: "tool-label" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-label']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
                        ...{ class: "tool-pre result" },
                    });
                    /** @type {__VLS_StyleScopedClasses['tool-pre']} */ ;
                    /** @type {__VLS_StyleScopedClasses['result']} */ ;
                    (tc.result.slice(0, 3000));
                }
            }
            if (tc.mediaUrl) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ onClick: () => { } },
                    ...{ class: "tool-media-preview" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-media-preview']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
                    ...{ onClick: (...[$event]) => {
                            if (!(__VLS_ctx.streaming))
                                return;
                            if (!(__VLS_ctx.streamToolCalls.length))
                                return;
                            if (!(tc.mediaUrl))
                                return;
                            __VLS_ctx.previewImg(tc.mediaUrl);
                            // @ts-ignore
                            [previewImg, toolIcon, toolSummary, fmtElapsed, fmtJson,];
                        } },
                    src: (tc.mediaUrl),
                    ...{ class: "tool-media-img" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-media-img']} */ ;
            }
            if (tc.fileCard) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ onClick: () => { } },
                    ...{ class: "tool-file-card" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-file-card']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
                    href: (tc.fileCard.url),
                    target: "_blank",
                    download: true,
                    ...{ class: "tool-file-link" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-file-link']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-file-icon" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-file-icon']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-file-name" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-file-name']} */ ;
                (tc.fileCard.name);
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-file-size" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-file-size']} */ ;
                (tc.fileCard.size);
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tool-file-dl" },
                });
                /** @type {__VLS_StyleScopedClasses['tool-file-dl']} */ ;
            }
            // @ts-ignore
            [];
        }
    }
    if (__VLS_ctx.streamText || !__VLS_ctx.streamToolCalls.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-bubble assistant" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
        /** @type {__VLS_StyleScopedClasses['assistant']} */ ;
        if (!__VLS_ctx.streamText && !__VLS_ctx.streamToolCalls.length) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "typing-dots" },
            });
            /** @type {__VLS_StyleScopedClasses['typing-dots']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span)({});
            __VLS_asFunctionalElement1(__VLS_intrinsics.span)({});
            __VLS_asFunctionalElement1(__VLS_intrinsics.span)({});
        }
        if (__VLS_ctx.streamText) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
                ...{ class: "msg-text" },
            });
            __VLS_asFunctionalDirective(__VLS_directives.vHtml, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.renderMd(__VLS_ctx.streamText)) }, null, null);
            /** @type {__VLS_StyleScopedClasses['msg-text']} */ ;
        }
        if (__VLS_ctx.streamText) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "blink" },
            });
            /** @type {__VLS_StyleScopedClasses['blink']} */ ;
        }
    }
}
if (__VLS_ctx.previewSrc) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.previewSrc))
                    return;
                __VLS_ctx.previewSrc = '';
                // @ts-ignore
                [renderMd, streamToolCalls, streamToolCalls, streamText, streamText, streamText, streamText, streamText, previewSrc, previewSrc,];
            } },
        ...{ class: "img-preview-mask" },
    });
    /** @type {__VLS_StyleScopedClasses['img-preview-mask']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
        src: (__VLS_ctx.previewSrc),
        ...{ class: "img-preview-full" },
    });
    /** @type {__VLS_StyleScopedClasses['img-preview-full']} */ ;
}
if (props.readOnly) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "readonly-banner" },
    });
    /** @type {__VLS_StyleScopedClasses['readonly-banner']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "readonly-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['readonly-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "14",
        height: "14",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        'stroke-width': "2",
        'stroke-linecap': "round",
        'stroke-linejoin': "round",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.rect)({
        x: "3",
        y: "11",
        width: "18",
        height: "11",
        rx: "2",
        ry: "2",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M7 11V7a5 5 0 0 1 10 0v4",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (props.readOnlyReason || '此对话来自外部渠道，仅可查看历史，不支持在面板中回复');
}
if (!props.readOnly) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-input-area" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-input-area']} */ ;
    if (__VLS_ctx.pendingImages.length || __VLS_ctx.pendingFiles.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "attachments-bar" },
        });
        /** @type {__VLS_StyleScopedClasses['attachments-bar']} */ ;
        for (const [src, i] of __VLS_vFor((__VLS_ctx.pendingImages))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: ('img-' + i),
                ...{ class: "attach-thumb" },
            });
            /** @type {__VLS_StyleScopedClasses['attach-thumb']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
                src: (src),
            });
            __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                ...{ onClick: (...[$event]) => {
                        if (!(!props.readOnly))
                            return;
                        if (!(__VLS_ctx.pendingImages.length || __VLS_ctx.pendingFiles.length))
                            return;
                        __VLS_ctx.removeImage(i);
                        // @ts-ignore
                        [previewSrc, pendingImages, pendingImages, pendingFiles, removeImage,];
                    } },
                ...{ class: "remove-attach" },
            });
            /** @type {__VLS_StyleScopedClasses['remove-attach']} */ ;
            // @ts-ignore
            [];
        }
        for (const [f, i] of __VLS_vFor((__VLS_ctx.pendingFiles))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: ('file-' + i),
                ...{ class: (['attach-file-chip', { 'attach-file-uploading': f.uploading, 'attach-file-error': f.uploadError }]) },
            });
            /** @type {__VLS_StyleScopedClasses['attach-file-chip']} */ ;
            /** @type {__VLS_StyleScopedClasses['attach-file-uploading']} */ ;
            /** @type {__VLS_StyleScopedClasses['attach-file-error']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "attach-file-icon" },
            });
            /** @type {__VLS_StyleScopedClasses['attach-file-icon']} */ ;
            (f.uploading ? '⏳' : f.uploadError ? '❌' : __VLS_ctx.fileTypeIcon(f.name));
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "attach-file-name" },
            });
            /** @type {__VLS_StyleScopedClasses['attach-file-name']} */ ;
            (f.name);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "attach-file-size" },
            });
            /** @type {__VLS_StyleScopedClasses['attach-file-size']} */ ;
            (f.uploading ? '上传中…' : f.uploadError ? f.uploadError : f.size ? __VLS_ctx.formatFileSize(f.size) : __VLS_ctx.formatFileSize(f.content.length));
            if (!f.uploading) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                    ...{ onClick: (...[$event]) => {
                            if (!(!props.readOnly))
                                return;
                            if (!(__VLS_ctx.pendingImages.length || __VLS_ctx.pendingFiles.length))
                                return;
                            if (!(!f.uploading))
                                return;
                            __VLS_ctx.pendingFiles.splice(i, 1);
                            // @ts-ignore
                            [pendingFiles, pendingFiles, fileTypeIcon, formatFileSize, formatFileSize,];
                        } },
                    ...{ class: "attach-file-remove" },
                });
                /** @type {__VLS_StyleScopedClasses['attach-file-remove']} */ ;
            }
            // @ts-ignore
            [];
        }
    }
    if (!__VLS_ctx.inputText && !__VLS_ctx.pendingImages.length && !__VLS_ctx.pendingFiles.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "mode-chips" },
        });
        /** @type {__VLS_StyleScopedClasses['mode-chips']} */ ;
        for (const [chip] of __VLS_vFor((__VLS_ctx.modeChips))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ onClick: (...[$event]) => {
                        if (!(!props.readOnly))
                            return;
                        if (!(!__VLS_ctx.inputText && !__VLS_ctx.pendingImages.length && !__VLS_ctx.pendingFiles.length))
                            return;
                        __VLS_ctx.appendModeChip(chip.tag);
                        // @ts-ignore
                        [pendingImages, pendingFiles, inputText, modeChips, appendModeChip,];
                    } },
                key: (chip.tag),
                ...{ class: "mode-chip" },
                title: (chip.hint),
            });
            /** @type {__VLS_StyleScopedClasses['mode-chip']} */ ;
            (chip.tag);
            // @ts-ignore
            [];
        }
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "input-row" },
    });
    /** @type {__VLS_StyleScopedClasses['input-row']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "textarea-wrap" },
    });
    /** @type {__VLS_StyleScopedClasses['textarea-wrap']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.textarea)({
        ...{ onKeydown: (__VLS_ctx.onTextareaKeydown) },
        ...{ onPaste: (__VLS_ctx.handlePaste) },
        ...{ onInput: (__VLS_ctx.autoGrow) },
        ref: "inputRef",
        value: (__VLS_ctx.inputText),
        placeholder: (props.noModel ? '请先配置 AI 模型才能开始对话…' : (__VLS_ctx.placeholder || '输入消息… (Enter 发送 · Shift+Enter 换行 · 支持拖拽图片/文件)')),
        disabled: (__VLS_ctx.streaming || __VLS_ctx.historyLoading || props.noModel),
        rows: "1",
        ...{ class: "chat-textarea" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-textarea']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "input-actions" },
    });
    /** @type {__VLS_StyleScopedClasses['input-actions']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
        ...{ class: "icon-btn" },
        title: "附加文件（图片/代码/文本）",
    });
    /** @type {__VLS_StyleScopedClasses['icon-btn']} */ ;
    let __VLS_108;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_109 = __VLS_asFunctionalComponent1(__VLS_108, new __VLS_108({}));
    const __VLS_110 = __VLS_109({}, ...__VLS_functionalComponentArgsRest(__VLS_109));
    const { default: __VLS_113 } = __VLS_111.slots;
    let __VLS_114;
    /** @ts-ignore @type { | typeof __VLS_components.Paperclip} */
    Paperclip;
    // @ts-ignore
    const __VLS_115 = __VLS_asFunctionalComponent1(__VLS_114, new __VLS_114({}));
    const __VLS_116 = __VLS_115({}, ...__VLS_functionalComponentArgsRest(__VLS_115));
    // @ts-ignore
    [historyLoading, streaming, inputText, onTextareaKeydown, handlePaste, autoGrow, placeholder,];
    var __VLS_111;
    __VLS_asFunctionalElement1(__VLS_intrinsics.input)({
        ...{ onChange: (__VLS_ctx.handleFileSelect) },
        type: "file",
        multiple: true,
        hidden: true,
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.send) },
        ...{ class: "send-btn" },
        disabled: (__VLS_ctx.streaming || __VLS_ctx.historyLoading || props.noModel || (!__VLS_ctx.inputText.trim() && !__VLS_ctx.pendingImages.length && !__VLS_ctx.pendingFiles.length) || __VLS_ctx.pendingFiles.some(f => f.uploading)),
    });
    /** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
    if (__VLS_ctx.streaming) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
            ...{ class: "spinner" },
        });
        /** @type {__VLS_StyleScopedClasses['spinner']} */ ;
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "input-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['input-hint']} */ ;
}
// @ts-ignore
var __VLS_12 = __VLS_11;
// @ts-ignore
[historyLoading, streaming, streaming, pendingImages, pendingFiles, pendingFiles, inputText, handleFileSelect, send,];
const __VLS_export = (await import('vue')).defineComponent({
    setup: () => __VLS_exposed,
    __typeEmits: {},
    __defaults: __VLS_defaults,
    __typeProps: {},
});
export default {};
