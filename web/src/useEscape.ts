import { useEffect } from 'react'

/** Escape closes the thing on top. Tapping the backdrop already did; a keyboard
 *  had no way out of a sheet at all. */
export function useEscape(onEscape: () => void) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.stopPropagation()
      onEscape()
    }
    // Capture, so the sheet on top wins over anything listening on the window.
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onEscape])
}
