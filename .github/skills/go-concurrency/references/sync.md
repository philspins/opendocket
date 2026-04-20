# Sync

sync package for concurrent access coordination.

## Mutex

```go
var mu sync.Mutex
var data map[string]string

func Write(key, value string) {
    mu.Lock()
    defer mu.Unlock()
    data[key] = value
}

func Read(key string) string {
    mu.Lock()
    defer mu.Unlock()
    return data[key]
}
```

## RWMutex (for read-heavy workloads)

```go
var mu sync.RWMutex
var data map[string]string

func Read(key string) string {
    mu.RLock()
    defer mu.RUnlock()
    return data[key]
}

func Write(key, value string) {
    mu.Lock()
    defer mu.Unlock()
    data[key] = value
}
```

## WaitGroup

```go
func FetchAll(urls []string) ([]string, error) {
    var wg sync.WaitGroup
    results := make([]string, len(urls))
    errs := make(chan error, len(urls))

    for i, url := range urls {
        wg.Add(1)
        go func(i int, url string) {
            defer wg.Done()
            resp, err := http.Get(url)
            if err != nil {
                errs <- err
                return
            }
            defer resp.Body.Close()
            body, _ := io.ReadAll(resp.Body)
            results[i] = string(body)
        }(i, url)
    }

    wg.Wait()
    close(errs)

    for err := range errs {
        if err != nil {
            return results, err
        }
    }

    return results, nil
}
```

## Once

```go
var once sync.Once
var instance *Singleton

func GetInstance() *Singleton {
    once.Do(func() {
        instance = &Singleton{}
    })
    return instance
}
```

## Pool (for reusable objects)

```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func getBuffer() *bytes.Buffer {
    return bufferPool.Get().(*bytes.Buffer)
}

func putBuffer(b *bytes.Buffer) {
    b.Reset()
    bufferPool.Put(b)
}
```
