const SERVER_URL_KEY = 'aipanel_url'

export function getServerBaseURL(): string {
  return (localStorage.getItem(SERVER_URL_KEY) || '').trim().replace(/\/+$/, '')
}

export function getAPIBaseURL(): string {
  const server = getServerBaseURL()
  return server ? `${server}/api` : '/api'
}

export function apiURL(path: string): string {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  return `${getAPIBaseURL()}${normalizedPath}`
}
