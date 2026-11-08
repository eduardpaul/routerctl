package poll

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeSleep records sleeps without waiting
func fakeSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	calls := &[]time.Duration{}
	old := sleep
	sleep = func(_ context.Context, d time.Duration) error {
		*calls = append(*calls, d)
		return nil
	}
	t.Cleanup(func() { sleep = old })
	return calls
}

// sequence returns a probe yielding the values in turn (the last repeats)
func sequence(count *int, values ...int) func(context.Context) (int, error) {
	return func(context.Context) (int, error) {
		v := values[min(*count, len(values)-1)]
		*count++
		return v, nil
	}
}

func TestReadyFirstAttempt(t *testing.T) {
	sleeps := fakeSleep(t)
	count := 0
	result, err := AsyncPollUntil(context.Background(), sequence(&count, 5), func(v int) bool { return v == 5 }, DefaultInterval, 3)
	if err != nil || result == nil || *result != 5 || count != 1 || len(*sleeps) != 0 {
		t.Fatal(result, err, count, *sleeps)
	}
}

func TestReadyAfterRetries(t *testing.T) {
	sleeps := fakeSleep(t)
	count := 0
	result, _ := AsyncPollUntil(context.Background(), sequence(&count, 0, 0, 7), func(v int) bool { return v == 7 }, 2*time.Second, 5)
	if result == nil || *result != 7 || count != 3 || len(*sleeps) != 2 || (*sleeps)[1] != 2*time.Second {
		t.Fatal(result, count, *sleeps)
	}
}

func TestNeverReadyReturnsNone(t *testing.T) {
	sleeps := fakeSleep(t)
	count := 0
	result, err := AsyncPollUntil(context.Background(), sequence(&count, 0), func(v int) bool { return v == 1 }, DefaultInterval, 4)
	if result != nil || err != nil || count != 4 || len(*sleeps) != 3 {
		t.Fatal(result, err, count, *sleeps)
	}
}

func TestProbeErrorStops(t *testing.T) {
	fakeSleep(t)
	boom := errors.New("boom")
	_, err := AsyncPollUntil(context.Background(), func(context.Context) (int, error) { return 0, boom }, func(int) bool { return true }, DefaultInterval, 3)
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count := 0
	_, err := AsyncPollUntil(ctx, sequence(&count, 0), func(int) bool { return false }, time.Hour, 3)
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatal(err, count)
	}
}
