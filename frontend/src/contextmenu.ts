// The Mac window's web view has its own right-click menu (Back, Reload),
// which is no use in an app and can reload away what was typed. It stays
// where it helps: in text fields (paste) and on selected text (copy, look
// up). A browser keeps its menu; this is only installed in the window.
export function hideWebMenu() {
  const on = (e: MouseEvent) => {
    if (e.defaultPrevented) return
    const el = e.target instanceof Element ? e.target : null
    if (el?.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])')) return
    const sel = window.getSelection()
    if (el && sel && !sel.isCollapsed && sel.toString().trim() && sel.containsNode(el, true)) return
    e.preventDefault()
  }
  document.addEventListener('contextmenu', on)
  return () => document.removeEventListener('contextmenu', on)
}
