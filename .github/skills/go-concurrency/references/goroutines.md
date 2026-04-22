# Goroutines

Goroutine lifecycle patterns and leak prevention.

## Worker Pattern

```go
func Worker(ctx context.Context, jobs <-chan Job, results chan<- Result) {
    for {
        select {
        case <-ctx.Done():
            return  // Clean exit on context cancellation
        case job, ok := <-jobs:
            if !ok {
                return  // Channel closed, exit
            }
            results <- process(job)
        }
    }
}
```

## Goroutine Leak Prevention

Every goroutine must have an exit path:

```go
// ❌ Bad: No way to stop goroutine
func RunWorker() {
    go func() {
        for {
            doWork()  // Never exits!
        }
    }()
}

// ✅ Good: Context-based cancellation
func RunWorker(ctx context.Context) error {
    go func() {
        <-ctx.Done()
        cleanup()
    }()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case work := <-workChan:
            process(work)
        }
    }
}
```

## Worker Pool

```go
func workerPool(ctx context.Context, jobs <-chan Job, results chan<- Result, numWorkers int) {
    var wg sync.WaitGroup
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            worker(ctx, jobs, results)
        }()
    }
    wg.Wait()
}
```

## Key Points

- Always provide a way for goroutines to exit
- Use context.Context for cancellation
- Use sync.WaitGroup to wait for goroutine completion
- Never create unbounded goroutines (use worker pools)
