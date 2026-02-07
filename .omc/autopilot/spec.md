# Git Branch Compatibility Analysis Report

## Summary

**Current Branch:** `new` (15 commits ahead of common ancestor)
**Target Branch:** `origin/main` (27 commits ahead of common ancestor)
**Common Ancestor:** `a35d66443b211ada8ea03378ea3464321796c89f`

## Compatibility Status: COMPATIBLE WITH MINOR CONFLICTS

### Files with Conflicts (2 files)

| File | Conflict Type | Risk Level | Resolution |
|------|---------------|------------|------------|
| `config.example.yaml` | Additive changes in different sections | LOW | Auto-merge likely clean |
| `sdk/cliproxy/auth/conductor.go` | Function signature + new function | MEDIUM | Manual merge required |

### Files Only in Origin/main (18 files)

- `assets/cubence.png` (deleted)
- `internal/auth/kimi/kimi.go`
- `internal/auth/kiro/aws_auth.go`
- `internal/runtime/executor/kimi_executor.go`
- `internal/runtime/executor/kimi_executor_test.go` (new)
- `internal/runtime/executor/kiro_executor.go`
- `internal/store/gitstore.go`
- `internal/translator/codex/claude/codex_claude_response.go`
- `internal/translator/gemini-cli/openai/chat-completions/gemini-cli_openai_response.go`
- `internal/translator/gemini/openai/chat-completions/gemini_openai_response.go`
- `internal/translator/kiro/claude/kiro_claude_request.go`
- `internal/translator/kiro/common/constants.go`
- `sdk/cliproxy/auth/conductor_availability_test.go` (new)
- `sdk/cliproxy/auth/oauth_model_alias.go`
- `sdk/cliproxy/auth/oauth_model_alias_test.go`
- `sdk/cliproxy/auth/selector.go`
- `sdk/cliproxy/auth/selector_test.go` (new)
- `test/config_migration_test.go` (deleted)

### Files Only in Current Branch (25 files)

- `.github/workflows/publish.yml` (new)
- `.github/workflows/release.yaml`
- `.gitignore`
- `MERGE_PLAN.md` (new)
- `examples/utls-demo/main.go` (new)
- `go.mod`, `go.sum`
- `internal/api/modules/amp/amp.go`
- `internal/api/modules/amp/proxy.go`
- `internal/api/modules/amp/proxy_test.go`
- `internal/api/modules/amp/routes_test.go`
- `internal/auth/kiro/aws.go`
- `internal/cmd/kiro_login.go`
- `internal/config/sdk_config.go`
- `internal/runtime/executor/antigravity_executor.go`
- `internal/translator/antigravity/claude/antigravity_claude_request.go`
- `internal/translator/antigravity/claude/antigravity_claude_request_optimized.go` (new)
- `internal/translator/antigravity/claude/antigravity_claude_request_v2.go` (new)
- `internal/translator/antigravity/openai/chat-completions/antigravity_openai_request.go`
- `internal/util/gemini_schema_benchmark_test.go` (new)
- `internal/util/gemini_schema_optimized.go` (new)
- `internal/util/proxy.go`
- `internal/util/utls.go` (new)
- `internal/util/utls_test.go` (new)
- `sdk/cliproxy/auth/conductor_overrides_test.go`

---

## Detailed Conflict Analysis

### 1. config.example.yaml - LOW RISK

**Origin/main changes:**
- Added `kimi` to oauth-model-alias supported channels list
- Added kimi examples in oauth-model-alias and excluded-models sections

**Current branch changes:**
- Added `tls-fingerprint` configuration section (lines 73-83)

**Resolution:** These changes are in completely different sections. Git should auto-merge cleanly. If not, manually combine both changes.

### 2. sdk/cliproxy/auth/conductor.go - MEDIUM RISK

**Origin/main changes:**
1. Added `isRequestInvalidError()` function (new, lines ~1442-1454)
2. Added early return checks in:
   - `executeMixedOnce` (line ~610)
   - `executeCountMixedOnce` (line ~666)
   - `executeStreamMixedOnce` (line ~720)
