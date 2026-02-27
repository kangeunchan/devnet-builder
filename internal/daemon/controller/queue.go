// Package controller provides the controller manager and work queue for the daemon.
package controller

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

const (
	defaultBackoffBaseDelay = time.Second
	defaultBackoffMaxDelay  = 60 * time.Second
	defaultBackoffJitter    = 0.20
	defaultBackoffRetries   = 8
)

// RequeueResult describes the outcome of scheduling a retry.
type RequeueResult struct {
	Attempt  int
	Delay    time.Duration
	Terminal bool
}

// WorkQueue is a rate-limited deduplicating work queue.
// Items added while an item is being processed will be re-queued
// when Done is called. This is inspired by the Kubernetes workqueue.
type WorkQueue struct {
	// queue is the ordered list of items to process
	queue []interface{}

	// dirty tracks items that need processing
	dirty map[interface{}]struct{}

	// processing tracks items currently being processed
	processing map[interface{}]struct{}

	// cond is used to signal when items are added
	cond *sync.Cond

	// shuttingDown indicates the queue is shutting down
	shuttingDown bool

	// retries tracks how many times an item has been retried.
	retries map[interface{}]int

	// backoff config
	baseDelay    time.Duration
	maxDelay     time.Duration
	jitterFactor float64
	maxRetries   int
	randFloat64  func() float64
	afterFunc    func(time.Duration, func()) *time.Timer

	mu sync.Mutex
}

// NewWorkQueue creates a new work queue.
func NewWorkQueue() *WorkQueue {
	q := &WorkQueue{
		queue:      make([]interface{}, 0),
		dirty:      make(map[interface{}]struct{}),
		processing: make(map[interface{}]struct{}),
		retries:    make(map[interface{}]int),
		baseDelay:  defaultBackoffBaseDelay,
		maxDelay:   defaultBackoffMaxDelay,
		// Keep jitter modest to spread retries without making behavior erratic.
		jitterFactor: defaultBackoffJitter,
		maxRetries:   defaultBackoffRetries,
		randFloat64:  rand.Float64,
		afterFunc:    time.AfterFunc,
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Add marks an item as needing processing. If the item is already
// in the queue or being processed, it will be re-queued when Done
// is called for that item.
func (q *WorkQueue) Add(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.shuttingDown {
		return
	}

	// Mark as dirty
	if _, exists := q.dirty[item]; exists {
		// Already dirty, nothing to do
		return
	}
	q.dirty[item] = struct{}{}

	// If being processed, it will be re-added when Done is called
	if _, exists := q.processing[item]; exists {
		return
	}

	// Add to queue
	q.queue = append(q.queue, item)
	q.cond.Signal()
}

// Get blocks until an item is ready, returning the item and whether
// the queue is shutting down.
func (q *WorkQueue) Get() (interface{}, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}

	if q.shuttingDown {
		return nil, true
	}

	// Pop from front
	item := q.queue[0]
	q.queue = q.queue[1:]

	// Move from dirty to processing
	delete(q.dirty, item)
	q.processing[item] = struct{}{}

	return item, false
}

// Done marks an item as done processing. If the item was re-added
// while being processed, it will be re-queued.
func (q *WorkQueue) Done(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.processing, item)

	// If item was marked dirty while processing, re-add to queue
	if _, exists := q.dirty[item]; exists {
		q.queue = append(q.queue, item)
		q.cond.Signal()
	}
}

// Requeue adds the item back to the queue after processing fails.
// This is equivalent to calling Done() then Add().
func (q *WorkQueue) Requeue(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.processing, item)

	if q.shuttingDown {
		return
	}

	// Add back to dirty set and queue
	q.dirty[item] = struct{}{}
	q.queue = append(q.queue, item)
	q.cond.Signal()
}

