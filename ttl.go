// Copyright 2024 Vedran Vuk. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package ttl

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// TTL is a Time-To-Live list of comparrable keys that exist only for the
// duration specified. When a key expires TTL fires the callback specified in
// [New].
//
// TTL worker should be stopped manually after use with [TTL.Stop].
//
// It works by maintaining an ascending sorted queue of key timeout times that
// get on the next tick that is upated each time a timeout, update, put or delete occur
// updates a ticker that ticks at the next timeout time in the queue and
// fires timeout callback. Precision is "okay", error is always a delay.
type TTL[K comparable] struct {
	queuemu    sync.Mutex
	waitersmu  sync.Mutex
	running    atomic.Bool      // true while worker is running,
	waiting    atomic.Bool      // true while a ticker is being waited on to fire.
	queue      []timeout[K]     // a slice of timeouts sorted by [timeout.When] asc
	dict       map[K]time.Time  // map of times when a key times out
	addTimeout chan timeout[K]  // worker comm for adding a new timeout
	delTimeout chan K           // worker comm for deleting a timeout
	pingWorker chan bool        // worker comm for starting and stopping worker
	errchan    chan error       // errchan is used to communicate an error from the worker to the caller method.
	cb         func(key K)      // timeout callback
	waiters    []chan time.Time //
}

// timeout stores a key queued for timeout in the ttl queue.
type timeout[K comparable] struct {
	// When is the time when the Key should time out.
	When time.Time
	// Key is the key to time out at When.
	Key K
}

// Returns a new TTL which calls the optional cb each time a key expires.
func New[K comparable](cb func(key K)) *TTL[K] {
	var p = &TTL[K]{
		dict:       make(map[K]time.Time),
		addTimeout: make(chan timeout[K]),
		delTimeout: make(chan K),
		pingWorker: make(chan bool),
		errchan:    make(chan error),
		cb:         cb,
	}
	go p.worker()
	<-p.pingWorker
	return p
}

// ErrNotRunning is returned when a timeout is being added or deleted to/from
// a ttl that has been stopped.
var ErrNotRunning = errors.New("ttl is not running")

// Len returns the number of events in the list left to fire.
func (self *TTL[K]) Len() (l int) {
	self.queuemu.Lock()
	l = len(self.queue)
	if self.waiting.Load() {
		l += 1
	}
	self.queuemu.Unlock()
	return
}

// Wait returns a channel that returns the current time when the TLL queue is
// empty and all events have fired.
func (self *TTL[K]) Wait() chan time.Time {
	c := make(chan time.Time)
	self.waitersmu.Lock()
	self.waiters = append(self.waiters, c)
	self.waitersmu.Unlock()
	return c
}

// Put adds the key to ttl which will last for duration.
// If key is already present its timeout is reset to duration.
// Returns ErrNotRunning if ttl is stopped.
func (self *TTL[K]) Put(key K, duration time.Duration) error {
	if !self.running.Load() {
		return ErrNotRunning
	}
	self.delTimeout <- key
	<-self.errchan
	self.addTimeout <- timeout[K]{time.Now().Add(duration), key}
	<-self.addTimeout
	return nil
}

// ErrNotFound is returned when delete does not find the item to be deleted.
var ErrNotFound = errors.New("not found")

// Delete removes a key from the list.
// Returns ErrNotRunning if ttl is stopped.
func (self *TTL[K]) Delete(key K) (err error) {
	if !self.running.Load() {
		return ErrNotRunning
	}
	self.delTimeout <- key
	err = <-self.errchan
	return
}

// Stop stops the worker. This method should be called on shutdown.
// Returns ErrNotRunning if ttl is already stopped.
func (self *TTL[K]) Stop() error {
	if !self.running.Load() {
		return ErrNotRunning
	}
	self.pingWorker <- true
	<-self.pingWorker
	return nil
}

// doOnTimeout calls ttl.cb if it's not nil and passes key to it.
func (self *TTL[K]) doOnTimeout(key K) {
	if self.cb != nil {
		self.cb(key)
	}
}

