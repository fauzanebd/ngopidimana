/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Absolute API origin for the deployed admin app; empty in development, where Vite proxies /v1. */
  readonly VITE_API_BASE_URL?: string;
}
