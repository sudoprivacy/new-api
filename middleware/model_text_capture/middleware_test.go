// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

type writeDeadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (recorder *writeDeadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	recorder.deadline = deadline
	return nil
}

func TestCaptureWriterPreservesWriteDeadlineAndFlush(t *testing.T) {
	recorder := &writeDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	ctx, _ := gin.CreateTestContext(recorder)
	writer := &captureWriter{ResponseWriter: ctx.Writer, format: types.RelayFormatOpenAI, maxTextBytes: 64, maxPayloadSize: 1024}
	writer.Header().Set("Content-Type", "text/event-stream")
	controller := http.NewResponseController(writer)
	deadline := time.Unix(100, 0)
	require.NoError(t, controller.SetWriteDeadline(deadline))

	event := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"
	written, err := writer.WriteString(event)
	require.NoError(t, err)
	require.Equal(t, len(event), written)
	require.NoError(t, controller.Flush())

	assert.Equal(t, deadline, recorder.deadline)
	assert.True(t, recorder.Flushed)
	assert.Equal(t, event, recorder.Body.String())
	text, err := writer.capturer.result()
	require.NoError(t, err)
	assert.Equal(t, "hello", text)
}

func TestCaptureWriterDiscardsOversizedJSONWithoutChangingResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	writer := &captureWriter{ResponseWriter: ctx.Writer, format: types.RelayFormatOpenAI, maxTextBytes: 64, maxPayloadSize: 1024}
	writer.Header().Set("Content-Type", "application/json")
	prefix := `{"choices":[{"message":{"content":"hello"}}],"ignored":"`
	largeField := strings.Repeat("x", writer.maxPayloadSize-len(prefix))
	for _, part := range []string{prefix, largeField, `"}`} {
		written, err := writer.WriteString(part)
		require.NoError(t, err)
		require.Equal(t, len(part), written)
	}

	text, err := writer.capturer.result()
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Equal(t, prefix+largeField+`"}`, recorder.Body.String())
}

func TestModelTextCaptureKeepsRequestLimitOnConfigUpdate(t *testing.T) {
	previous := captureSettings()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		setting = previous
		common.OptionMapRWMutex.Unlock()
	})
	filename := filepath.Join(t.TempDir(), "records.jsonl")
	common.OptionMapRWMutex.Lock()
	setting = Setting{
		Enabled: true, MaxTextBytes: 4, MaxPayloadSize: 1024,
		File: fileSetting{Enabled: true, Filename: filename, MaxSize: 1},
	}
	common.OptionMapRWMutex.Unlock()

	var captured string
	var dump textDump
	defer func() { assert.NoError(t, dump.shutdown(t.Context())) }()
	router := gin.New()
	router.POST("/", dump.middleware(types.RelayFormatOpenAI), func(ctx *gin.Context) {
		common.OptionMapRWMutex.Lock()
		setting.MaxTextBytes = 8
		common.OptionMapRWMutex.Unlock()

		ctx.Header("Content-Type", "application/json")
		_, err := ctx.Writer.WriteString(`{"choices":[{"message":{"content":"0123456789"}}]}`)
		require.NoError(t, err)
		writer := ctx.Writer.(*captureWriter)
		captured, err = writer.capturer.result()
		require.NoError(t, err)
	})

	for _, want := range []string{"0123", "01234567"} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
		assert.Equal(t, want, captured)
	}
}
