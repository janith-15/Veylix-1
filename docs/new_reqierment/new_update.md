# VEYLIX — MASTER CODEBASE IMPROVEMENT & 3X-UI INTEGRATION INSTRUCTIONS

## MISSION

Improve the existing Veylix application incrementally, file by file, while preserving its established architecture, business rules, and existing working features.

The application must use the official 3X-UI API for supported VPN management operations.

Do not rebuild the application from scratch. Do not replace working code without evidence that replacement is necessary. Do not make speculative changes.

The objective is to create a reliable, maintainable, secure, thoroughly tested application—not merely to make the build pass.

## 1. REQUIRED INITIAL ACTION

Before editing anything:

1. Inspect the complete repository structure.
2. Read the project documentation, architecture decisions, existing review reports, and relevant implementation notes.
3. Inspect the current Git working tree and preserve all pre-existing user changes.
4. Identify the frontend entry points, backend entry points, database schema, migrations, service layer, API handlers, repositories, configuration files, tests, and deployment scripts.
5. Identify the existing Veylix-to-3X-UI integration.
6. Trace the actual execution flow of important features from frontend to backend, database, and remote panel.
7. Identify all unfinished, duplicated, unsafe, unverified, or inconsistent implementations.
8. Establish the existing test baseline before making changes.
9. Identify the target 3X-UI release and obtain the corresponding official API/OpenAPI contract.

Create an evidence-based inventory before modifying source files.

Never assume that a feature works because a function exists or a UI button is visible.

## 2. OFFICIAL 3X-UI REFERENCES

Official repository:
https://github.com/MHSanaei/3x-ui

Official API reference:
https://docs.sanaei.dev/docs/reference/api/

Inbound API:
https://docs.sanaei.dev/docs/reference/api/inbounds/

Documentation:
https://docs.sanaei.dev/

Use the actual deployed version's contract to verify endpoint paths, HTTP methods, request fields, authentication, response schemas, and supported operations.

Do not invent endpoints or rely on undocumented assumptions.

Keep panel credentials on the backend. Never commit tokens or expose them to the browser.

## 3. FILE-BY-FILE IMPROVEMENT POLICY

For every file that may need improvement:

1. Read the complete relevant implementation and its callers.
2. Understand the responsibilities and invariants of the file.
3. Identify the specific defect, missing capability, security risk, performance problem, or maintainability issue.
4. Find every dependent component, service, database operation, API contract, and test affected by the proposed change.
5. Design the smallest safe improvement that solves the actual problem.
6. Implement the change using established project conventions.
7. Update related tests and documentation.
8. Run the appropriate formatter, linter, static analysis, and tests.
9. Inspect the final diff for regressions and unintended changes.
10. Record the evidence and result in the improvement ledger.

Do not make changes merely to increase the number of modified files.

Do not split files, combine modules, rename public interfaces, or reorganize directories without a concrete justification.

Avoid unnecessary abstractions, duplicated logic, oversized service objects, circular dependencies, global mutable state, and hidden side effects.

Preserve public API contracts unless an explicitly justified, tested migration is necessary.

## 4. REQUIRED ARCHITECTURE RULES

Maintain the existing architecture wherever it is sound.

For the Go backend, preserve clear separation between:

- HTTP handlers and transport concerns.
- Authentication and authorization.
- Business services and domain rules.
- Persistence and database transactions.
- External 3X-UI API integration.
- Background jobs and synchronization.
- Configuration, secrets, logging, and observability.

For the frontend, preserve clear separation between:

- Page layouts and reusable components.
- API clients and data fetching.
- Forms and validation.
- Authentication and permission-aware navigation.
- Loading, empty, error, and success states.
- Business workflows and presentation logic.

Follow the actual frameworks and conventions already present in the repository. Do not introduce an alternative stack without a demonstrated need.

## 5. 3X-UI INTEGRATION REQUIREMENTS

Audit and improve all relevant, supported operations:

- Connection configuration and authenticated health checks.
- Inbound listing and inspection.
- Supported inbound create, update, enable, disable, and delete operations.
- Client create, retrieve, update, enable, disable, and delete operations.
- Protocol-specific client configuration.
- Supported expiry, traffic quota, and IP/device limits.
- Traffic statistics and online status.
- Supported subscription and connection-link delivery.
- Runtime and server metrics exposed by the actual API.
- Multi-panel synchronization if required by the existing architecture.

Verify every operation against the actual target API.

Implement safe timeouts, bounded retries, accurate error handling, duplicate-operation prevention, and fresh-state verification.

When a request times out, determine whether the remote operation was applied before retrying.

Do not silently ignore failed writes. Preserve fail-closed behavior for unverified panel operations.

Do not delete or overwrite valid client state because a remote response is temporarily unavailable.

## 6. BUSINESS DATA INTEGRITY

Veylix remains authoritative for:

- Customers and account ownership.
- Packages and pricing.
- Orders, payments, and invoices.
- Subscription lifecycle and renewal.
- Roles and authorization.
- Application audit history.

3X-UI remains the external VPN management integration.

Maintain explicit mappings between Veylix customers, subscriptions, panel identities, inbounds, and client identifiers.

Prevent duplicate client creation, cross-customer data leakage, concurrent-update corruption, and inconsistent billing records.

Use transactions for local database consistency and a suitable reconciliation or outbox workflow for remote operations when justified by the existing design.

Never claim that a transaction atomically covers both the local database and a remote 3X-UI panel.

Preserve historical usage records when remote traffic counters are reset.

## 7. SECURITY REVIEW FOR EVERY RELEVANT FILE