3. Added check in `shouldRetryAfterError` (line ~1122)
4. Bug fix in `updateAggregatedAvailability`: changed `stateUnavailable = true` to `false` when `NextRetryAfter.IsZero()` (line ~1314)

**Current branch changes:**
1. Modified `nextQuotaCooldown` signature from:
   ```go
   nextQuotaCooldown(prevLevel int, disableCooling bool)
   ```
   to:
   ```go
   nextQuotaCooldown(prevLevel int, auth ...*Auth)
   ```
2. Updated calls in `MarkResult` and `applyAuthFailureState` to pass auth directly

**Resolution Strategy:**
1. Keep local branch's `nextQuotaCooldown` signature (more flexible)
2. Add all origin/main's new functions and checks
3. Apply the `updateAggregatedAvailability` bug fix
4. Adapt origin/main's `nextQuotaCooldown` call sites to use local signature

---

## Origin/main Feature Summary

### Kimi Provider Improvements
- Reduced redundant payload cloning
- Updated base URL and integrated ClaudeExecutor fallback
- Fixed tool-call reasoning_content normalization
- Added OAuth model-alias channel support
- Added excluded-models coverage with tests

### Kiro Improvements
- Handle empty content in current user message for compaction
- Added contextUsageEvent handler and simplified model structs

### Auth/Retry Improvements
- 400 invalid_request_error now returns immediately without retry
- Fixed aggregated availability calculation

### Store Improvements
- Added proper GC with Handler and interval gating

### Translator Improvements
- Normalized stop_reason/finish_reason usage
- Captured cached token count in usage metadata
- Corrected gemini-cli log prefix

### Other
- Fixed assistant placeholder text to prevent model parroting
- Removed Cubence sponsorship from README

---

## Current Branch Feature Summary

### Performance Optimizations
- Optimized JSON parsing in ConvertOpenAIRequestToAntigravity
- Optimized buildRequest to avoid gjson/sjson in hot path
- Implemented optimized JSON schema cleaner with caching

### New Features
- uTLS fingerprinting support for HTTP requests
- Per-auth override support for request_retry and disable_cooling

### CI/CD
- Restored publish.yml workflow with workflow_dispatch support

---

## Merge Recommendations

### Priority 1: Must Incorporate from Origin/main
1. `isRequestInvalidError` function and all its call sites
2. `updateAggregatedAvailability` bug fix
3. All Kimi-related fixes (isolated files, no conflict)
4. Store GC improvements
5. Translator normalization fixes

### Priority 2: Preserve from Current Branch
1. `nextQuotaCooldown` variadic signature
2. uTLS fingerprinting feature
3. All JSON/schema optimization code
4. Per-auth override support
5. CI workflow updates

### Merge Command Sequence
```bash
git fetch origin
git checkout new
git merge origin/main --no-commit

# Resolve conductor.go conflict manually:
# 1. Keep local nextQuotaCooldown signature
# 2. Add isRequestInvalidError function from origin
# 3. Add isRequestInvalidError checks to execute* functions
# 4. Add isRequestInvalidError check to shouldRetryAfterError
# 5. Apply updateAggregatedAvailability fix
# 6. Adapt nextQuotaCooldown call sites to pass auth

git add -A
git commit -m "merge: integrate origin/main changes with local optimizations"
go test ./...
```

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| conductor.go merge conflict | HIGH | LOW | Manual resolution following documented strategy |
| Test failures after merge | MEDIUM | MEDIUM | Run full test suite, fix as needed |
| Performance regression | LOW | MEDIUM | Run benchmarks before/after merge |
| Feature incompatibility | LOW | LOW | Code changes are in isolated areas |

## Conclusion

The two branches are **highly compatible**. The only significant conflict is in `conductor.go`, where both branches made independent improvements to the retry/cooldown logic. The resolution is straightforward:

1. **Keep local signature change** (more flexible API)
2. **Add origin's new feature** (invalid request detection)
3. **Apply origin's bug fix** (availability calculation)

All other changes are to different files and will merge cleanly.
