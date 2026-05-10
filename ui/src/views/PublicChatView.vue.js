/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, nextTick } from 'vue';
import { useRoute } from 'vue-router';
const route = useRoute();
const agentId = route.params.agentId;
const channelId = route.params.channelId; // undefined on legacy route
// Build API path prefix: /pub/chat/:agentId/:channelId  OR  /pub/chat/:agentId (legacy)
const apiBase = channelId
    ? `/pub/chat/${agentId}/${channelId}`
    : `/pub/chat/${agentId}`;
// Per-channel session token stored in localStorage — identifies this browser/visitor.
// Enables server-side conversation history + memory compaction per visitor.
function getOrCreateSessionToken() {
    const key = `chat-session-${agentId}-${channelId || 'default'}`;
    let token = localStorage.getItem(key);
    if (!token) {
        token = Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10);
        localStorage.setItem(key, token);
    }
    return token;
}
const sessionToken = getOrCreateSessionToken();
const info = ref(null);
const infoLoaded = ref(false);
const channelDeleted = ref(false);
const needPassword = ref(false);
const authed = ref(false);
const passwordInput = ref('');
const passwordError = ref(false);
const password = ref('');
// Server-side session ID — persisted in localStorage for reconnect across page refreshes.
const sessionIdKey = `pub-session-id-${agentId}-${channelId || 'default'}`;
const currentSessionId = ref(localStorage.getItem(sessionIdKey) ?? '');
const messages = ref([]);
const inputText = ref('');
const streaming = ref(false);
const streamingText = ref('');
const messagesRef = ref();
const inputRef = ref();
const initial = computed(() => (info.value?.name || '?').charAt(0).toUpperCase());
// Password storage key scoped to channel
const pwStorageKey = `chat-pw-${agentId}-${channelId || 'default'}`;
async function loadInfo() {
    try {
        const res = await fetch(`${apiBase}/info`);
        if (res.status === 404 || res.status === 410) {
            channelDeleted.value = true;
            infoLoaded.value = false;
            return;
        }
        if (!res.ok) {
            infoLoaded.value = true;
            return;
        }
        const data = await res.json();
        info.value = data;
        if (data.title)
            document.title = data.title;
        needPassword.value = data.hasPassword;
        if (!data.hasPassword) {
            authed.value = true;
            await loadHistory();
        }
        else {
            const saved = sessionStorage.getItem(pwStorageKey);
            if (saved) {
                password.value = saved;
                authed.value = true;
                await loadHistory();
            }
            else {
                authed.value = false;
            }
        }
        infoLoaded.value = true;
    }
    catch {
        infoLoaded.value = true;
    }
}
// Load history from server on mount (or after password auth).
async function loadHistory() {
    if (!sessionToken)
        return;
    try {
        const params = new URLSearchParams({ sessionToken });
        const headers = {};
        if (password.value)
            headers['X-Chat-Password'] = password.value;
        const res = await fetch(`${apiBase}/history?${params}`, { headers });
        if (!res.ok)
            return;
        const data = await res.json();
        if (data.sessionId)
            currentSessionId.value = data.sessionId;
        if (Array.isArray(data.messages) && data.messages.length > 0) {
            messages.value = data.messages.map((m) => ({ role: m.role, content: m.content }));
            await scrollBottom();
        }
    }
    catch { /* no history yet */ }
}
async function submitPassword() {
    if (!passwordInput.value)
        return;
    const pw = passwordInput.value;
    try {
        const res = await fetch(`${apiBase}/info`, {
            headers: { 'X-Chat-Password': pw },
        });
        if (res.status === 401) {
            passwordError.value = true;
            return;
        }
    }
    catch {
        // Network error — proceed optimistically
    }
    password.value = pw;
    sessionStorage.setItem(pwStorageKey, pw);
    authed.value = true;
    passwordError.value = false;
    await loadHistory();
}
async function sendMessage() {
    const text = inputText.value.trim();
    if (!text || streaming.value)
        return;
    inputText.value = '';
    nextTick(() => autoResize());
    messages.value.push({ role: 'user', content: text });
    await scrollBottom();
    await streamResponse(text);
}
// consumeSSE reads an SSE stream and processes events.
// Returns true if generation completed normally.
async function consumeSSE(res) {
    if (!res.body)
        return false;
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    let done = false;
    while (true) {
        const { done: streamDone, value } = await reader.read();
        if (streamDone)
            break;
        buf += decoder.decode(value, { stream: true });
        const parts = buf.split('\n\n');
        buf = parts.pop() ?? '';
        for (const part of parts) {
            const line = part.startsWith('data: ') ? part.slice(6) : part;
            if (!line.trim())
                continue;
            try {
                const ev = JSON.parse(line);
                if (ev.type === 'text_delta') {
                    streamingText.value += ev.text;
                    await scrollBottom();
                }
                else if (ev.type === 'done') {
                    if (ev.sessionId) {
                        currentSessionId.value = ev.sessionId;
                        localStorage.setItem(sessionIdKey, ev.sessionId);
                    }
                    done = true;
                    break;
                }
                else if (ev.type === 'idle') {
                    done = true;
                    break;
                }
                else if (ev.type === 'error') {
                    streamingText.value += `\n[错误: ${ev.error ?? ev.text}]`;
                }
            }
            catch { }
        }
        if (done)
            break;
    }
    return done;
}
async function streamResponse(message) {
    streaming.value = true;
    streamingText.value = '';
    const headers = { 'Content-Type': 'application/json' };
    if (password.value)
        headers['X-Chat-Password'] = password.value;
    try {
        const res = await fetch(`${apiBase}/stream`, {
            method: 'POST',
            headers,
            body: JSON.stringify({ message, sessionToken }),
        });
        if (res.status === 401) {
            password.value = '';
            sessionStorage.removeItem(pwStorageKey);
            authed.value = false;
            passwordError.value = true;
            streaming.value = false;
            streamingText.value = '';
            messages.value.pop();
            return;
        }
        if (res.status === 410) {
            channelDeleted.value = true;
            infoLoaded.value = false;
            return;
        }
        if (!res.ok)
            throw new Error('Request failed');
        await consumeSSE(res);
    }
    catch {
        streamingText.value += '\n[连接错误，请重试]';
    }
    finally {
        if (streamingText.value) {
            messages.value.push({ role: 'assistant', content: streamingText.value });
        }
        streaming.value = false;
        streamingText.value = '';
        await scrollBottom();
    }
}
// reconnectIfGenerating checks if there's an in-progress generation for our session
// and subscribes to receive its output (handles page refresh mid-generation).
async function reconnectIfGenerating() {
    if (!currentSessionId.value)
        return;
    const headers = {};
    if (password.value)
        headers['X-Chat-Password'] = password.value;
    try {
        const res = await fetch(`${apiBase}/reconnect?sessionId=${encodeURIComponent(currentSessionId.value)}`, { headers });
        if (!res.ok)
            return;
        streaming.value = true;
        streamingText.value = '';
        await consumeSSE(res);
        if (streamingText.value) {
            // Check if last message is the one being generated (avoid duplicate)
            const last = messages.value[messages.value.length - 1];
            if (!last || last.role !== 'assistant' || last.content !== streamingText.value) {
                messages.value.push({ role: 'assistant', content: streamingText.value });
            }
        }
        streaming.value = false;
        streamingText.value = '';
        await scrollBottom();
        // Refresh history to ensure everything is up to date
        await loadHistory();
    }
    catch { /* not generating */ }
}
async function scrollBottom() {
    await nextTick();
    if (messagesRef.value) {
        messagesRef.value.scrollTop = messagesRef.value.scrollHeight;
    }
}
function autoResize() {
    if (!inputRef.value)
        return;
    inputRef.value.style.height = 'auto';
    inputRef.value.style.height = Math.min(inputRef.value.scrollHeight, 140) + 'px';
}
function renderText(text) {
    return text
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/`([^`]+)`/g, '<code>$1</code>')
        .replace(/\n/g, '<br>');
}
onMounted(async () => {
    await loadInfo();
    // If a generation was in progress when the page was closed, reconnect to receive it
    if (authed.value && currentSessionId.value) {
        await reconnectIfGenerating();
    }
});
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['gate-input']} */ ;
/** @type {__VLS_StyleScopedClasses['gate-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['gate-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['user']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['input-box']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-footer-link']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
/** @type {__VLS_StyleScopedClasses['user']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "pub-chat-page" },
});
/** @type {__VLS_StyleScopedClasses['pub-chat-page']} */ ;
if (__VLS_ctx.needPassword && !__VLS_ctx.authed) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "password-gate" },
    });
    /** @type {__VLS_StyleScopedClasses['password-gate']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gate-card" },
    });
    /** @type {__VLS_StyleScopedClasses['gate-card']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "gate-avatar" },
        ...{ style: ({ background: __VLS_ctx.info?.avatarColor || '#409EFF' }) },
    });
    /** @type {__VLS_StyleScopedClasses['gate-avatar']} */ ;
    (__VLS_ctx.initial);
    __VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
        ...{ class: "gate-name" },
    });
    /** @type {__VLS_StyleScopedClasses['gate-name']} */ ;
    (__VLS_ctx.info?.name || '...');
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
        ...{ class: "gate-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['gate-hint']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.input)({
        ...{ onKeydown: (__VLS_ctx.submitPassword) },
        type: "password",
        placeholder: "请输入密码",
        ...{ class: "gate-input" },
    });
    (__VLS_ctx.passwordInput);
    /** @type {__VLS_StyleScopedClasses['gate-input']} */ ;
    if (__VLS_ctx.passwordError) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "gate-error" },
        });
        /** @type {__VLS_StyleScopedClasses['gate-error']} */ ;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.submitPassword) },
        ...{ class: "gate-btn" },
        disabled: (!__VLS_ctx.passwordInput),
    });
    /** @type {__VLS_StyleScopedClasses['gate-btn']} */ ;
}
else if (__VLS_ctx.channelDeleted) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "closed-page" },
    });
    /** @type {__VLS_StyleScopedClasses['closed-page']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "closed-card" },
    });
    /** @type {__VLS_StyleScopedClasses['closed-card']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "closed-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['closed-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
        ...{ class: "closed-title" },
    });
    /** @type {__VLS_StyleScopedClasses['closed-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
        ...{ class: "closed-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['closed-hint']} */ ;
}
else if (__VLS_ctx.infoLoaded) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-page" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-page']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-header" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-header-left" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-header-left']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "header-avatar" },
        ...{ style: ({ background: __VLS_ctx.info?.avatarColor || '#409EFF' }) },
    });
    /** @type {__VLS_StyleScopedClasses['header-avatar']} */ ;
    (__VLS_ctx.initial);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "header-info" },
    });
    /** @type {__VLS_StyleScopedClasses['header-info']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "header-name" },
    });
    /** @type {__VLS_StyleScopedClasses['header-name']} */ ;
    (__VLS_ctx.info?.name);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "header-subtitle" },
    });
    /** @type {__VLS_StyleScopedClasses['header-subtitle']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "messages-area" },
        ref: "messagesRef",
    });
    /** @type {__VLS_StyleScopedClasses['messages-area']} */ ;
    if (!__VLS_ctx.messages.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "welcome-msg" },
        });
        /** @type {__VLS_StyleScopedClasses['welcome-msg']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "welcome-avatar" },
            ...{ style: ({ background: __VLS_ctx.info?.avatarColor || '#409EFF' }) },
        });
        /** @type {__VLS_StyleScopedClasses['welcome-avatar']} */ ;
        (__VLS_ctx.initial);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "welcome-bubble" },
        });
        /** @type {__VLS_StyleScopedClasses['welcome-bubble']} */ ;
        (__VLS_ctx.info?.welcomeMsg || `你好！我是 ${__VLS_ctx.info?.name}，有什么可以帮你的？`);
    }
    for (const [msg, i] of __VLS_vFor((__VLS_ctx.messages))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (i),
            ...{ class: (['msg-row', msg.role]) },
        });
        /** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
        if (msg.role === 'assistant') {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "msg-avatar" },
                ...{ style: ({ background: __VLS_ctx.info?.avatarColor || '#409EFF' }) },
            });
            /** @type {__VLS_StyleScopedClasses['msg-avatar']} */ ;
            (__VLS_ctx.initial);
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-bubble" },
        });
        __VLS_asFunctionalDirective(__VLS_directives.vHtml, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.renderText(msg.content)) }, null, null);
        /** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
        // @ts-ignore
        [needPassword, authed, info, info, info, info, info, info, info, info, initial, initial, initial, initial, submitPassword, submitPassword, passwordInput, passwordInput, passwordError, channelDeleted, infoLoaded, messages, messages, renderText,];
    }
    if (__VLS_ctx.streaming) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-row assistant" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-row']} */ ;
        /** @type {__VLS_StyleScopedClasses['assistant']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-avatar" },
            ...{ style: ({ background: __VLS_ctx.info?.avatarColor || '#409EFF' }) },
        });
        /** @type {__VLS_StyleScopedClasses['msg-avatar']} */ ;
        (__VLS_ctx.initial);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "msg-bubble streaming" },
        });
        /** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
        /** @type {__VLS_StyleScopedClasses['streaming']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        __VLS_asFunctionalDirective(__VLS_directives.vHtml, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.renderText(__VLS_ctx.streamingText)) }, null, null);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "cursor" },
        });
        /** @type {__VLS_StyleScopedClasses['cursor']} */ ;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "input-area" },
    });
    /** @type {__VLS_StyleScopedClasses['input-area']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.textarea)({
        ...{ onKeydown: (__VLS_ctx.sendMessage) },
        ...{ onInput: (__VLS_ctx.autoResize) },
        value: (__VLS_ctx.inputText),
        ...{ class: "input-box" },
        placeholder: "输入消息…",
        rows: "1",
        ref: "inputRef",
    });
    /** @type {__VLS_StyleScopedClasses['input-box']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.sendMessage) },
        ...{ class: "send-btn" },
        disabled: (!__VLS_ctx.inputText.trim() || __VLS_ctx.streaming),
    });
    /** @type {__VLS_StyleScopedClasses['send-btn']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "20",
        height: "20",
        viewBox: "0 0 24 24",
        fill: "currentColor",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M2.01 21L23 12 2.01 3 2 10l15 2-15 2z",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-footer" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-footer']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
        href: "https://github.com/sunhuihui6688-star/ai-panel",
        target: "_blank",
        ...{ class: "chat-footer-link" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-footer-link']} */ ;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "loading-page" },
    });
    /** @type {__VLS_StyleScopedClasses['loading-page']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "loading-spinner" },
    });
    /** @type {__VLS_StyleScopedClasses['loading-spinner']} */ ;
}
// @ts-ignore
[info, initial, renderText, streaming, streaming, streamingText, sendMessage, sendMessage, autoResize, inputText, inputText,];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
