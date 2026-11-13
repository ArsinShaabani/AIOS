-- 0003_plugin_security: Ed25519 signatures and WASM metadata.
ALTER TABLE plugins ADD COLUMN signature TEXT NOT NULL DEFAULT '';
ALTER TABLE plugins ADD COLUMN wasm_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE plugins ADD COLUMN wasm_size INTEGER NOT NULL DEFAULT 0;
