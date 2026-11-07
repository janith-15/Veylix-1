import { useEffect, useMemo, useState } from 'react'
import { Button, Card, Input, PageHeader, Skeleton, useToast } from '../components/ui'
import { Calculator, Coins, Save } from 'lucide-react'
import { usePricingStore, type PricingInput, type PricingRates } from '../stores/pricingStore'
import { useServersStore } from '../stores/serversStore'
import { ApiError } from '../lib/api'
import './pricing.css'

/**
 * L4 — the rate card (D-035.4).
 *
 * This screen is the reason a CUSTOM order has a price at all. It edits two
 * scopes: the platform default (what every server inherits) and a per-server
 * override. A customer never edits these numbers — they only ever receive the
 * server's own quote.
 *
 * Money is entered in the currency's major unit and converted to integer cents
 * on submit; a fraction of a cent is refused rather than rounded, because a
 * rounded rate silently changes every future quote.
 */

/** Major unit ("12.50") → integer cents (1250). `null` = not a valid amount. */
function toCents(text: string): number | null {
  const trimmed = text.trim()
  if (trimmed === '') return null
  if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return null
  const [whole, frac = ''] = trimmed.split('.')
  const cents = Number(whole) * 100 + Number((frac + '00').slice(0, 2))
  return Number.isFinite(cents) ? cents : null
}

/** Integer cents → "12.50" for an input box. */
function toMajor(cents: number): string {
  return (cents / 100).toFixed(2)
}

interface FormState {
  currency: string
  perGb: string
  perDay: string
  perDevice: string
  perIp: string
  minPrice: string
  maxPrice: string
  maxTrafficGb: string
  maxDurationDays: string
  maxDevices: string
  maxIps: string
}

function formFrom(row: PricingRates | null): FormState {
  return {
    currency: row?.currency ?? 'USD',
    perGb: row ? toMajor(row.perGbCents) : '',
    perDay: row ? toMajor(row.perDayCents) : '',
    perDevice: row ? toMajor(row.perDeviceCents) : '',
    perIp: row ? toMajor(row.perIpCents) : '',
    minPrice: row ? toMajor(row.minPriceCents) : '',
    maxPrice: row ? toMajor(row.maxPriceCents) : '',
    maxTrafficGb: row ? String(row.maxTrafficGb) : '',
    maxDurationDays: row ? String(row.maxDurationDays) : '',
    maxDevices: row ? String(row.maxDevices) : '',
    maxIps: row ? String(row.maxIps) : '',
  }
}

/** Every money field, in the order the page shows them. */
const MONEY_FIELDS = [
  { key: 'perGb', label: 'Per GB of traffic', hint: 'Charged once per GB in the spec' },
  { key: 'perDay', label: 'Per day', hint: 'Charged once per day of duration' },
  { key: 'perDevice', label: 'Per device', hint: 'Charged once per allowed device' },
  { key: 'perIp', label: 'Per IP', hint: 'Charged once per allowed IP' },
  { key: 'minPrice', label: 'Minimum order', hint: 'A floor under the total' },
  { key: 'maxPrice', label: 'Price ceiling', hint: '0 means no ceiling' },
] as const

/** Integer fields that bound a spec. */
const BOUND_FIELDS = [
  { key: 'maxTrafficGb', label: 'Max traffic (GB)', hint: '0 means no bound' },
  { key: 'maxDurationDays', label: 'Max duration (days)', hint: '0 means no bound' },
  { key: 'maxDevices', label: 'Max devices', hint: '0 means no bound' },
  { key: 'maxIps', label: 'Max IPs', hint: '0 means no bound' },
] as const

/** Form field → wire field, so the two tables stay side by side. */
const MONEY_TO_WIRE: Record<string, keyof PricingInput> = {
  perGb: 'perGbCents',
  perDay: 'perDayCents',
  perDevice: 'perDeviceCents',
  perIp: 'perIpCents',
  minPrice: 'minPriceCents',
  maxPrice: 'maxPriceCents',
}

const BOUND_TO_WIRE: Record<string, keyof PricingInput> = {
  maxTrafficGb: 'maxTrafficGb',
  maxDurationDays: 'maxDurationDays',
  maxDevices: 'maxDevices',
  maxIps: 'maxIps',
}

