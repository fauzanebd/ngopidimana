/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * Absolute origin of the API, e.g. "https://api.example.com".
   * Leave unset (or empty) in development so requests stay relative and go
   * through the Vite dev proxy configured in vite.config.ts.
   * Must not end with a trailing slash.
   */
  readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
