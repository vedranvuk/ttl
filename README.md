# TTL

Generic Time-To-Live list.

```Go
func ExampleTTL() {

	list := ttl.New[int](
		func(key int) {
			fmt.Printf("timed out: %v\n", key)
		},
	)
	list.Put(42, 1*time.Millisecond)

	<-list.Wait()
	list.Stop()

	// Output:
	// timed out: 42
}
```

Uses channels for internals sync, not compensated. Timeout events are always a few microseconds late.

## License

MIT