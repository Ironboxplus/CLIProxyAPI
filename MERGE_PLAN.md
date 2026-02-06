# Merge Plan: new -> origin/main

**Created**: 2026-02-06
**Branch**: `new`
**Target**: `origin/main`
**Merge Base**: `dbecf533` (Merge pull request #181)

---

## Executive Summary

The `new` branch contains 35 commits (excluding merges) on top of the merge base with `origin/main`. These commits include:

1. **Performance optimizations** (to be preserved)
2. **Deprecated SkipIncrement/rate limiting code** (already cleaned up in later commits)
3. **gin-contrib/pprof dependency** (to be removed - origin/main has its own pprof)
4. **CI workflow improvements** (to be preserved)

The recommended strategy is **cherry-pick** rather than rebase, as many intermediate commits introduced and then removed features (SkipIncrement, rate limiters), and we only need the final cleaned-up state.

---

## Commit Analysis

### Category 1: Performance Optimizations (KEEP)

These commits add valuable performance improvements and should be preserved:

| Commit | Description | Files |
|--------|-------------|-------|
| `5dd64c7b` | feat(schema): implement optimized JSON schema cleaner with caching | `gemini_schema_optimized.go`, benchmark tests |
| `82b57f36` | feat(antigravity): optimize JSON handling in geminiToAntigravity | antigravity executor |
| `7cd5ed48` | feat(antigravity): add optimized request conversion and fallback | `antigravity_claude_request_optimized.go` |
| `18806dbf` | perf: optimize buildRequest to avoid gjson/sjson in hot path | antigravity translator |
| `6ecdd319` | perf: optimize JSON parsing in ConvertOpenAIRequestToAntigravity | antigravity translator |
| `9ff6a14c` | perf: collect content nodes to optimize sjson.SetRawBytes calls | antigravity translator |
| `810bca92` | fix: flatten type arrays in cleanSchemaInPlace for Gemini API | schema optimizer |

### Category 2: uTLS Fingerprinting Support (KEEP)

| Commit | Description | Files |
|--------|-------------|-------|
| `63f237e8` | feat: add uTLS fingerprinting support for HTTP requests | `utls.go`, `utls_test.go`, `proxy.go`, config, amp module |

### Category 3: Per-Auth Override Support (KEEP)

| Commit | Description | Files |
|--------|-------------|-------|
| `bb214f84` | feat: restore per-auth override support for request_retry and disable_cooling | `conductor.go`, tests |
| `f8e825df` | feat: integrate WithSkipPersist to prevent file watcher write-back loops | `conductor.go` |

### Category 4: Bug Fixes (KEEP)

| Commit | Description | Files |
|--------|-------------|-------|
| `717215eb` | fix: use pointer for ThinkingBudget to properly serialize zero values | `antigravity_claude_request_optimized.go` |
| `12506fa6` | fix(kiro): display full error message for disabled Google login | kiro login |
| `7cfc6995` | fix: add missing net/url import for auth_files.go | auth_files.go |

### Category 5: CI Workflow (KEEP)

| Commit | Description | Files |
|--------|-------------|-------|
| `5f4e9ac7` | ci(workflow): auto-build on main push, selectable platforms | `.github/workflows/publish.yml` |

### Category 6: Cleanup Commits (KEEP - Final State)

These commits clean up deprecated code that was introduced and then removed:

| Commit | Description | Files |
|--------|-------------|-------|
| `d77314c6` | refactor: remove SkipIncrement exponential backoff, keep performance optimizations | conductor, types |
| `382c6021` | fix: remove SkipIncrement references from management API | auth_files.go |
| `6a3e48b1` | chore: remove rate_limiter, concurrency_limiter and related tests | deleted files |
| `ac5ed6eb` | refactor: simplify antigravity_executor by using origin/main version | antigravity_executor.go |

### Category 7: TO BE DISCARDED

These commits introduced code that was later removed or conflicts with origin/main:

| Commit | Description | Reason |
|--------|-------------|--------|
| `93ca7bb0` | feat(pprof): register pprof for performance profiling | origin/main has its own pprof implementation |
| `4caf45cd` | feat(auth): implement exponential backoff polling for 429 | Superseded by later cleanup |
| `26fc0b4e` | feat(auth): add warning logs, API quota fields | Superseded by cleanup |
| `180fb883` | feat(auth): improve exponential backoff with SkipIncrement | Superseded by d77314c6 |
| `719f0fae` | fix: add HTTPStatus to auth errors | Part of removed SkipIncrement |
| `d7eb3e6f` | fix: update skip increment cap to 64 | Superseded by cleanup |
| `0a1504be` | Implement adaptive concurrency and rate limiting | Removed in 6a3e48b1 |
| `43b0a092` | fix: improve concurrency limiter | Removed in 6a3e48b1 |
| `c55780c8` | Fixed: #1077 | Check if still needed |
| `bcc16a27` | fix(service): cache executor instances | Removed in ac5ed6eb |
| `a39fba1b` | fix: implement SkipIncrement exponential backoff | Superseded by d77314c6 |
| `48397192` | fix(conductor): add SkipIncrement logic | Superseded by d77314c6 |
| `c5088d11` | fix(conductor): handle 400 Bad Request errors | May need review |
| `4d1bbddf` | fix: update quota fields individually | May need review |
| `d17ed61e` | fix: reduce test sleep time | May need review |
| `6f7801ff` | fix: update cache function signatures | Intermediate fix |
| `550c7194` | refactor: revert to optimized version, remove V2 | Intermediate refactor |

---

## Files with Potential Conflicts

Based on the diff analysis, these files are modified in both branches:

### High Conflict Risk

| File | new Changes | origin/main Changes | Resolution Strategy |
|------|-------------|---------------------|---------------------|
| `internal/api/server.go` | pprof.Register (to remove) | Kimi auth route added | Take origin/main, remove pprof import |
| `internal/config/config.go` | AntigravityRateLimitConfig removed | Migration disabled | Take origin/main |
| `sdk/cliproxy/service.go` | executorCache removed | Kimi executor added | Take origin/main |
| `sdk/cliproxy/auth/conductor.go` | per-auth overrides, SkipPersist | Different retry logic | Manual merge needed |
| `go.mod` | gin-contrib/pprof, sonic v1.15 | Kimi deps, sonic v1.11 | Take origin/main deps, remove pprof |

### Medium Conflict Risk

| File | Resolution |
|------|------------|
| `internal/api/handlers/management/auth_files.go` | Take origin/main (has Kimi), ensure net/url import |
| `internal/translator/antigravity/claude/antigravity_claude_request.go` | Keep new's optimizations |

### New Files from new Branch (Must Add)

These files are new in `new` branch and should be added:

- `internal/util/utls.go`
- `internal/util/utls_test.go`
- `internal/util/gemini_schema_optimized.go`
- `internal/util/gemini_schema_optimized_test.go`
- `internal/util/gemini_schema_optimized_edge_test.go`
- `internal/util/gemini_schema_benchmark_test.go`
- `internal/translator/antigravity/claude/antigravity_claude_request_optimized.go`
- `examples/utls-demo/main.go`

---

## Recommended Merge Strategy: Selective Cherry-Pick

Given the complexity (many commits introduced then removed), the cleanest approach is:

### Step 1: Create Working Branch

```powershell
# Create a new branch from origin/main for the merge
git checkout origin/main
git checkout -b merge-new-optimizations
```

### Step 2: Cherry-Pick Performance Optimization Commits

```powershell
# Schema optimization (creates new files, low conflict risk)
git cherry-pick 5dd64c7b

# Antigravity optimizations
git cherry-pick 82b57f36
git cherry-pick 7cd5ed48
git cherry-pick 18806dbf
git cherry-pick 6ecdd319
git cherry-pick 9ff6a14c
git cherry-pick 810bca92
```

### Step 3: Cherry-Pick uTLS Support

```powershell
# uTLS fingerprinting (new files + config changes)
git cherry-pick 63f237e8
```

**Conflict Resolution for 63f237e8:**
- `go.mod`: Add `github.com/refraction-networking/utls` dependency
- `config.example.yaml`: Add `utls-fingerprint` option
- `internal/config/sdk_config.go`: Add UTLSFingerprint field

### Step 4: Cherry-Pick Per-Auth Overrides

```powershell
# Per-auth override support
git cherry-pick bb214f84
git cherry-pick f8e825df
```

**Conflict Resolution for bb214f84/f8e825df:**
- `sdk/cliproxy/auth/conductor.go`: Merge per-auth override functions with origin/main's retry logic
- Keep origin/main's `shouldRetryAfterError` signature changes
- Add `quotaCooldownDisabledForAuth()` and `closestCooldownWait()` override logic

### Step 5: Cherry-Pick Bug Fixes

```powershell
# ThinkingBudget pointer fix
git cherry-pick 717215eb

# Kiro error message fix
git cherry-pick 12506fa6
```

### Step 6: Cherry-Pick CI Workflow

```powershell
# CI workflow improvements
git cherry-pick 5f4e9ac7
```

### Step 7: Update go.mod

After all cherry-picks, run:

```powershell
# Ensure pprof dependency is NOT added
# Update dependencies
go mod tidy
```

---

## Manual Merge Tasks

### Task 1: conductor.go Reconciliation

The `sdk/cliproxy/auth/conductor.go` file has significant changes in both branches:

**origin/main changes:**
- Removed `maxAttempts` parameter from `shouldRetryAfterError`
- Changed to infinite retry loop: `for attempt := 0; ; attempt++`
- Removed per-auth override logic

**new branch changes (to preserve):**
- `quotaCooldownDisabledForAuth()` function
- Per-auth `request_retry` override in `closestCooldownWait()`
- `shouldSkipPersist()` check in `persist()` method
- `WithSkipPersist` context handling

**Merge approach:**
1. Start with origin/main's conductor.go
2. Add `quotaCooldownDisabledForAuth()` function
3. Modify `closestCooldownWait()` to accept per-auth override
4. Add `shouldSkipPersist()` in persist() method
5. Update `nextQuotaCooldown()` to accept optional auth parameter

### Task 2: Verify gin-contrib/pprof Removal

Ensure these are NOT in the final code:

```go
// DO NOT INCLUDE:
import "github.com/gin-contrib/pprof"

// DO NOT INCLUDE:
pprof.Register(s.engine)
```

origin/main already has pprof at a different path - verify it works.

### Task 3: Test File Review

Some test files may need updates:

- `sdk/cliproxy/auth/conductor_overrides_test.go` - Ensure it works with origin/main's conductor
- `sdk/cliproxy/auth/selector_test.go` - Check for any changes needed

---

## Execution Commands (Full Sequence)

```powershell
# 1. Ensure we're in the repo
Set-Location "E:\Go\CLIProxyAPIPlus"

# 2. Fetch latest
git fetch origin

# 3. Create working branch
git checkout origin/main
git checkout -b merge-new-optimizations

# 4. Cherry-pick performance optimizations (in order)
git cherry-pick 5dd64c7b  # schema optimizer
git cherry-pick 82b57f36  # antigravity JSON handling
git cherry-pick 7cd5ed48  # optimized request conversion
git cherry-pick 18806dbf  # buildRequest optimization
git cherry-pick 6ecdd319  # JSON parsing optimization
git cherry-pick 9ff6a14c  # content nodes optimization
git cherry-pick 810bca92  # flatten type arrays fix

# 5. Cherry-pick uTLS (may need conflict resolution)
git cherry-pick 63f237e8

# 6. Cherry-pick per-auth overrides (likely needs manual merge)
git cherry-pick bb214f84
git cherry-pick f8e825df

# 7. Cherry-pick bug fixes
git cherry-pick 717215eb  # ThinkingBudget pointer
git cherry-pick 12506fa6  # Kiro error message

# 8. Cherry-pick CI
git cherry-pick 5f4e9ac7

# 9. Clean up dependencies
go mod tidy

# 10. Verify no pprof
Select-String -Path "go.mod" -Pattern "gin-contrib/pprof"
Select-String -Path "internal/api/server.go" -Pattern "pprof"

# 11. Run tests
go test ./...

# 12. If all good, merge to main
git checkout main
git merge merge-new-optimizations
```

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| conductor.go merge conflicts | High | High | Manual review, comprehensive testing |
| Missing dependencies | Medium | Medium | Run `go mod tidy`, test imports |
| Test failures | Medium | Medium | Run full test suite before merge |
| pprof accidentally included | Low | Low | Grep verification step |
| Broken per-auth overrides | Medium | High | Run conductor_overrides_test.go |

---

## Post-Merge Verification

1. **Build verification:**
   ```powershell
   go build -o CLIProxyAPIPlus ./cmd/server/
   ```

2. **Test suite:**
   ```powershell
   go test ./... -v
   ```

3. **Specific tests:**
   ```powershell
   go test ./sdk/cliproxy/auth/... -v
   go test ./internal/util/... -v
   go test ./internal/translator/antigravity/... -v
   ```

4. **Manual verification:**
   - Start server and verify pprof works at origin/main's path
   - Test uTLS fingerprinting with config option
   - Test per-auth override with custom request_retry value

---

## Alternative Strategy: Rebase

If you prefer a linear history, you can rebase only the essential commits:

```powershell
# Create a fresh branch
git checkout origin/main
git checkout -b new-rebased

# Interactive rebase to pick only essential commits
# (This is more complex due to dependencies between commits)
git rebase -i new --onto origin/main
```

**Note:** The interactive rebase approach is riskier because:
1. Many commits have interdependencies
2. Some commits were superseded by later commits
3. The cherry-pick approach gives more control

---

## Conclusion

The recommended approach is **selective cherry-pick** of 12-14 commits that represent the final desired state:

1. Performance optimizations (7 commits)
2. uTLS support (1 commit)
3. Per-auth overrides (2 commits)
4. Bug fixes (2 commits)
5. CI workflow (1 commit)

This avoids bringing in deprecated code (SkipIncrement, rate limiters, gin-contrib/pprof) while preserving all valuable improvements.
