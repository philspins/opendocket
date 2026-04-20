# Context

Context propagation and cancellation in Go.

## Context First

context.Context should be the first parameter:

```go
func DoSomething(ctx context.Context, arg string) error {
    // ...
}
```

## WithCancel

```go
func DoWork(ctx context.Context) error {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()  // Always call cancel

    go func() {
        <-ctx.Done()
        cleanup()
    }()

    // Do work...
    return nil
}
```

## WithTimeout

```go
func FetchWithTimeout(ctx context.Context, url string) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return err
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    return nil
}
```

## WithDeadline

```go
deadline := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
ctx, cancel := context.WithDeadline(ctx, deadline)
defer cancel()
```

## Propagating Context

```go
func (s *Service) ProcessOrder(ctx context.Context, orderID string) error {
    order, err := s.repo.FindOrder(ctx, orderID)  // Pass ctx
    if err != nil {
        return err
    }

    payment, err := s.payment.Charge(ctx, order.Total)  // Pass ctx
    if err != nil {
        return err
    }

    return s.shipment.Create(ctx, order)  // Pass ctx
}
```

## Checking Cancellation

```go
select {
case <-ctx.Done():
    return ctx.Err()  // Returns context.Canceled or context.DeadlineExceeded
case result := <-ch:
    return process(result)
}
```
