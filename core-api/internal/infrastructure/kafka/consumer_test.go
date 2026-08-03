package kafka

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoffWithJitter_StaysWithinBounds(t *testing.T) {
	base := 100 * time.Millisecond
	ceiling := 5 * time.Second

	for attempt := 0; attempt < 10; attempt++ {
		wait := backoffWithJitter(attempt, base, ceiling)
		assert.GreaterOrEqual(t, wait, time.Duration(0))
		assert.LessOrEqual(t, wait, ceiling)
	}
}

func TestBackoffWithJitter_LargeAttemptDoesNotOverflow(t *testing.T) {
	wait := backoffWithJitter(1000, time.Second, 30*time.Second)
	assert.GreaterOrEqual(t, wait, time.Duration(0))
	assert.LessOrEqual(t, wait, 30*time.Second)
}
