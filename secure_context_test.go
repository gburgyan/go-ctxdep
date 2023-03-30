package ctxdep

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_secureContext(t *testing.T) {
	ctx := context.Background()

	valCtx1 := context.WithValue(ctx, "ctx1", "val1")
	cancelCtx, _ := context.WithTimeout(valCtx1, time.Minute)
	cycleCtx := context.WithValue(cancelCtx, cycleKey, &cycleChecker{})
	valCtx2 := context.WithValue(cycleCtx, "ctx2", "val2")

	secCtx := &secureContext{
		baseContext:  valCtx1,
		innerContext: valCtx2,
	}

	assert.NotNil(t, secCtx.Value("ctx1"))
	assert.Nil(t, secCtx.Value("ctx2"))
	assert.NotNil(t, secCtx.Value(cycleKey))

	secDeadline, ok := secCtx.Deadline()
	assert.True(t, ok)
	assert.Greater(t, secDeadline.Unix(), int64(0))
	assert.NoError(t, secCtx.Err())
	assert.NotNil(t, secCtx.Done())

	assert.NotNil(t, valCtx2.Value(cycleKey))
}
