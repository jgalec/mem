package runtime

import (
	"time"
)

type WriteOp struct {
	Query string
	Args  []interface{}
	Done  chan error
}

type writeBatchResult struct {
	Ops []WriteOp
	Err error
}

func (r *Runtime) WriteQueue() chan<- WriteOp {
	return r.writeQueue
}

func (r *Runtime) SetOnFlush(fn func([]WriteOp) error) {
	r.onFlush = fn
}

func (r *Runtime) EnqueueWrite(query string, args ...interface{}) <-chan error {
	done := make(chan error, 1)
	select {
	case r.writeQueue <- WriteOp{Query: query, Args: args, Done: done}:
	default:
		go func() { done <- nil }()
	}
	return done
}

func (r *Runtime) flushLoop() {
	defer close(r.flushDone)
	ticker := time.NewTicker(r.config.FlushInterval)
	defer ticker.Stop()

	var batch []WriteOp

	for {
		select {
		case op, ok := <-r.writeQueue:
			if !ok {
				r.executeBatch(batch)
				return
			}
			batch = append(batch, op)
			if len(batch) >= r.config.WriteQueueSize {
				r.executeBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				r.executeBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (r *Runtime) executeBatch(batch []WriteOp) {
	if len(batch) == 0 {
		return
	}
	var err error
	if r.onFlush != nil {
		err = r.onFlush(batch)
	}
	for _, op := range batch {
		if op.Done != nil {
			op.Done <- err
		}
	}
}