export function PricingPage() {
  const { rates, loading, saving, error, load, save } = usePricingStore()
  const { servers, load: loadServers } = useServersStore()
  const toast = useToast()

  /** Which scope is being edited: `null` = platform default. */
  const [scope, setScope] = useState<number | null>(null)
  const [formError, setFormError] = useState('')
  /**
   * Edits are stored per scope, not as one blob.
   *
   * A single `form` had to be re-seeded by an effect whenever the scope
   * changed, which is both a render-time side effect and the exact way an
   * override silently inherits the default's numbers. Keeping one draft per
   * scope means the value shown is always the value for the scope on screen.
   */
  const [drafts, setDrafts] = useState<Record<string, FormState>>({})
  const [touched, setTouched] = useState<Record<string, boolean>>({})

  useEffect(() => {
    void load()
    void loadServers()
  }, [load, loadServers])

  const current = useMemo(() => {
    if (scope === null) return rates.find((r) => r.serverId === null) ?? null
    return rates.find((r) => r.serverId === scope) ?? null
  }, [rates, scope])

  /** The row this server would inherit when it has no override of its own. */
  const inherited = useMemo(() => {
    if (scope === null) return null
    return rates.find((r) => r.serverId === null) ?? null
  }, [rates, scope])

  const scopeKey = scope === null ? 'default' : String(scope)
  // Derived during render: the saved row (or what this scope inherits) unless
  // the operator has typed something for THIS scope.
  const form = drafts[scopeKey] ?? formFrom(current ?? inherited)
  const dirty = touched[scopeKey] ?? false

  const set = (key: keyof FormState, value: string) => {
    setDrafts((d) => ({ ...d, [scopeKey]: { ...form, [key]: value } }))
    setTouched((t) => ({ ...t, [scopeKey]: true }))
    setFormError('')
  }

  const submit = async () => {
    const input: PricingInput = { serverId: scope }

    for (const f of MONEY_FIELDS) {
      const raw = form[f.key]
      if (raw.trim() === '') continue
      const cents = toCents(raw)
      if (cents === null || cents < 0) {
        setFormError(`${f.label}: enter an amount with at most two decimals.`)
        return
      }
      ;(input as Record<string, unknown>)[MONEY_TO_WIRE[f.key]] = cents
    }

    for (const f of BOUND_FIELDS) {
      const raw = form[f.key]
      if (raw.trim() === '') continue
      const n = Number(raw)
      if (!Number.isInteger(n) || n < 0) {
        setFormError(`${f.label}: enter a whole number of 0 or more.`)
        return
      }
      ;(input as Record<string, unknown>)[BOUND_TO_WIRE[f.key]] = n
    }

    if (form.currency.trim()) input.currency = form.currency.trim().toUpperCase()

    // A ceiling below the floor would refuse every quote; catch it here so the
    // operator is not left with a rate card that prices nothing.
    const minCents = toCents(form.minPrice) ?? current?.minPriceCents ?? 0
    const maxCents = toCents(form.maxPrice) ?? current?.maxPriceCents ?? 0
    if (maxCents > 0 && minCents > maxCents) {
      setFormError('The price ceiling cannot be lower than the minimum order.')
      return
    }

    try {
      await save(input)
      // Drop this scope's draft so the form shows what the server now stored,
      // rather than a local copy that could drift from it.
      setDrafts((d) => {
        const next = { ...d }
        delete next[scopeKey]
        return next
      })
      setTouched((t) => ({ ...t, [scopeKey]: false }))
      toast(
        'success',
        scope === null ? 'Platform rate card saved' : 'Server override saved',
        scope === null
          ? 'Every server without its own override now inherits these rates.'
          : 'This server alone uses these rates from now on.',
      )
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Could not save the rates.')
    }
  }

  return (
    <div className="pricing-page">
      <PageHeader
        title="Rate card"
        subtitle="What a CUSTOM Premium order is priced from: one platform default, plus an optional override per server."
        actions={
          <Button leading={<Save size={14} />} onClick={() => void submit()} disabled={saving || !dirty}>
            {saving ? 'Saving…' : 'Save rates'}
          </Button>
        }
      />

      {error && (
        <Card className="pricing-notice">
          <p className="t-sm2">{error}</p>
        </Card>
      )}

      <div className="pricing-grid">
        <Card className="pricing-scope">
          <label className="ui-field">
            <span className="ui-field__label">Editing scope</span>
            <select
              className="ui-input"
              value={scope === null ? 'default' : String(scope)}
              onChange={(e) => setScope(e.target.value === 'default' ? null : Number(e.target.value))}
            >
              <option value="default">Platform default (all servers)</option>
              {servers.map((s) => (
                <option key={s.id} value={s.apiId}>
                  {s.name}
                  {rates.some((r) => r.serverId === s.apiId) ? ' · has an override' : ''}
                </option>
              ))}
            </select>
          </label>

          <p className="ui-muted t-sm2 pricing-scope__note">
            {scope === null
              ? 'These rates apply to every server that has no override of its own.'
              : inherited
                ? 'Fields left blank keep this server’s current value. A saved override replaces the platform default for this server only.'
                : 'There is no platform default yet — publish that first, or this server has nothing to inherit.'}
          </p>

          {current ? (
            <p className="ui-muted t-sm2 pricing-scope__meta">
              Last saved {new Date(current.updatedAt * 1000).toLocaleString()}
            </p>
          ) : scope !== null ? (
            <p className="ui-muted t-sm2 pricing-scope__meta">No override for this server yet.</p>
          ) : null}
        </Card>

        <Card className="pricing-form">
          <div className="pricing-form__head">
            <Coins size={16} aria-hidden />
            <h2>Unit prices</h2>
          </div>

          {loading ? (
            <Skeleton height={220} />
          ) : (
            <>
              <div className="pricing-fields">
                <Input
                  label="Currency"
                  hint="Three-letter code, e.g. USD or LKR"
                  value={form.currency}
                  onChange={(e) => set('currency', e.target.value)}
                  maxLength={3}
                />
                {MONEY_FIELDS.map((f) => (
                  <Input
                    key={f.key}
                    label={f.label}
                    hint={f.hint}
                    inputMode="decimal"
                    placeholder="0.00"
                    value={form[f.key]}
                    onChange={(e) => set(f.key, e.target.value)}
                  />
                ))}
              </div>

              <div className="pricing-form__head pricing-form__head--spaced">
                <Calculator size={16} aria-hidden />
                <h2>Specification bounds</h2>
              </div>
              <p className="ui-muted t-sm2">
                A bound refuses a spec that exceeds it rather than quietly clamping it — a customer is
                never sold something other than what they asked for.
              </p>
              <div className="pricing-fields">
                {BOUND_FIELDS.map((f) => (
                  <Input
                    key={f.key}
                    label={f.label}
                    hint={f.hint}
                    inputMode="numeric"
                    placeholder="0"
                    value={form[f.key]}
                    onChange={(e) => set(f.key, e.target.value)}
                  />
                ))}
              </div>

              {formError && (
                <p className="pricing-error" role="alert">
                  {formError}
                </p>
              )}
            </>
          )}
        </Card>
      </div>
    </div>
  )
}

