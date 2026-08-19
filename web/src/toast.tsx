// A single transient toast with an optional action — the undo rail and the
// update prompt share it. Module-level so plain code (row actions, the service
// worker hook) can raise one without threading React context around.
import { useEffect, useState } from 'react'

interface Toast {
  message: string
  action?: { label: string; run: () => void }
  /** 0 keeps it up until acted on or replaced (the update prompt). */
  ttl: number
}

let listener: ((toast: Toast | null) => void) | null = null

export function showToast(message: string, action?: Toast['action'], ttl = 5000) {
  listener?.({ message, action, ttl })
}

export function Toasts() {
  const [toast, setToast] = useState<Toast | null>(null)

  useEffect(() => {
    listener = setToast
    return () => { listener = null }
  }, [])

  useEffect(() => {
    if (!toast || toast.ttl === 0) return
    const timer = setTimeout(() => setToast(null), toast.ttl)
    return () => clearTimeout(timer)
  }, [toast])

  if (!toast) return null
  return (
    <div className="toast" role="status">
      <span>{toast.message}</span>
      {toast.action && (
        <button onClick={() => { toast.action!.run(); setToast(null) }}>
          {toast.action.label}
        </button>
      )}
    </div>
  )
}
