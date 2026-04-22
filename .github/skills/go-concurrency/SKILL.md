---
name: go-concurrency
description: Go concurrency patterns including goroutines, channels, context propagation, and sync primitives. Use when working with goroutines, channels, context.Context, or any concurrent Go code.
---

# Go Concurrency

Expert guidance for writing safe, leak-free concurrent Go code.

## Quick Reference

| Pattern | When to Use | Key Points |
|---------|-------------|------------|
| Goroutine + select | Long-running workers | Always have exit condition |
| context.WithCancel | Cancellable operations | defer cancel() |
| context.WithTimeout | Time-bound operations | Check ctx.Done() |
| sync.Mutex | Mutual exclusion | Lock/defer Unlock |
| sync.RWMutex | Read-heavy locking | Separate RLock/RUnlock |
| sync.WaitGroup | Wait for goroutines | Add before, Done after |
| errgroup.Group | Goroutines with errors | Wait collects first error |
| channel | Signaling/streaming | Close sender side |

## What Do You Need?

1. **Goroutine lifecycle** - Starting, stopping, leaking prevention
2. **Channel usage** - Buffered vs unbuffered, closing patterns
3. **Context propagation** - Passing context through call stack
4. **Sync primitives** - Mutex, RWMutex, WaitGroup, Once
5. **Error handling** - errgroup for concurrent errors

Specify a number or describe your concurrent Go code.

## Routing

| Response | Reference to Read |
|----------|-------------------|
| 1, "goroutine", "worker", "job" | [goroutines.md](./references/goroutines.md) |
| 2, "channel", "buffer", "send", "receive" | [channels.md](./references/channels.md) |
| 3, "context", "cancel", "timeout", "deadline" | [context.md](./references/context.md) |
| 4, "mutex", "lock", "race", "sync" | [sync.md](./references/sync.md) |
| 5, general concurrency | Read all references based on code |

## Critical Rules

- **Goroutines must exit**: Every goroutine needs a known exit path
- **No goroutine leaks**: Always have a way to signal shutdown
- **Context goes first**: context.Context should be the first parameter
- **Defer cancel()**: Always defer cancel() when using WithCancel/Timeout
- **Channel ownership**: Only the sender should close a channel
- **Avoid buffered channels as mutexes**: Use sync.Mutex instead

## Common Pitfalls

| Pitfall | Severity | Fix |
|---------|----------|-----|
| Goroutine that never exits | Critical | Add ctx.Done() select case |
| Missing defer cancel() | High | Always defer after WithCancel/Timeout |
| Using buffered channel as mutex | High | Use sync.Mutex |
| Not propagating context | High | Add ctx parameter throughout call stack |
| Race conditions | Critical | Use proper sync or eliminate shared state |
| Unbounded goroutine creation | High | Use worker pool pattern |
| Reading from closed channel | Medium | Always check ok value from range |

## Reference Index

| File | Topics |
|------|--------|
| [goroutines.md](./references/goroutines.md) | Lifecycle patterns, worker pools, leak prevention |
| [channels.md](./references/channels.md) | Buffered vs unbuffered, closing patterns, select |
| [context.md](./references/context.md) | Propagation, cancellation, timeouts, deadlines |
| [sync.md](./references/sync.md) | Mutex, RWMutex, WaitGroup, Once, Pool |

## Success Criteria

Code is concurrency-safe when:
- All goroutines have exit conditions
- Context is propagated through the call stack
- No race conditions (go test -race passes)
- Resources are cleaned up (defer cancel(), defer wg.Done())
- Channels are closed by sender only
- No buffered-channel-as-mutex patterns
