package ttl

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

func TestLatency(t *testing.T) {

	var arrivedAt time.Time
	list := New[int](func(key int) {
		arrivedAt = time.Now()
		fmt.Printf("timed out: %d\n", key)
	})
	defer list.Stop()

	expectedAt := time.Now().Add(1 * time.Second)
	list.Put(42, 1*time.Second)

	<-list.Wait()
	fmt.Printf("list is %v late\n", arrivedAt.Sub(expectedAt))
}

func TestLen(t *testing.T) {
	list := New[int](func(key int) {})
	defer list.Stop()
	list.Put(1, 1*time.Hour)
	list.Put(2, 1*time.Hour)
	list.Put(3, 1*time.Hour)
	if l := list.Len(); l != 3 {
		t.Fatalf("len failed, expected 3, got %v", l)
	}
}

func TestPut(t *testing.T) {

	var list = New(func(key int) {})

	if err := list.Put(42, 1*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := list.Put(42, 1*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := list.Put(42, 1*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := list.Put(42, 1*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := list.Put(42, 1*time.Second); err != nil {
		t.Fatal(err)
	}
	if list.Len() != 1 {
		t.Fatal("put failed")
	}
	if err := list.Delete(42); err != nil {
		t.Fatal(err)
	}
	if err := list.Delete(42); err != ErrNotFound {
		t.Fatal("put failed: expected ErrNotFound")
	}

	if err := list.Put(42, 1*time.Second); err != nil {
		t.Fatal(err)
	}
	<-list.Wait()

	list.Stop()

	if err := list.Put(42, 1*time.Second); err != ErrNotRunning {
		t.Fatal("expected ErrNotRunning")
	}
}

func TestDelete(t *testing.T) {
	list := New[int](func(key int) {})

	list.Put(42, 1*time.Hour)
	if err := list.Delete(42); err != nil {
		t.Fatal(err)
	}
	if err := list.Delete(69); err != ErrNotFound {
		t.Fatal("expected ErrNotFound")
	}
	list.Stop()
	if err := list.Delete(42); err != ErrNotRunning {
		t.Fatal("expected ErrNotRunning")
	}
}

func TestPutAscending(t *testing.T) {
	list := New[int](func(key int) {})
	for i := 0; i < 10; i++ {
		list.Put(i, time.Duration(i)*time.Hour)
	}
}

func TestPutDescending(t *testing.T) {
	list := New[int](func(key int) {})
	for i := 0; i < 10; i++ {
		list.Put(i+1, time.Duration(10-i+1)*time.Hour)
	}
}

func TestPutRandom(t *testing.T) {
	list := New[int](func(key int) {})
	keys := rand.Perm(10)
	for i := 0; i < 10; i++ {
		list.Put(keys[i], time.Duration(keys[i])*time.Hour)
	}
}

func TestRandom(t *testing.T) {
	const numLoops int = 1e3
	list := New[int](func(key int) { fmt.Printf("timed out: %d\n", key) })
	defer list.Stop()
	keys := rand.Perm(numLoops)
	for i := 0; i < numLoops; i++ {
		list.Put(keys[i], time.Duration(keys[i])*time.Millisecond)
	}
	<-list.Wait()
}

func BenchmarkPutAscending(b *testing.B) {
	list := New[int](func(key int) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list.Put(i, time.Duration(i)*time.Hour)
	}
}

func BenchmarkPutDescending(b *testing.B) {
	list := New[int](func(key int) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list.Put(i+1, time.Duration(b.N-i+1)*time.Hour)
	}
}

func BenchmarkPutRandom(b *testing.B) {
	list := New[int](func(key int) {})
	keys := rand.Perm(b.N + 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list.Put(keys[i], time.Duration(keys[i])*time.Hour)
	}
}
