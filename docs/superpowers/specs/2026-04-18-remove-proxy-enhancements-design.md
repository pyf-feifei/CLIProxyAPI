# Remove Proxy Enhancements Design

**Date:** 2026-04-18

**Goal**

Remove all proxy-related enhancements added after `upstream/main`, while preserving the original upstream direct-networking behavior and upstream-native proxy support.

**Problem Summary**

This fork accumulated Hugging Face specific proxy bootstrap logic and additional proxy-oriented runtime behavior after diverging from `upstream/main`. The user wants those later additions removed completely, but does **not** want to remove proxy capabilities that already exist in `upstream/main`.

That means the target state is:

- keep upstream-native direct networking behavior
- keep upstream-native `proxy-url` and related baseline support
- remove all later-added HF/Qwen/xray/mihomo/subscription/probe/bypass logic

## Scope

### In Scope

- Revert or delete all post-upstream proxy enhancement files and code paths
- Remove HF-specific proxy bootstrap scripts, docs, and assets
- Remove local proxy runtime bootstrap behavior (`xray`, `mihomo`, subscription download, node probing)
- Remove Qwen-specific proxy/WAF-bypass additions introduced after upstream
- Preserve unrelated fixes that are not themselves proxy enhancements
- Add or update tests so the removal is verified

### Out of Scope

- Removing proxy features that already exist in `upstream/main`
- Removing ordinary upstream networking features such as:
  - management asset updates
  - version checks
  - OAuth flows
  - model registry updates
- Re-architecting HF deployment beyond deleting the fork-added proxy bootstrap path
- Cleaning unrelated upstream test failures that pre-exist this work

## Source-of-Truth Rule

For proxy-related behavior, `upstream/main` is the baseline truth.

Implementation rule:

- if a file or code path exists only to support later proxy enhancements, remove it
- if a file differs from upstream for both proxy and non-proxy reasons, remove only the proxy delta and keep the non-proxy fixes
- if a file is purely a proxy-enhancement artifact, prefer deleting or restoring to upstream instead of partial surgery

## Design Overview

Use a file-boundary-first cleanup strategy:

1. hard-delete or restore files whose purpose is overwhelmingly proxy bootstrap
2. surgically remove proxy deltas from mixed-responsibility files
3. keep recent non-proxy fixes already made in this branch
4. verify the remaining diff against `upstream/main` no longer contains the later proxy enhancement layer

This minimizes the chance of leaving hidden proxy residue behind.

## File Strategy

### Group A: Delete or Restore to Upstream

These are primarily or exclusively fork-added proxy enhancement artifacts:

- `Dockerfile`
- `start.sh`
- `xray-config.json`
- `deploy-hf.ps1`
- `deploy/hf-profile/Dockerfile`
- `deploy/hf-profile/README.md`
- `deploy/hf-profile/start.sh`

Expected treatment:

- restore `Dockerfile` and `start.sh` to upstream behavior
- delete `xray-config.json`
- delete `deploy-hf.ps1`
- delete entire `deploy/hf-profile/` directory

### Group B: Surgical Removal of Proxy Deltas

These files contain both proxy-related changes and other useful changes, so they must be edited carefully:

- `internal/api/handlers/management/auth_files.go`
- `internal/config/config.go`
- `sdk/cliproxy/auth/conductor.go`
- `sdk/cliproxy/auth/types.go`

Expected treatment:

- compare each file to `upstream/main`
- remove only later-added proxy enhancement logic
- keep unrelated fixes

### Group C: Keep Current Non-Proxy Fixes

These fixes should remain because they are not themselves proxy enhancements:

- `internal/api/handlers/management/config_basic.go`
- `internal/api/handlers/management/config_basic_test.go`
- `internal/store/gitstore.go`
- `internal/store/gitstore_test.go`
- `static/management.html`

Rationale:

- `config.yaml` fallback for the config page is not proxy functionality
- gitstore HTTP remote compaction fix addresses HF git transport reliability, not proxy execution
- synced management panel asset is UI/runtime compatibility work, not proxy bootstrap

## Behavioral Target After Cleanup

After the change:

- no subscription download remains
- no `mihomo` startup remains
- no `xray` startup remains
- no local socks5 proxy bootstrap remains
- no HF-specific Qwen proxy probing remains
- no later-added WAF-bypass code remains

But the repository still supports:

- upstream direct outbound networking
- upstream-native proxy configuration behavior already present in upstream

## Validation Strategy

### Static Validation

Run repository-wide searches to ensure later enhancement terms are gone from production paths:

- `CLASH_SUB_URL`
- `mihomo`
- `xray`
- Qwen proxy probe identifiers
- HF-only proxy bootstrap phrases

Searches should allow upstream-native generic `proxy-url` support to remain.

### Code Validation

At minimum:

- targeted tests for files changed during cleanup
- build verification of `./cmd/server`
- HF deployment guard tests may need removal or replacement depending on deleted files

### Diff Validation

After cleanup, review `upstream/main...HEAD` again and confirm:

- remaining differences are unrelated to the later proxy enhancement layer
- deleted artifacts are no longer present

## Risks

### Risk 1: Accidentally Removing Upstream Proxy Support

Mitigation:

- do not remove generic upstream `proxy-url` behavior
- use `upstream/main` as the comparison baseline before each edit

### Risk 2: Leaving Hidden HF Proxy Residue

Mitigation:

- delete whole files when possible
- use repository-wide grep validation after edits

### Risk 3: Losing Non-Proxy Fixes in Mixed Files

Mitigation:

- handle mixed files surgically instead of blanket revert
- keep the already-added config YAML fallback and gitstore HTTP fix

## Acceptance Criteria

The work is complete when all of the following are true:

1. The fork-added HF proxy bootstrap files are deleted or restored to upstream
2. No later-added `xray`/`mihomo`/subscription/probe code remains
3. Upstream-native proxy support remains intact
4. Non-proxy fixes explicitly marked to keep are still present
5. Verification commands pass for the touched areas and for the server build
6. Post-cleanup diff against `upstream/main` shows no remaining later proxy enhancement layer

