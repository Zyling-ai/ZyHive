/**
 * ZyHive Install Worker
 * 部署在 install.zyling.ai
 *
 * 路由：
 *   GET /zyhive.sh          → 最新安装脚本（从 GitHub raw 代理，实时最新）
 *   GET /latest             → 最新版本号 JSON {"version":"v0.9.7"}
 *   GET /dl/{ver}/{file}    → Release 二进制代理（解决国内无法访问 GitHub）
 *   GET /                   → 重定向到 zyling.ai
 */

const REPO = 'Zyling-ai/zyhive'
const RAW_SCRIPT = `https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh`
const GH_API_LATEST = `https://api.github.com/repos/${REPO}/releases/latest`
const GH_DOWNLOAD = `https://github.com/${REPO}/releases/download`

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url)
    const path = url.pathname

    // ── 根路由 → 跳转官网 ────────────────────────────────────────────────
    if (path === '/') {
      return Response.redirect('https://zyling.ai', 302)
    }

    // ── 安装脚本 ─────────────────────────────────────────────────────────
    if (path === '/zyhive.sh') {
      const res = await fetch(RAW_SCRIPT, {
        headers: { 'User-Agent': 'ZyHive-Install-Worker/1.0' },
        cf: { cacheEverything: false },
      })
      if (!res.ok) {
        return new Response(`无法获取安装脚本: ${res.status}`, { status: 502 })
      }
      return new Response(res.body, {
        headers: {
          'Content-Type': 'text/plain; charset=utf-8',
          'Cache-Control': 'no-cache',
          'X-Served-By': 'install.zyling.ai',
        },
      })
    }

    // ── 最新版本号 ───────────────────────────────────────────────────────
    if (path === '/latest') {
      const cacheKey = new Request('https://gh-latest-cache/zyhive', request)
      const cache = caches.default
      let cached = await cache.match(cacheKey)
      if (cached) return cached

      const res = await fetch(GH_API_LATEST, {
        headers: {
          'Accept': 'application/vnd.github.v3+json',
          'User-Agent': 'ZyHive-Install-Worker/1.0',
        },
      })
      if (!res.ok) {
        return new Response(JSON.stringify({ error: 'GitHub API error', status: res.status }), {
          status: 502,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      const data = await res.json()
      const body = JSON.stringify({ version: data.tag_name, published_at: data.published_at })
      const response = new Response(body, {
        headers: {
          'Content-Type': 'application/json',
          'Cache-Control': 'public, max-age=300',
          'Access-Control-Allow-Origin': '*',
        },
      })
      ctx.waitUntil(cache.put(cacheKey, response.clone()))
      return response
    }

    // ── 二进制下载代理 /dl/{version}/{filename} ──────────────────────────
    if (path.startsWith('/dl/')) {
      const sub = path.slice(4) // "v0.9.7/aipanel-linux-amd64"
      const slash = sub.indexOf('/')
      if (slash === -1) {
        return new Response('Bad Request: 路径格式应为 /dl/{version}/{filename}', { status: 400 })
      }
      const version = sub.slice(0, slash)
      const filename = sub.slice(slash + 1)

      if (!filename || filename.includes('..') || filename.includes('/')) {
        return new Response('Bad Request: 非法文件名', { status: 400 })
      }

      const ghUrl = `${GH_DOWNLOAD}/${version}/${filename}`

      // 先走 CF 缓存
      const cacheKey = new Request(ghUrl, { method: 'GET' })
      const cache = caches.default
      let cached = await cache.match(cacheKey)
      if (cached) {
        return new Response(cached.body, {
          headers: {
            ...Object.fromEntries(cached.headers),
            'X-Cache': 'HIT',
          },
        })
      }

      const res = await fetch(ghUrl, {
        redirect: 'follow',
        headers: { 'User-Agent': 'ZyHive-Install-Worker/1.0' },
      })
      if (!res.ok) {
        return new Response(`下载失败 (${res.status})：${ghUrl}`, { status: res.status })
      }

      const response = new Response(res.body, {
        status: 200,
        headers: {
          'Content-Type': 'application/octet-stream',
          'Content-Disposition': `attachment; filename="${filename}"`,
          'Cache-Control': 'public, max-age=86400',
          'X-Cache': 'MISS',
          'X-Source-URL': ghUrl,
          ...(res.headers.get('Content-Length')
            ? { 'Content-Length': res.headers.get('Content-Length') }
            : {}),
        },
      })
      ctx.waitUntil(cache.put(cacheKey, response.clone()))
      return response
    }

    return new Response('Not Found', { status: 404 })
  },
}
