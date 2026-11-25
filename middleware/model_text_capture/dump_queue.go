// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	maxPendingRecords   = 64
	maxPendingTextBytes = 32 << 20
)

var modelTextDump textDump

type textDump struct {
	mu           sync.Mutex // protects queue admission and shutdown, never file I/O
	records      chan textRecord
	inactive     chan struct{}
	done         chan struct{}
	closed       bool
	pendingBytes int
	dropped      atomic.Uint64
	sink         textFileSink
}

func (dump *textDump) enqueue(record textRecord) {
	size := record.textBytes()
	dump.mu.Lock()
	defer dump.mu.Unlock()
	if dump.closed {
		return
	}
	if dump.records == nil {
		dump.records = make(chan textRecord, maxPendingRecords)
		dump.inactive = make(chan struct{}, 1)
		dump.done = make(chan struct{})
		go dump.run()
	}
	if size > maxPendingTextBytes-dump.pendingBytes {
		dump.dropped.Add(1)
		return
	}
	select {
	case dump.records <- record:
		dump.pendingBytes += size
	default:
		dump.dropped.Add(1)
	}
}

func (dump *textDump) notifyInactive() {
	dump.mu.Lock()
	defer dump.mu.Unlock()
	select {
	case dump.inactive <- struct{}{}:
	default:
	}
}

func (dump *textDump) run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	defer close(dump.done)
	defer dump.reportDrops()
	defer func() { logDumpError(dump.sink.file.close()) }()
	for {
		select {
		case record, ok := <-dump.records:
			if !ok {
				return
			}
			logDumpError(dump.sink.writeRecord(record))
			dump.mu.Lock()
			dump.pendingBytes -= record.textBytes()
			dump.mu.Unlock()
		case <-dump.inactive:
			logDumpError(dump.sink.closeIfInactive())
		case <-ticker.C:
			dump.reportDrops()
		}
	}
}

func logDumpError(err error) {
	if err != nil {
		common.SysError("model text capture: " + err.Error())
	}
}

func (dump *textDump) reportDrops() {
	if count := dump.dropped.Swap(0); count > 0 {
		common.SysError(fmt.Sprintf("model text capture: dropped %d records because the file queue reached its limits", count))
	}
}

// Shutdown drains queued capture records within ctx's deadline.
func Shutdown(ctx context.Context) error {
	return modelTextDump.shutdown(ctx)
}

func (dump *textDump) shutdown(ctx context.Context) error {
	dump.mu.Lock()
	if !dump.closed {
		dump.closed = true
		if dump.records != nil {
			close(dump.records)
		}
	}
	done := dump.done
	dump.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
