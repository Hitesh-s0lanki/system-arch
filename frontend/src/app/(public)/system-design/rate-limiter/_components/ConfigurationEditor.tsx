'use client'

import { useState, type FormEvent } from 'react'
import {
  Check,
  ChevronDown,
  Download,
  Plus,
  Settings2,
  Trash2,
} from 'lucide-react'
import { Input } from '@/components/ui/input'
import {
  identityLabels,
  proxyConfiguration,
  validateConfiguration,
  type LimiterConfiguration,
  type PolicyRule,
} from './configuration'

const selectStyle =
  'h-9 w-full min-w-0 rounded-lg border border-input bg-background px-2 text-xs focus-visible:outline-2 focus-visible:outline-ring'

export function ConfigurationEditor({
  config,
  onApply,
  onModeChange,
}: {
  config: LimiterConfiguration
  onApply: (config: LimiterConfiguration) => void
  onModeChange: (mode: LimiterConfiguration['mode']) => void
}) {
  const [draft, setDraft] = useState<LimiterConfiguration>(() =>
    structuredClone(config),
  )
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const currentDraft = { ...draft, mode: config.mode }

  function update(change: Partial<LimiterConfiguration>) {
    setDraft((current) => ({ ...current, ...change }))
    setSaved(false)
    setError('')
  }

  function updateRule(index: number, change: Partial<PolicyRule>) {
    update({
      rules: draft.rules.map((rule, i) =>
        i === index ? { ...rule, ...change } : rule,
      ),
    })
  }

  function addRule() {
    let suffix = 2
    while (draft.rules.some((rule) => rule.id === `policy-${suffix}`)) suffix++
    update({
      rules: [
        ...draft.rules,
        {
          id: `policy-${suffix}`,
          pattern: `GET /api/route-${suffix}`,
          identity: 'api_key',
          limit: 10,
          windowSeconds: 60,
          maxKeys: 10000,
          perRoute: true,
        },
      ],
    })
  }

  function apply(event: FormEvent) {
    event.preventDefault()
    const issue = validateConfiguration(currentDraft)
    if (issue) {
      setError(issue)
      return
    }
    const normalized = {
      ...currentDraft,
      upstream: new URL(draft.upstream).toString(),
      rules: draft.rules.map((rule) => ({
        ...rule,
        pattern: rule.pattern.trim().replace(/\s+/g, ' '),
      })),
    }
    onApply(normalized)
    setDraft(normalized)
    setSaved(true)
    setError('')
  }

  function download() {
    const issue = validateConfiguration(currentDraft)
    if (issue) {
      setError(issue)
      return
    }
    const url = URL.createObjectURL(
      new Blob(
        [JSON.stringify(proxyConfiguration(currentDraft), null, 2) + '\n'],
        { type: 'application/json' },
      ),
    )
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'proxy.json'
    anchor.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  return (
    <section
      aria-labelledby="configuration-title"
      className="rounded-xl border border-border bg-card p-5"
    >
      <div className="mb-5 flex items-start gap-3">
        <Settings2
          className="mt-0.5 size-4 shrink-0 text-primary"
          aria-hidden="true"
        />
        <div>
          <h2 id="configuration-title" className="text-sm">
            Your website & policies
          </h2>
          <p className="mt-1 text-xs">
            Configure an upstream and one or more route rules. Each request uses
            one matching policy.
          </p>
        </div>
      </div>
      <form onSubmit={apply} noValidate className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)]">
          <label className="space-y-1.5 text-xs font-medium">
            Backend website URL
            <Input
              type="url"
              value={draft.upstream}
              onChange={(event) => update({ upstream: event.target.value })}
              placeholder="https://your-service.com"
              className="h-9 text-xs"
            />
          </label>
          <label className="space-y-1.5 text-xs font-medium">
            Storage
            <select
              aria-label="Storage"
              value={config.mode}
              onChange={(event) =>
                onModeChange(event.target.value as LimiterConfiguration['mode'])
              }
              className={selectStyle}
            >
              <option value="ram">RAM · per process</option>
              <option value="redis">Redis · shared</option>
            </select>
          </label>
          <label className="space-y-1.5 text-xs font-medium">
            Namespace
            <Input
              value={draft.namespace}
              onChange={(event) => update({ namespace: event.target.value })}
              className="h-9 text-xs"
            />
          </label>
        </div>
        <div className="space-y-3">
          {draft.rules.map((rule, index) => (
            <fieldset
              key={index}
              className="min-w-0 rounded-lg border border-border bg-muted/20 px-4 pb-4"
            >
              <legend className="px-1 text-[11px] font-semibold text-muted-foreground">
                Rule {index + 1}
              </legend>
              <div className="mb-3 flex items-center justify-between gap-3 pt-1">
                <span className="text-xs font-medium">
                  {rule.id || 'Unnamed policy'}
                </span>
                <button
                  type="button"
                  disabled={draft.rules.length === 1}
                  aria-label={`Remove rule ${index + 1}`}
                  onClick={() =>
                    update({ rules: draft.rules.filter((_, i) => i !== index) })
                  }
                  className="flex min-h-9 min-w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-destructive focus-visible:outline-2 focus-visible:outline-ring disabled:cursor-not-allowed disabled:opacity-30"
                >
                  <Trash2 className="size-3.5" aria-hidden="true" />
                </button>
              </div>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                <label className="space-y-1.5 text-xs">
                  Policy name · rule {index + 1}
                  <Input
                    value={rule.id}
                    onChange={(event) =>
                      updateRule(index, { id: event.target.value })
                    }
                    className="h-9 text-xs"
                  />
                </label>
                <label className="space-y-1.5 text-xs">
                  Route pattern · rule {index + 1}
                  <Input
                    value={rule.pattern}
                    onChange={(event) =>
                      updateRule(index, { pattern: event.target.value })
                    }
                    placeholder="POST /orders"
                    className="h-9 font-mono text-xs"
                  />
                </label>
                <label className="space-y-1.5 text-xs">
                  Limit by · rule {index + 1}
                  <select
                    aria-label={`Limit by · rule ${index + 1}`}
                    value={rule.identity}
                    onChange={(event) =>
                      updateRule(index, {
                        identity: event.target.value as PolicyRule['identity'],
                      })
                    }
                    className={selectStyle}
                  >
                    {Object.entries(identityLabels).map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="space-y-1.5 text-xs">
                  Requests per window · rule {index + 1}
                  <Input
                    type="number"
                    min={1}
                    max={2147483647}
                    step={1}
                    value={Number.isNaN(rule.limit) ? '' : rule.limit}
                    onChange={(event) =>
                      updateRule(index, {
                        limit:
                          event.target.value === ''
                            ? NaN
                            : Number(event.target.value),
                      })
                    }
                    className="h-9 text-xs"
                  />
                </label>
                <label className="space-y-1.5 text-xs">
                  Window in seconds · rule {index + 1}
                  <Input
                    type="number"
                    min={1}
                    max={86400}
                    step={1}
                    value={
                      Number.isNaN(rule.windowSeconds) ? '' : rule.windowSeconds
                    }
                    onChange={(event) =>
                      updateRule(index, {
                        windowSeconds:
                          event.target.value === ''
                            ? NaN
                            : Number(event.target.value),
                      })
                    }
                    className="h-9 text-xs"
                  />
                </label>
                <label className="space-y-1.5 text-xs">
                  Maximum identities · rule {index + 1}
                  <Input
                    type="number"
                    min={1}
                    max={2147483647}
                    step={1}
                    value={Number.isNaN(rule.maxKeys) ? '' : rule.maxKeys}
                    onChange={(event) =>
                      updateRule(index, {
                        maxKeys:
                          event.target.value === ''
                            ? NaN
                            : Number(event.target.value),
                      })
                    }
                    className="h-9 text-xs"
                  />
                </label>
              </div>
              <label className="mt-3 flex min-h-9 cursor-pointer items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={rule.perRoute}
                  onChange={(event) =>
                    updateRule(index, { perRoute: event.target.checked })
                  }
                  className="size-4 accent-primary"
                />{' '}
                Separate allowance by route · rule {index + 1}
              </label>
            </fieldset>
          ))}
        </div>
        {error && (
          <p
            role="alert"
            className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive"
          >
            {error}
          </p>
        )}
        <div className="flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={addRule}
            disabled={draft.rules.length >= 20}
            className="inline-flex min-h-10 items-center gap-2 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-40"
          >
            <Plus className="size-3.5" aria-hidden="true" /> Add policy rule
          </button>
          <button
            type="submit"
            className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-primary px-4 text-xs font-medium text-primary-foreground hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            Apply to diagram
          </button>
          <button
            type="button"
            onClick={download}
            className="inline-flex min-h-10 items-center gap-2 rounded-lg px-3 text-xs font-medium text-primary hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
          >
            <Download className="size-3.5" aria-hidden="true" /> Download
            proxy.json
          </button>
          {saved && (
            <span
              aria-live="polite"
              className="inline-flex items-center gap-1.5 text-xs text-emerald-700 dark:text-emerald-400"
            >
              <Check className="size-3.5" aria-hidden="true" /> Applied to
              illustration
            </span>
          )}
        </div>
        <details className="rounded-lg border border-border px-3 py-2 text-xs">
          <summary className="flex min-h-8 cursor-pointer items-center gap-2 font-medium">
            <ChevronDown className="size-3.5" aria-hidden="true" />{' '}
            Configuration preview & deployment
          </summary>
          <p className="my-3 text-xs">
            Apply updates this illustration only. To run it, download the JSON,
            replace configs/proxy.json, and restart the Docker gateway.
            Configure Redis through REDIS_URL. User and API-key policies require
            verified bearer credentials in RATE_LIMIT_CREDENTIALS; use
            credential IDs in the example below, not secret tokens.
          </p>
          <p className="mb-3 text-xs">
            Rules select policies for different routes. Simultaneous IP +
            API-key enforcement on the same request is not implemented in the
            gateway. Docker startup also checks for conflicting route patterns.
          </p>
          <pre
            className="max-h-72 overflow-auto rounded-lg bg-muted p-3 font-mono text-[11px] leading-relaxed"
            aria-label="Generated proxy configuration"
          >
            {JSON.stringify(proxyConfiguration(currentDraft), null, 2)}
          </pre>
        </details>
      </form>
    </section>
  )
}
