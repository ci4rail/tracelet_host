package lsiclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPosQueue(t *testing.T) {
	pq := NewPosQueue(3, 100)
	assert.NotNil(t, pq)

	// empty queue
	_, _, err := pq.Pop(10*time.Millisecond)
	assert.Equal(t, ErrTimeout, err)

	// 2 items
	pq.PushOverwrite([]byte("m1"), true)
	pq.PushOverwrite([]byte("m2"), true)

	msg, have, err := pq.Pop(10*time.Millisecond)
	assert.NoError(t, err)
	assert.True(t, have)
	assert.Equal(t, "m1", string(msg))

	msg, have, err = pq.Pop(10*time.Millisecond)
	assert.NoError(t, err)
	assert.True(t, have)
	assert.Equal(t, "m2", string(msg))

	assert.False(t, pq.IsFull())
	_, _, err = pq.Pop(10*time.Millisecond)
	assert.Equal(t, ErrTimeout, err)


	// overwrite
	pq.PushOverwrite([]byte("m1"), true)
	pq.PushOverwrite([]byte("m2"), true)
	pq.PushOverwrite([]byte("m3"), true)
	assert.True(t, pq.IsFull())
	pq.PushOverwrite([]byte("m4"), true)

	msg, have, err = pq.Pop(10*time.Millisecond)
	assert.NoError(t, err)
	assert.True(t, have)
	assert.Equal(t, "m2", string(msg))

	assert.False(t, pq.IsFull())

	msg, have, err = pq.Pop(10*time.Millisecond)
	assert.NoError(t, err)
	assert.True(t, have)
	assert.Equal(t, "m3", string(msg))

	msg, have, err = pq.Pop(10*time.Millisecond)
	assert.NoError(t, err)
	assert.True(t, have)
	assert.Equal(t, "m4", string(msg))

	_, _, err = pq.Pop(10*time.Millisecond)
	assert.Equal(t, ErrTimeout, err)

	// wait for item
	go func() {
		time.Sleep(50 * time.Millisecond)
		pq.PushOverwrite([]byte("m5"), true)
	}()

	msg, have, err = pq.Pop(200 * time.Millisecond)
	assert.NoError(t, err)
	assert.True(t, have)
	assert.Equal(t, "m5", string(msg))
}