// RequeueWithBackoff schedules a delayed retry using exponential backoff + jitter.
// Returns terminal=true when max retries is exceeded and no further retry is scheduled.
func (q *WorkQueue) RequeueWithBackoff(item interface{}) RequeueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.processing, item)

	if q.shuttingDown {
		return RequeueResult{Terminal: true}
	}

	attempt := q.retries[item] + 1
	if attempt > q.maxRetries {
		delete(q.dirty, item)
		return RequeueResult{
			Attempt:  attempt - 1,
			Terminal: true,
		}
	}

	q.retries[item] = attempt
	q.dirty[item] = struct{}{}

	delay := q.computeBackoffDelayLocked(attempt)
	q.afterFunc(delay, func() {
		q.enqueueIfDirty(item)
	})

	return RequeueResult{
		Attempt: attempt,
		Delay:   delay,
	}
}

// Forget clears retry bookkeeping for an item after successful processing.
func (q *WorkQueue) Forget(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.retries, item)
}

// NumRequeues returns the number of recorded retries for an item.
func (q *WorkQueue) NumRequeues(item interface{}) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.retries[item]
}

// MaxRetries returns the configured max retry count.
func (q *WorkQueue) MaxRetries() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.maxRetries
}

// SetBackoffConfig configures delay/jitter/retry policy. Intended for tests.
func (q *WorkQueue) SetBackoffConfig(baseDelay, maxDelay time.Duration, jitterFactor float64, maxRetries int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if baseDelay > 0 {
		q.baseDelay = baseDelay
	}
	if maxDelay > 0 {
		q.maxDelay = maxDelay
	}
	if jitterFactor >= 0 {
		q.jitterFactor = jitterFactor
	}
	if maxRetries > 0 {
		q.maxRetries = maxRetries
	}
}

// SetRandomSource overrides jitter source. Intended for tests.
func (q *WorkQueue) SetRandomSource(randFloat64 func() float64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if randFloat64 != nil {
		q.randFloat64 = randFloat64
	}
}

// SetAfterFunc overrides timer scheduling. Intended for tests.
func (q *WorkQueue) SetAfterFunc(afterFunc func(time.Duration, func()) *time.Timer) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if afterFunc != nil {
		q.afterFunc = afterFunc
	}
}

// Len returns the number of items in the queue.
func (q *WorkQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

// ShutDown signals the queue to stop processing.
func (q *WorkQueue) ShutDown() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.shuttingDown = true
	q.cond.Broadcast()
}

// ShuttingDown returns true if the queue is shutting down.
func (q *WorkQueue) ShuttingDown() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.shuttingDown
}

func (q *WorkQueue) computeBackoffDelayLocked(attempt int) time.Duration {
	exp := attempt - 1
	if exp < 0 {
		exp = 0
	}
	// Cap exponent to avoid overflow; delay is capped to maxDelay anyway.
	if exp > 30 {
		exp = 30
	}
	delay := q.baseDelay * time.Duration(1<<exp)
	if delay > q.maxDelay {
		delay = q.maxDelay
	}

	if q.jitterFactor <= 0 {
		return delay
	}

	jitterRange := float64(delay) * q.jitterFactor
	if jitterRange <= 0 {
		return delay
	}

	jitter := (q.randFloat64()*2 - 1) * jitterRange
	jittered := float64(delay) + jitter
	if jittered < 0 {
		return 0
	}
	if jittered > float64(math.MaxInt64) {
		return q.maxDelay
	}
	jitteredDelay := time.Duration(jittered)
	if jitteredDelay > q.maxDelay {
		return q.maxDelay
	}
	return jitteredDelay
}

func (q *WorkQueue) enqueueIfDirty(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.shuttingDown {
		return
	}
	if _, processing := q.processing[item]; processing {
		return
	}
	if _, dirty := q.dirty[item]; !dirty {
		return
	}
	if q.inQueueLocked(item) {
		return
	}

	q.queue = append(q.queue, item)
	q.cond.Signal()
}

func (q *WorkQueue) inQueueLocked(item interface{}) bool {
	for _, queued := range q.queue {
		if queued == item {
			return true
		}
	}
	return false
}
