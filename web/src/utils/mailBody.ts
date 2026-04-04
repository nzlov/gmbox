// extractMailHtml 只保留邮件 body 里的可渲染内容，避免 head/style 等模板残留污染正文。
export function extractMailHtml(html: string): string {
  const trimmedHtml = html.trim()
  if (!trimmedHtml) {
    return ''
  }

  const parser = new DOMParser()
  const doc = parser.parseFromString(trimmedHtml, 'text/html')
  doc.querySelectorAll('base, head, link, meta, script, style, title').forEach((node) => node.remove())
  return doc.body.innerHTML.trim()
}

// extractMailText 优先从解析后的正文提取纯文本，避免把样式或脚本内容误当成邮件正文。
export function extractMailText(html: string): string {
  const bodyHtml = extractMailHtml(html)
  if (!bodyHtml) {
    return ''
  }

  const parser = new DOMParser()
  const doc = parser.parseFromString(bodyHtml, 'text/html')
  return doc.body.textContent?.replace(/\s+/g, ' ').trim() ?? ''
}
