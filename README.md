# TTL

Generic concurency safe Time-To-Live list.

```Go
func ExampleTTL() {

	list := ttl.New[int](
		func(key int) {
			fmt.Printf("timed out: %v\n", key)
		},
	)
	list.Put(42, 1*time.Millisecond)

	<-list.Wait() // optionally wait for everything to time out.
	list.Stop()   // stops the internal worker.

	// Output:
	// timed out: 42
}
```

Uses channels for internals sync, operations are not time compensated. Timeout events are always a few microseconds late.

It stores keys in a slice sorted by key timeout in ascending order and continually resets a ticker that runs in a worker routine to a timeout of the next key in queue.

Has room for optimizations. Adding items whose timeout is always greater than the last in the list is fast but is slow if timeouts being added continually decrease, i.e. are put to the front of the key list.

```
goos: linux
goarch: amd64
pkg: github.com/vedranvuk/ttl
cpu: 11th Gen Intel(R) Core(TM) i7-1165G7 @ 2.80GHz
BenchmarkPutAscending
BenchmarkPutAscending-8           732739              1643 ns/op
BenchmarkPutDescending
BenchmarkPutDescending-8           40346            182233 ns/op
BenchmarkPutRandom
BenchmarkPutRandom-8               63727            146609 ns/op
PASS
ok      github.com/vedranvuk/ttl        20.472s
```

## License

MIT
