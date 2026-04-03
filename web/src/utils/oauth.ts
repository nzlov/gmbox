const storagePrefix = 'microsoft_oauth_pkce:'
const stateSize = 24
const verifierSize = 64

export type MicrosoftOAuthSession = {
  state: string
  codeVerifier: string
  createdAt: number
}

// randomString 统一生成 URL 安全随机串，避免 PKCE 参数出现额外转义问题。
function randomString(size: number) {
  const bytes = new Uint8Array(size)
  crypto.getRandomValues(bytes)
  return toBase64URL(bytes)
}

// toBase64URL 把二进制稳定编码为 base64url，满足 PKCE 对 challenge 的格式要求。
function toBase64URL(bytes: Uint8Array) {
  let binary = ''
  bytes.forEach((value) => {
    binary += String.fromCharCode(value)
  })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

// createMicrosoftOAuthSession 在发起授权前生成一次性的 state 与 verifier。
export function createMicrosoftOAuthSession(): MicrosoftOAuthSession {
  return {
    state: randomString(stateSize),
    codeVerifier: randomString(verifierSize),
    createdAt: Date.now(),
  }
}

// createCodeChallenge 使用浏览器原生摘要算法生成 S256 challenge，避免手写实现出错。
export async function createCodeChallenge(codeVerifier: string) {
  const data = new TextEncoder().encode(codeVerifier)
  const digest = await crypto.subtle.digest('SHA-256', data)
  return toBase64URL(new Uint8Array(digest))
}

// saveMicrosoftOAuthSession 以 state 为键保存 PKCE 会话，便于回调页按 state 找回 verifier。
export function saveMicrosoftOAuthSession(session: MicrosoftOAuthSession) {
  localStorage.setItem(`${storagePrefix}${session.state}`, JSON.stringify(session))
}

// loadMicrosoftOAuthSession 按 state 读取授权上下文，避免多个授权窗口互相覆盖。
export function loadMicrosoftOAuthSession(state: string) {
  const raw = localStorage.getItem(`${storagePrefix}${state}`)
  if (!raw) {
    return null
  }
  try {
    const parsed = JSON.parse(raw) as MicrosoftOAuthSession
    if (!parsed.state || !parsed.codeVerifier || !parsed.createdAt) {
      return null
    }
    return parsed
  } catch {
    return null
  }
}

// clearMicrosoftOAuthSession 在授权结束后立即清理本地凭据，减少残留风险。
export function clearMicrosoftOAuthSession(state: string) {
  localStorage.removeItem(`${storagePrefix}${state}`)
}
