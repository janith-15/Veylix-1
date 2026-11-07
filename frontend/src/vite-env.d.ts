/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Optional absolute API base; empty means same-origin (Vite proxy / Go binary). */
  readonly VITE_API_BASE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
