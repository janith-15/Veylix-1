import { useEffect, useState } from 'react'
import { GitBranch } from 'lucide-react'
import { api } from '../../lib/api'

/**
 * The build stamp at the foot of the sidebar: version + source repository.
 *
 * WHY THE VERSION COMES FROM THE SERVER
 * A version baked into the bundle would describe the last `npm run build`, not
 * the binary actually answering requests — and those two are the pair a support
 * conversation needs to agree on. `GET /api/system/version` reports the RUNNING
 * build (D-081), so this line cannot drift from the thing it describes.
 *
 * It is fetched ONCE per mount and silently absent while in flight or after a
 * failure. A build stamp is not worth an error state, an empty dash, or a
 * retry button; "no version shown" is the honest quiet answer, and the sidebar
 * must not flicker for it.
 *
 * WHY THE REPO URL IS A BUILD CONSTANT
 * The source location is fixed for a given build and is not a server secret, so
 * it is inlined at build time (`VITE_REPO_URL`) rather than served — no request,
 * no permission, and it cannot be edited by a caller. When it is unset the link
 * is simply not rendered instead of pointing at a dead placeholder.
 */
/*
 * The public source repository, shown as a link at the foot of the sidebar
 * (D-101).
 *
 * Injected by `vite.config.ts` at build time, not read from `.env`: that file is
 * git-ignored here, so a value living only there would vanish in every
 * deployment built from the repository. Declared here so TypeScript knows the
 * name exists — the value is a compile-time literal produced by the build.
 *
 * Empty is a valid outcome and hides the link; it must never render a
 * placeholder that points nowhere.
 */
declare const __VEYLIX_REPO_URL__: string

/** The repository URL, or undefined when the build deliberately leaves it out. */
const REPO_URL: string | undefined = __VEYLIX_REPO_URL__.trim() || undefined

/** `https://github.com/owner/name` → `owner/name`, for a label that fits. */
function repoLabel(url: string): string {
  try {
    const u = new URL(url)
    return u.pathname.replace(/^\/+|\/+$/g, '') || u.hostname
  } catch {
    return url
  }
}

export function BuildFooter({ rail = false }: { rail?: boolean }) {
  const [version, setVersion] = useState<string | null>(null)

  useEffect(() => {
    let live = true
    void api
      .systemVersion()
      .then((v) => {
        if (live) setVersion(v.version || null)
      })
      // An unreadable version is not a page failure: the footer just stays
      // empty. Swallowing it here is deliberate, not an oversight.
      .catch(() => undefined)
    return () => {
      live = false
    }
  }, [])

  if (rail) {
    // In the collapsed rail there is no room for text, so the two facts become
    // two tooltips. Rendering them stacked would push the nav into the rail's
    // 20px band and break the icon grid.
    return (
      <div className="app-sidebar__build app-sidebar__build--rail">
        {version && (
          <span className="app-sidebar__build-version" title={`Version ${version}`}>
            {version}
          </span>
        )}
        {REPO_URL && (
          <a
            className="app-sidebar__build-link"
            href={REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            title={repoLabel(REPO_URL)}
            aria-label={`Source code: ${repoLabel(REPO_URL)}`}
          >
            <GitBranch size={14} aria-hidden="true" />
          </a>
        )}
      </div>
    )
  }

  return (
    <div className="app-sidebar__build">
      {version && (
        <span className="app-sidebar__build-version" data-testid="app-version">
          v{version.replace(/^v/, '')}
        </span>
      )}
      {REPO_URL && (
        <a
          className="app-sidebar__build-link"
          href={REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          data-testid="app-repo-link"
          title={`Source code — ${repoLabel(REPO_URL)}`}
        >
          <GitBranch size={12} aria-hidden="true" />
          <span className="app-sidebar__build-repo">{repoLabel(REPO_URL)}</span>
        </a>
      )}
    </div>
  )
}
