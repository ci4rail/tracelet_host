package lsiclient

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrTimeout     = errors.New("timeout waiting for item")
	ErrInvalidSize = errors.New("destination buffer too small")
)

type posQueueItem struct {
	msg             []byte // preallocated to maxMsgSize; only first msgLen bytes valid
	msgLen          int
	haveCompleteMsg bool
}

// PosQueue: fixed-size ring buffer with overwrite-on-full semantics.
// itemsAvail is a counting semaphore with max count = capacity.
type PosQueue struct {
	items      []posQueueItem
	wr, rd, n  int
	maxMsgSize int

	mu         sync.Mutex
	itemsAvail chan struct{} // counting semaphore: buffered to capacity
}

// NewPosQueue preallocates per-slot message buffers and an empty counting semaphore.
func NewPosQueue(maxItems int, maxMsgSize int) *PosQueue {
	q := &PosQueue{
		items:      make([]posQueueItem, maxItems),
		maxMsgSize: maxMsgSize,
		itemsAvail: make(chan struct{}, maxItems),
	}
	for i := 0; i < maxItems; i++ {
		q.items[i].msg = make([]byte, maxMsgSize)
	}
	return q
}

// PushOverwrite inserts an item; if full, it drops the oldest.
// After pushing, it "gives" the counting semaphore non-blocking (like FreeRTOS when at max count).
func (q *PosQueue) PushOverwrite(msg []byte, haveCompleteMsg bool) {
	q.mu.Lock()

	// Drop oldest if full
	if q.n == cap(q.items) {
		q.rd = (q.rd + 1) % cap(q.items)
		q.n--
	}

	item := &q.items[q.wr]
	copyLen := len(msg)
	if copyLen > q.maxMsgSize {
		copyLen = q.maxMsgSize // defensive cap, analogous to memcpy with bounded dst
	}
	copy(item.msg[:copyLen], msg[:copyLen])
	item.msgLen = copyLen
	item.haveCompleteMsg = haveCompleteMsg

	q.wr = (q.wr + 1) % cap(q.items)
	q.n++

	q.mu.Unlock()

	// counting semaphore give (non-blocking; discard if already at max count)
	select {
	case q.itemsAvail <- struct{}{}:
	default:
		// already at max count; match FreeRTOS xSemaphoreGive on full: no-op
	}
}

// Pop waits for an available item (using the counting semaphore) up to timeout,
// then copies it into dst. Returns (haveCompleteMsg, error).
// timeout == 0 => non-blocking; timeout < 0 => block indefinitely.
func (q *PosQueue) Pop(timeout time.Duration) ([]byte, bool, error) {
	// Take from counting semaphore with timeout behavior
	switch {
	case timeout == 0:
		select {
		case <-q.itemsAvail:
		default:
			return nil, false, ErrTimeout
		}
	case timeout < 0:
		<-q.itemsAvail
	default:
		timer := time.NewTimer(timeout)
		select {
		case <-q.itemsAvail:
			if !timer.Stop() {	
				<-timer.C
			}
		case <-timer.C:
			return nil, false, ErrTimeout
		}
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	item := &q.items[q.rd]

	dst := make([]byte, item.msgLen)
	copy(dst, item.msg[:item.msgLen])
	have := item.haveCompleteMsg

	q.rd = (q.rd + 1) % cap(q.items)
	q.n--

	return dst, have, nil
}

// IsFull reports whether the queue is full right now.
func (q *PosQueue) IsFull() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.n == cap(q.items)
}
