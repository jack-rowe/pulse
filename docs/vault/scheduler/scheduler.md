# scheduler/scheduler.go

## Purpose

Orchestrates periodic health checks across configured endpoints, managing state transitions and alert deduplication.

## Key Types

### `Target`
- `Name` — unique endpoint identifier, used as key in state map
- `Interval` — check frequency; also determines jitter range (jitter = rand 0 to Interval/2)
- `FailThreshold` — consecutive failures before DOWN alert; **defaults to 3 if <= 0** (default lives in `handleStateChange`, not config)
- `Checker` — implements `checker.Checker`, executes the actual health check

### `Scheduler`
- `store` — persists every check result via `SaveResult`
- `notifier` — sends alerts on state transitions (DOWN and recovery)
- `mu` — guards `states` map; all state reads/writes must hold this
- `states` — `map[string]*endpointState`, single source of truth for endpoint status

### `endpointState`
- `currentStatus` — `StatusUp` or `StatusDown`; initialized to `StatusUp` on first check
- `consecutiveFails` — increments on DOWN, resets to 0 on UP
- `alerted` — dedup flag; true after DOWN alert sent, false after recovery

## How It Works

### `Start` vs `StartAsync`
- `Start` — blocks until `ctx` cancelled; uses `WaitGroup` to wait for all target goroutines
- `StartAsync` — fire-and-forget; returns immediately; caller must manage ctx lifecycle
- Use `Start` for CLI/main process, `StartAsync` for tests or when API server needs to run alongside

### Startup Jitter
- Random delay: `0` to `Interval/2`
- **Purpose**: prevents thundering herd when service boots with many targets
- Respects ctx cancellation during jitter wait

### Check Execution Flow
1. Jitter delay (once, on startup)
2. Immediate first check via `executeCheck`
3. Ticker fires every `Interval`
4. `executeCheck`:
   - Calls `Checker.Check(ctx)` — context passed through
   - Builds `store.CheckRecord` from result
   - `store.SaveResult` — errors logged, never fatal
   - `handleStateChange` — updates state, fires alerts if needed

## State Machine

### Transitions

```
           ┌─────────────────────────────────────────┐
           │                                         │
           ▼                                         │
┌──────────────────┐   consecutiveFails >= threshold ┌─────────────────┐
│   UP             │ ─────────────────────────────▶ │   DOWN          │
│   alerted=false  │   (fires DOWN alert once)       │   alerted=true  │
│   fails=0        │                                 │   fails=N       │
└──────────────────┘ ◀───────────────────────────── └─────────────────┘
                       StatusUp received
                       (fires RECOVERY alert, resets fails)
```

### DOWN Trigger
- `consecutiveFails` increments on each `StatusDown` result
- Alert fires when `consecutiveFails >= threshold` **AND** `!alerted`
- `alerted = true` prevents duplicate alerts while endpoint remains down

### Recovery Trigger
- `StatusUp` result when `alerted == true`
- Sends recovery notification
- Resets: `consecutiveFails = 0`, `alerted = false`, `currentStatus = StatusUp`

### `FailThreshold` Behavior
- If target sets `FailThreshold <= 0`, defaults to `3`
- Default is applied at check time, not at config load — allows zero-config targets

### `alerted` Flag
- Guards against: spamming DOWN alerts every failed check
- Set to `true` when DOWN alert sent
- Reset to `false` on recovery only
- Consequence: if threshold is 3 and check fails 10 times, only 1 DOWN alert sent

## Concurrency Model

### Mutex (`s.mu`)
- **Protects**: `s.states` map — both the map itself and the `*endpointState` values
- `RLock`: `GetState` — read-only snapshot for API layer
- `Lock`: `handleStateChange` — mutates state, may fire notifications

### Goroutine-per-Target
- Each target runs in its own goroutine
- No shared state between targets except via mutex-protected `states` map
- Goroutines are independent — one slow/stuck check doesn't block others
- Graceful shutdown: ctx cancellation logged per-target, ticker stopped via defer

### Gotchas
- `executeCheck` calls `handleStateChange` which holds Lock while calling `notifier.Notify` — slow notifier blocks state updates for that target
- `states` map entry created lazily on first check, not at scheduler start — `GetState` may return empty map before any checks complete
- Notifier errors are logged but do not affect state transitions — state will show DOWN even if notification failed
