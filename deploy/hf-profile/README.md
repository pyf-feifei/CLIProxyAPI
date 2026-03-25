# HF Deploy Profile

This directory is the only source of truth for Hugging Face Spaces deployment overrides.

Why it exists:

- `main` may continue to track upstream Docker/startup files.
- HF deployment needs a separate, known-good Qwen proxy bootstrap flow.
- Future upstream merges must not silently switch HF back to the legacy `xray` path.

Rules:

- HF deployment must overlay files from this directory on top of the exported repository snapshot.
- HF deployment must remove `xray-config.json` from the snapshot.
- Qwen OAuth must keep using `cfg.ProxyURL` as the single proxy source of truth.
- Do not add `QWEN_AUTH_PROXY_URL` back as a second HF-only runtime path.
- HF startup must fail fast when `CLASH_SUB_URL`, mihomo config generation, or the Qwen proxy probe is broken.
- HF startup must not silently clear `proxy-url` and continue serving a deployment that cannot complete Qwen OAuth.
- HF startup must probe candidate nodes against the real Qwen OAuth endpoint before serving traffic.
- HF startup must lock Qwen traffic to a verified node via a `select` group.
- HF startup must not use `url-test`, `auto`, or gstatic probes for Qwen routing.
