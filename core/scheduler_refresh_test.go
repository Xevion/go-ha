package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Xevion/go-ha/internal"
)

type alwaysDue struct{}

func (alwaysDue) dynamic() bool { return true }
func (alwaysDue) Hash() uint64  { return 1 }
func (alwaysDue) NextTime(after time.Time) *time.Time {
	t := time.Now().Add(time.Millisecond)
	return &t
}

// refresh empties the queue to rebuild it. The run loop must not be able to
// look into that window and conclude there is nothing left to do.
func TestRefreshDoesNotKillTheRunLoop(t *testing.T) {
	s := newScheduler(internal.RealClock{})
	s.add(alwaysDue{}, func() {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rescheduled := make(chan struct{}, 1)
	var wg sync.WaitGroup
	loopDone := make(chan struct{})

	wg.Add(1)
	go func() { defer wg.Done(); defer close(loopDone); s.run(ctx, rescheduled, "probe") }()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			s.refresh(time.Now())
			select {
			case rescheduled <- struct{}{}:
			default:
			}
		}
	}()

	time.Sleep(time.Second)

	select {
	case <-loopDone:
		t.Fatal("the run loop exited on its own: it saw the queue empty mid-refresh")
	default:
	}

	cancel()
	wg.Wait()
}

// A queue that empties is not a queue that is finished: a trigger can retire,
// and an automation can be registered afterwards.
func TestRunLoopSurvivesAnEmptyQueue(t *testing.T) {
	s := newScheduler(internal.RealClock{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rescheduled := make(chan struct{}, 1)
	loopDone := make(chan struct{})
	go func() { defer close(loopDone); s.run(ctx, rescheduled, "probe") }()

	time.Sleep(50 * time.Millisecond)
	select {
	case <-loopDone:
		t.Fatal("the run loop gave up on an empty queue")
	default:
	}

	fired := make(chan struct{}, 1)
	s.add(alwaysDue{}, func() { fired <- struct{}{} })
	rescheduled <- struct{}{}

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("an automation registered after the queue emptied never ran")
	}

	cancel()
	<-loopDone
}
