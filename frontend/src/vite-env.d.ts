/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Optional absolute API base; empty means same-origin (Vite proxy / Go binary). */
  readonly VITE_API_BASE?: string
  /**
   * The public source repository, shown as a link at the foot of the sidebar.
   *
   * Build-time rather than served: where the code lives is a property of the
   * BUILD, it is not a secret, and it must not need a request or a permission to
   * display. Unset means the link is not rendered at all (never a placeholder).
   */
  readonly VITE_REPO_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
