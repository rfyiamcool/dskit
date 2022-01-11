package concurrency

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestForEachUser(t *testing.T) {
	var (
		ctx = context.Background()

		// Keep track of processed users.
		processedMx sync.Mutex
		processed   []string
	)

	input := []string{"a", "b", "c"}

	err := ForEachUser(ctx, input, 2, func(ctx context.Context, user string) error {
		processedMx.Lock()
		defer processedMx.Unlock()
		processed = append(processed, user)
		return nil
	})

	require.NoError(t, err)
	assert.ElementsMatch(t, input, processed)
}

func TestForEachUser_ShouldContinueOnErrorButReturnIt(t *testing.T) {
	var (
		ctx = context.Background()

		// Keep the processed users count.
		processed atomic.Int32
	)

	input := []string{"a", "b", "c"}

	err := ForEachUser(ctx, input, 2, func(ctx context.Context, user string) error {
		if processed.CAS(0, 1) {
			return errors.New("the first request is failing")
		}

		// Wait 1s and increase the number of processed jobs, unless the context get canceled earlier.
		select {
		case <-time.After(time.Second):
			processed.Add(1)
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	})

	require.EqualError(t, err, "the first request is failing")

	// Since we expect it continues on error, the number of processed users should be equal to the input length.
	assert.Equal(t, int32(len(input)), processed.Load())
}

func TestForEachUser_ShouldReturnImmediatelyOnNoUsersProvided(t *testing.T) {
	require.NoError(t, ForEachUser(context.Background(), nil, 2, func(ctx context.Context, user string) error {
		return nil
	}))
}

func TestForEachJob(t *testing.T) {
	var (
		ctx = context.Background()
	)

	jobs := []string{"a", "b", "c"}
	processed := make([]string, len(jobs))

	err := ForEachJob(ctx, len(jobs), 2, func(ctx context.Context, idx int) error {
		processed[idx] = jobs[idx]
		return nil
	})

	require.NoError(t, err)
	assert.ElementsMatch(t, jobs, processed)
}

func TestForEachJob_ShouldBreakOnFirstError_ContextCancellationHandled(t *testing.T) {
	var (
		ctx = context.Background()

		// Keep the processed jobs count.
		processed atomic.Int32
	)

	err := ForEachJob(ctx, 3, 2, func(ctx context.Context, idx int) error {
		if processed.CAS(0, 1) {
			return errors.New("the first request is failing")
		}

		// Wait 1s and increase the number of processed jobs, unless the context get canceled earlier.
		select {
		case <-time.After(time.Second):
			processed.Add(1)
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	})

	require.EqualError(t, err, "the first request is failing")

	// Since we expect the first error interrupts the workers, we should only see
	// 1 job processed (the one which immediately returned error).
	assert.Equal(t, int32(1), processed.Load())
}

func TestForEachJob_ShouldBreakOnFirstError_ContextCancellationUnhandled(t *testing.T) {
	var (
		ctx = context.Background()

		// Keep the processed jobs count.
		processed atomic.Int32
	)

	// waitGroup to await the start of the first two jobs
	var wg sync.WaitGroup
	wg.Add(2)

	err := ForEachJob(ctx, 3, 2, func(ctx context.Context, idx int) error {
		wg.Done()

		if processed.CAS(0, 1) {
			// wait till two jobs have been started
			wg.Wait()
			return errors.New("the first request is failing")
		}

		// Wait till context is cancelled to add processed jobs.
		<-ctx.Done()
		processed.Add(1)

		return nil
	})

	require.EqualError(t, err, "the first request is failing")

	// Since we expect the first error interrupts the workers, we should only
	// see 2 job processed (the one which immediately returned error and the
	// job with "b").
	assert.Equal(t, int32(2), processed.Load())
}

func TestForEachJob_ShouldReturnImmediatelyOnNoJobsProvided(t *testing.T) {
	var processed atomic.Int32
	require.NoError(t, ForEachJob(context.Background(), 0, 2, func(ctx context.Context, idx int) error {
		processed.Inc()
		return nil
	}))
	require.Zero(t, processed.Load())
}
