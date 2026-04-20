# Channels

Channel usage patterns and best practices.

## Buffered vs Unbuffered

```go
// Unbuffered: Synchronous send/receive
ch := make(chan int)
ch <- 1      // Blocks until receiver ready
val := <-ch  // Blocks until sender ready

// Buffered: Asynchronous up to capacity
ch := make(chan int, 10)
ch <- 1      // Doesn't block if buffer not full
val := <-ch  // Doesn't block if buffer not empty
```

## Channel Closing

Only the sender should close a channel:

```go
// Sender
func producer(ch chan<- int) {
    for i := 0; i < 10; i++ {
        ch <- i
    }
    close(ch)  // Sender closes
}

// Receiver
func consumer(ch <-chan int) {
    for val := range ch {  // Automatically detects close
        fmt.Println(val)
    }
}
```

## Select Pattern

```go
select {
case <-ctx.Done():
    return ctx.Err()
case val := <-ch1:
    process(val)
case ch2 <- result:
    // sent result
}
```

## Never Use Buffered Channel as Mutex

```go
// ❌ Bad: Using buffered channel as mutex
var mu chan struct{}
mu = make(chan struct{}, 1)

func criticalSection() {
    mu <- struct{}{}        // acquire
    defer func() { <-mu }() // release
    // work
}

// ✅ Good: Use sync.Mutex
var mu sync.Mutex

func criticalSection() {
    mu.Lock()
    defer mu.Unlock()
    // work
}
```
