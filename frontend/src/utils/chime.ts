// Play a gentle two-tone chime using Web Audio API without any external audio assets.
export function playChime() {
  try {
    const AC = window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
    if (!AC) return
    const ctx = new AC()
    if (ctx.state === 'suspended') {
      void ctx.resume().catch(() => {})
    }
    const now = ctx.currentTime
    const notes = [523.25, 659.25] // C5 -> E5
    notes.forEach((freq, i) => {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      const start = now + i * 0.18
      osc.type = 'sine'
      osc.frequency.setValueAtTime(freq, start)
      gain.gain.setValueAtTime(0.0001, start)
      gain.gain.exponentialRampToValueAtTime(0.12, start + 0.04)
      gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.55)
      osc.connect(gain)
      gain.connect(ctx.destination)
      osc.start(start)
      osc.stop(start + 0.6)
    })
    window.setTimeout(() => {
      void ctx.close().catch(() => {})
    }, 1200)
  } catch {
    // Ignore audio restrictions in headless/muted environments.
  }
}
