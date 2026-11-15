// sudoapi: Complete failed-request refunds before HTTP drain returns.

package service

import (
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type blockingRefundFunding struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (f *blockingRefundFunding) Source() string       { return BillingSourceWallet }
func (f *blockingRefundFunding) PreConsume(int) error { return nil }
func (f *blockingRefundFunding) Settle(int) error     { return nil }
func (f *blockingRefundFunding) Refund() error {
	f.calls.Add(1)
	close(f.entered)
	<-f.release
	return nil
}

func TestBillingRefundCompletesBeforeHandlerReturnsAndDoesNotRepeat(t *testing.T) {
	funding := &blockingRefundFunding{entered: make(chan struct{}), release: make(chan struct{})}
	session := &BillingSession{funding: funding, tokenConsumed: 10,
		relayInfo: &relaycommon.RelayInfo{IsPlayground: true}}
	done := make(chan struct{})
	go func() { session.Refund(&gin.Context{}); close(done) }()
	select {
	case <-funding.entered:
	case <-time.After(time.Second):
		t.Fatal("refund did not reach funding source")
	}
	select {
	case <-done:
		t.Fatal("handler returned before its refund completed")
	default:
	}
	close(funding.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completed refund did not release handler")
	}
	session.Refund(&gin.Context{})
	require.EqualValues(t, 1, funding.calls.Load())
}

func TestBillingRefundNeverCreditsAlreadySettledFunding(t *testing.T) {
	funding := &blockingRefundFunding{entered: make(chan struct{}), release: make(chan struct{})}
	session := &BillingSession{funding: funding, fundingSettled: true, tokenConsumed: 10,
		relayInfo: &relaycommon.RelayInfo{IsPlayground: true}}
	session.Refund(&gin.Context{})
	require.Zero(t, funding.calls.Load())
}