Check for:

- Broken authentication or authorization.
- Cross-customer access and insecure object references.
- Hardcoded secrets or leaked tokens.
- Unsafe panel URL handling and SSRF.
- Insecure TLS configuration.
- Missing request validation.
- SQL injection and unsafe database queries.
- Command injection and unsafe process execution.
- Excessive privileges.
- Sensitive information in logs or API responses.
- Unbounded resource consumption.
- Race conditions and unsafe concurrent operations.
- Missing audit records for privileged actions.
- Unsafe default configuration.
- Missing backup or recovery safeguards.

Fix verified vulnerabilities according to their severity.

Do not weaken authentication, remove checks, disable TLS verification, or loosen authorization merely to make a test pass.

## 8. PERFORMANCE AND RESOURCE MANAGEMENT

Evaluate the target deployment, which may have 1 CPU and 500 MB RAM.

- Avoid unbounded goroutines and uncontrolled background jobs.
- Bound API concurrency and polling frequency.
- Reuse HTTP clients and database connections appropriately.
- Prevent N+1 database queries.
- Use indexes where justified by query patterns.
- Avoid unnecessary repeated full-state synchronization.
- Avoid loading large frontend modules or datasets unnecessarily.
- Use pagination and table virtualization when appropriate.
- Preserve responsive UI behavior.
- Measure memory, CPU, API latency, database performance, and synchronization duration.

Do not claim that a specific number of active VPN users is supported without representative load-test evidence.

## 9. REQUIRED TESTING

Before declaring a change complete:

### Go backend

- Run existing unit and integration tests.
- Add regression tests for fixed defects.
- Run gofmt and the project's configured lint/static-analysis tools.
- Use race detection where appropriate and feasible.
- Verify database transaction and migration behavior.

### Frontend

- Run existing frontend tests.
- Run the configured type checker, linter, and production build.
- Test forms, API errors, loading states, empty states, and authorization behavior.
- Verify mobile responsiveness for affected screens.

### API integration

- Validate requests and responses against the target API schema.
- Test authentication failures, timeouts, server errors, malformed responses, and unavailable panels.
- Verify state-changing operations with a fresh read.
- Test duplicate requests, ambiguous outcomes, and reconciliation.

### Security and regression

- Verify customer isolation and role-based permissions.
- Confirm that API tokens never reach frontend bundles or logs.
- Run relevant dependency and security checks.
- Ensure existing tests have not been weakened to obtain passing results.

Use mocks for isolated unit tests and a controlled test panel for live integration tests. Do not treat mocked tests as proof of live compatibility.

## 10. IMPROVEMENT PRIORITY

Classify each verified finding:

P0 — Critical security vulnerability, data corruption, unauthorized access, or catastrophic failure.

P1 — Broken core functionality, unverified remote writes, incorrect billing/usage, or major security weakness.

P2 — Important reliability, performance, maintainability, or user-experience issue.

P3 — Non-critical optimization, visual refinement, or code cleanup.

Fix P0 and P1 findings first. Address P2 and P3 according to dependencies, risk, and available evidence.

Do not make large unrelated refactors while fixing critical issues.

## 11. IMPLEMENTATION PROCESS

Work in small, reviewable stages:

Stage A: Baseline and repository inventory.

Stage B: Architecture and dependency mapping.

Stage C: File-by-file defect and capability audit.

Stage D: Prioritized implementation plan.

Stage E: Incremental fixes and feature improvements.

Stage F: Unit, integration, contract, regression, and security tests.

Stage G: Live verification when a safe test panel and authorized credentials are available.

Stage H: Final diff review, documentation, and recovery instructions.

After each stage, record completed work, test results, remaining issues, and any blocked verification.

Continue to the next safe stage instead of stopping after producing an audit report.

Do not modify unrelated files, delete data, reset production counters, alter production clients, deploy to production, or commit changes without appropriate authorization.

If access to a required service is unavailable, finish all work that can be performed safely and identify exactly what remains unverified.

## 12. IMPROVEMENT LEDGER

Maintain a repository document named `docs/IMPROVEMENT_LEDGER.md`, unless the project already has an equivalent authoritative document.

For each finding, record:

- Finding ID and priority.
- Affected file and function.
- Evidence of the problem.
- Root cause.
- Planned and completed changes.
- Related API endpoint or database operation.
- Tests executed and actual results.
- Regression risks.
- Live-verification status.
- Remaining work.

Do not copy secrets, credentials, customer data, or sensitive production responses into this document.

Update the ledger as work progresses.

## 13. FINAL REPORT

At completion, report:

1. Files inspected.
2. Files changed and the reason for each change.
3. Existing functionality preserved.
4. Defects identified and fixed.
5. 3X-UI capabilities implemented.
6. API methods and schemas verified.
7. Database and configuration changes.
8. Commands and tests actually executed.
9. Actual pass/fail results.
10. Security findings and remediation.
11. Performance measurements, if available.
12. Remaining risks and blockers.
13. Live operations verified against the real panel.
14. Deployment and rollback instructions.

Classify each feature as:

- Implemented and tested.
- Implemented but not live-verified.
- Partially implemented.
- Unsupported by the target API.
- Blocked by missing access or environment.

Never fabricate test results or claim that a feature is complete simply because its code exists.

## FINAL DIRECTIVE

Begin by inspecting the real repository and preserving the existing working tree.

Audit first, prioritize findings, and then improve the application file by file. Maintain existing business invariants and architecture, verify all external API assumptions, and test every change.

The definition of done is evidence of correct behavior—not the number of files changed, the size of the diff, or a successful compilation alone.