// worker is the main ttl logic. It enqueues, re-queues and deletes timeouts
// to/from the queue using channels in order to be concurency safe. It also
// drains the timeouts queue and fires callbacks for timed out timeouts.
func (self *TTL[K]) worker() {

	self.running.Store(true)
	self.pingWorker <- true

	var (
		b      bool
		dur    time.Duration
		key    K
		when   time.Time
		ticker = time.NewTicker(time.Second * 1)
	)

loop:
	for {
		select {
		case <-self.pingWorker:
			ticker.Stop()
			break loop
		case t := <-self.addTimeout:
			// Timeout is somehow before Now(), fire immediately without queue.
			if t.When.Before(time.Now()) {
				self.doOnTimeout(t.Key)
				self.addTimeout <- timeout[K]{}
				continue
			}
			// No active key, initialize it to received timeout.
			if !self.waiting.Load() {
				// Set active timeout to key being added.
				if dur = time.Until(t.When); dur > 0 {
					key, when = t.Key, t.When
					self.waiting.Store(true)
					ticker.Reset(dur)
					self.addTimeout <- timeout[K]{}
					continue
				}
				// Key being added has already timed out, fire callback.
				self.doOnTimeout(t.Key)
				key, when, b = self.advanceQueue(ticker)
				self.waiting.Store(b)
				self.addTimeout <- timeout[K]{}
				continue
			}
			// New timeout is before current tick being waited on,
			// replace current tick with the new one and reinsert
			// the tick that was being waited on into timeouts queue.
			if t.When.Before(when) {
				ticker.Stop()
				self.insertTimeout(timeout[K]{when, key})
				if dur = time.Until(t.When); dur > 0 {
					key, when = t.Key, t.When
					self.waiting.Store(true)
					ticker.Reset(dur)
				} else {
					self.doOnTimeout(t.Key)
					key, when, b = self.advanceQueue(ticker)
					self.waiting.Store(b)
				}
				self.addTimeout <- timeout[K]{}
				continue
			}
			// A ticker is active and new event is after current, queue it.
			self.insertTimeout(t)
			self.addTimeout <- timeout[K]{}
		case k := <-self.delTimeout:
			// Key being deleted is currently active.
			if k == key && self.waiting.Load() {
				ticker.Stop()
				key, when, b = self.advanceQueue(ticker)
				self.waiting.Store(b)
				self.errchan <- nil
				continue
			}
			// Delete key from queue.
			if idx, found := self.findTimeout(self.dict[key]); found {
				self.queuemu.Lock()
				self.queue = append(self.queue[:idx], self.queue[idx+1:]...)
				delete(self.dict, k)
				self.queuemu.Unlock()
				self.errchan <- nil
				continue
			}
			self.errchan <- ErrNotFound
		case <-ticker.C:
			ticker.Stop()
			if self.waiting.Load() {
				self.queuemu.Lock()
				delete(self.dict, key)
				self.queuemu.Unlock()
				self.doOnTimeout(key)
			}
			key, when, b = self.advanceQueue(ticker)
			self.waiting.Store(b)
		}
		// Fire wait waiters.
		if !self.waiting.Load() {
			self.waitersmu.Lock()
			now := time.Now()
			for _, c := range self.waiters {
				go func(c chan time.Time) {
					c <- now
				}(c)
			}
			self.waiters = nil
			self.waitersmu.Unlock()
		}
	}
	self.queuemu.Lock()
	clear(self.queue)
	clear(self.dict)
	self.queuemu.Unlock()
	self.running.Store(false)
	self.pingWorker <- false
}

// advanceQueue returns the next key and when time from the queue and resets the
// ticker to appropriate duration if a timeout entry exists in the queue.
// If now is past the next timeout in the queue the timeout is immediately
// fired as well as all following timeouts before now until a timeout that is
// after now is found. If the queue is empty the ticker is not reset and an
// empty key and zero time are returned.
func (self *TTL[K]) advanceQueue(ticker *time.Ticker) (key K, when time.Time, wait bool) {
	var dur time.Duration
	for {
		self.queuemu.Lock()

		if len(self.queue) == 0 {
			self.queuemu.Unlock()
			break
		}

		key, when = self.queue[0].Key, self.queue[0].When
		self.queue = self.queue[1:]
		delete(self.dict, key)

		self.queuemu.Unlock()

		if dur = time.Until(when); dur <= 0 {
			self.doOnTimeout(key)
			continue
		}

		ticker.Reset(dur)
		wait = true
		return
	}
	return *new(K), time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC), false
}

// insertTimeout inserts timeout into the timeouts slice sorted by timeout.When.
// It ignores a possible duplicate of timeout.Key in the queue; other logic
// handles key duplicates, specifically Put().
func (self *TTL[K]) insertTimeout(t timeout[K]) {

	self.queuemu.Lock()
	defer self.queuemu.Unlock()

	var i, j = 0, len(self.queue)
	for i < j {
		var h = int(uint(i+j) >> 1)
		if self.queue[h].When.Before(t.When) {
			i = h + 1
		} else {
			j = h
		}
	}
	// Deadlocks in a benchmark.
	// self.queue = slices.Insert(self.queue, i, t)

	// Deadlocks in a benchmark.
	// self.queue = append(self.queue, timeout[K]{})
	// copy(self.queue[i+1:], self.queue[i:])
	// self.queue[i] = t

	// Slower, does not deadlock.
	self.queue = append(self.queue[:i], append([]timeout[K]{t}, self.queue[i:]...)...)

	self.dict[t.Key] = t.When
}

// findTimeout binary searches the queue slice for a timeout by its when time
// and returns its index and true if found or an invalid index and false if not.
func (self *TTL[K]) findTimeout(when time.Time) (idx int, found bool) {

	self.queuemu.Lock()
	defer self.queuemu.Unlock()

	var (
		n    = len(self.queue)
		i, j = 0, n
	)
	for i < j {
		h := int(uint(i+j) >> 1)
		switch self.cmp(when, h) {
		case 1:
			i = h + 1
		case 0:
			return h, true
		case -1:
			j = h
		}
	}
	return i, i < n && self.cmp(when, i) == 0
}

// cmpTime compares v with with when time of a timeout in queue at i.
// Returns -1 if v is less, 0 if same, 1 if more.
func (self *TTL[K]) cmp(v time.Time, i int) int {
	if v.Before(self.queue[i].When) {
		return -1
	} else if v.After(self.queue[i].When) {
		return 1
	}
	return 0
}
