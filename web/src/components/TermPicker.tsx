import { useRef, useState } from 'react'

/** Tap-first term entry: common choices are chips you toggle, anything else is
 *  typed and becomes a removable chip too. Commas and Enter commit, and the
 *  whole thing degrades to plain typing — the old comma-separated input still
 *  works, it just grows chips as you go. */
export function TermPicker(props: {
  value: string[]
  onChange: (next: string[]) => void
  suggestions: string[]
  placeholder: string
  tone?: 'danger'
}) {
  const [draft, setDraft] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const { value, suggestions } = props

  const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase()
  const has = (term: string) => value.some((v) => same(v, term))
  const toggle = (term: string) =>
    props.onChange(has(term) ? value.filter((v) => !same(v, term)) : [...value, term])

  const commit = (raw: string) => {
    const terms = raw.split(',')
      .map((t) => t.trim().toLowerCase())
      .filter((t) => t && !has(t))
    if (terms.length) props.onChange([...value, ...terms])
    setDraft('')
  }

  // Typed terms the chip row doesn't already offer.
  const custom = value.filter((v) => !suggestions.some((s) => same(s, v)))

  return (
    <div
      className={`picker ${props.tone ?? ''}`}
      onClick={(event) => {
        if (event.target === event.currentTarget) inputRef.current?.focus()
      }}
    >
      {suggestions.map((term) => (
        <button type="button" key={term}
          className={`pick ${has(term) ? 'on' : ''}`}
          onClick={() => toggle(term)}
        >
          {term}
        </button>
      ))}
      {custom.map((term) => (
        <button type="button" key={term} className="pick on custom"
          title="Remove" onClick={() => toggle(term)}
        >
          {term}<span className="x" aria-hidden>✕</span>
        </button>
      ))}
      <input
        ref={inputRef}
        className="pick-input"
        type="text"
        value={draft}
        placeholder={value.length ? 'add more…' : props.placeholder}
        autoCapitalize="none" autoCorrect="off" spellCheck={false}
        enterKeyHint="done"
        onChange={(event) => {
          const raw = event.target.value
          if (raw.includes(',')) commit(raw)
          else setDraft(raw)
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            commit(draft)
          } else if (event.key === 'Backspace' && !draft && value.length) {
            props.onChange(value.slice(0, -1))
          }
        }}
        onBlur={() => commit(draft)}
      />
    </div>
  )
}
