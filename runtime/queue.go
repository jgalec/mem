package runtime

import (
	"fmt"
	"log"
	"time"
)

type WriteOp struct {
	Query string
	Args  []interface{}
	Done  chan error
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
		err := fmt.Errorf("write queue full (%d ops pending)", len(r.writeQueue))
		log.Printf("%v — dropping write: %s", err, query)
		go func() { done <- err }()
	}
	return done
}

func (r *Runtime) DrainWriteQueue() {
	done := make(chan error, 1)
	select {
	case r.writeQueue <- WriteOp{Done: done}:
		<-done
	default:
	}
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
	var realOps []WriteOp
	var drainSignals []chan error
	for _, op := range batch {
		if op.Query == "" && len(op.Args) == 0 {
			drainSignals = append(drainSignals, op.Done)
		} else {
			realOps = append(realOps, op)
		}
	}

	if len(realOps) > 0 {
		var err error
		if r.onFlush == nil {
			err = fmt.Errorf("write queue: onFlush callback not set — %d ops dropped", len(realOps))
			log.Print(err)
		} else {
			backoffs := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
			for attempt := 0; attempt <= len(backoffs); attempt++ {
				err = r.onFlush(realOps)
				if err == nil {
					break
				}
				if attempt < len(backoffs) {
					log.Printf("write flush attempt %d failed: %v — retrying in %v",
						attempt+1, err, backoffs[attempt])
					time.Sleep(backoffs[attempt])
				}
			}
			if err != nil {
				log.Printf("write flush failed after all retries: %v — %d ops dropped", err, len(realOps))
			}
		}
		for _, op := range realOps {
			if op.Done != nil {
				op.Done <- err
			}
		}
	}

	for _, ch := range drainSignals {
		if ch != nil {
			ch <- nil
		}
	}
}
