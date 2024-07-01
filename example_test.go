package ttl_test

import (
	"fmt"
	"time"

	"github.com/vedranvuk/ttl"
)

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
