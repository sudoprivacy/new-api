// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
)

func TestModelTextDumpHotReload(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "capture", "records.jsonl")
	configureFile := modelTextDumpTestConfig(t)
	target := fileSetting{Filename: filename, MaxSize: 1}
	configureFile(target)

	const response = `{"choices":[{"message":{"content":"first\nsecond"}}]}`
	var disableBeforeResponse bool
	sendRequest := func(dump *textDump) {
		router := gin.New()
		router.POST("/", func(ctx *gin.Context) {
			defer common.CleanupBodyStorage(ctx)
			common.SetContextKey(ctx, constant.ContextKeyUserId, 42)
			common.SetContextKey(ctx, constant.ContextKeyTokenId, 73)
			ctx.Next()
		}, dump.middleware(types.RelayFormatOpenAI), func(ctx *gin.Context) {
			ctx.Set(common.RequestIdKey, "test-request")
			_, err := common.GetBodyStorage(ctx)
			require.NoError(t, err)
			if disableBeforeResponse {
				target.Enabled = false
				configureFile(target)
			}
			ctx.Data(http.StatusOK, "application/json", []byte(response))
		})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, response, recorder.Body.String())
	}

	var firstDump textDump
	sendRequest(&firstDump)
	assert.NoFileExists(t, filename)

	target.Enabled = true
	configureFile(target)
	sendRequest(&firstDump)
	require.NoError(t, firstDump.shutdown(t.Context()))
	want := textRecord{UserID: 42, KeyID: 73, RequestID: "test-request", InputText: "user: hello\n", OutputText: "first\nsecond"}
	assert.Equal(t, []textRecord{want}, readDumpRecords(t, filename))

	nextFilename := filepath.Join(filepath.Dir(filename), "next.jsonl")
	require.NoError(t, os.WriteFile(nextFilename, []byte("{\"request_id\":\"existing\"}\n"), 0600))
	target.Filename = nextFilename
	configureFile(target)
	var nextDump textDump
	sendRequest(&nextDump)
	require.NoError(t, nextDump.shutdown(t.Context()))
	assert.Equal(t, []textRecord{want}, readDumpRecords(t, filename))
	wantNext := []textRecord{{RequestID: "existing"}, want}
	assert.Equal(t, wantNext, readDumpRecords(t, nextFilename))

	disableBeforeResponse = true
	var disabledDump textDump
	sendRequest(&disabledDump)
	require.NoError(t, disabledDump.shutdown(t.Context()))
	assert.Equal(t, wantNext, readDumpRecords(t, nextFilename))
}

func TestModelTextDumpDoesNotReadUncachedRequestBody(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "records.jsonl")
	configureFile := modelTextDumpTestConfig(t)
	configureFile(fileSetting{Enabled: true, Filename: filename, MaxSize: 1})

	var dump textDump
	body := &trackedRequestBody{reader: strings.NewReader(`{"messages":[{"role":"user","content":"not cached"}]}`)}
	request := httptest.NewRequest(http.MethodPost, "/", body)
	router := gin.New()
	router.POST("/", dump.middleware(types.RelayFormatOpenAI), func(ctx *gin.Context) {
		ctx.Data(http.StatusOK, "application/json", []byte(`{"choices":[{"message":{"content":"answer"}}]}`))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.NoError(t, dump.shutdown(t.Context()))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.False(t, body.read)
	assert.False(t, body.closed)
	assert.Equal(t, []textRecord{{OutputText: "answer"}}, readDumpRecords(t, filename))
}

func TestModelTextDumpSkipsOversizedRequestBody(t *testing.T) {
	const input = `{"messages":[{"role":"user","content":"hello"}]}`
	filename := filepath.Join(t.TempDir(), "records.jsonl")
	modelTextDumpTestConfig(t)(fileSetting{Enabled: true, Filename: filename, MaxSize: 1})
	common.OptionMapRWMutex.Lock()
	setting.MaxPayloadSize = len(input)
	common.OptionMapRWMutex.Unlock()

	var dump textDump
	router := gin.New()
	router.POST("/", func(ctx *gin.Context) {
		defer common.CleanupBodyStorage(ctx)
		_, err := common.GetBodyStorage(ctx)
		require.NoError(t, err)
		ctx.Next()
	}, dump.middleware(types.RelayFormatOpenAI), func(ctx *gin.Context) {
		ctx.Data(http.StatusOK, "application/json", []byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	for _, body := range []string{input, input + " "} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusOK, response.Code)
	}
	require.NoError(t, dump.shutdown(t.Context()))
	assert.Equal(t, []textRecord{
		{InputText: "user: hello\n", OutputText: "ok"},
		{OutputText: "ok"},
	}, readDumpRecords(t, filename))
}

func TestModelTextDumpBoundsPendingWritesAndDrainsOnShutdown(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "records.jsonl")
	modelTextDumpTestConfig(t)(fileSetting{Enabled: true, Filename: filename, MaxSize: 100})
	file := &blockedDumpFile{filename: filename, started: make(chan struct{}), release: make(chan struct{})}
	dump := textDump{sink: textFileSink{file: rollingFile{handle: file}}}
	release := sync.OnceFunc(func() { close(file.release) })
	defer func() {
		release()
		assert.NoError(t, dump.shutdown(t.Context()))
	}()

	dump.enqueue(textRecord{RequestID: "in flight"})
	<-file.started
	largeRecord := textRecord{OutputText: strings.Repeat("x", 1<<20)}
	for range (maxPendingTextBytes - len("in flight")) / len(largeRecord.OutputText) {
		dump.enqueue(largeRecord)
	}
	dump.enqueue(largeRecord)
	assert.EqualValues(t, 1, dump.dropped.Load(), "byte budget")
	for range maxPendingRecords - len(dump.records) {
		dump.enqueue(textRecord{RequestID: "pending"})
	}
	dump.enqueue(textRecord{RequestID: "queue full"})
	assert.EqualValues(t, 2, dump.dropped.Load(), "record budget")

	release()
	require.NoError(t, dump.shutdown(t.Context()))
	assert.Equal(t, maxPendingRecords+1, file.writes)
	assert.True(t, file.closed)
}

type blockedDumpFile struct {
	filename string
	started  chan struct{}
	release  chan struct{}
	writes   int
	closed   bool
}

func (file *blockedDumpFile) Name() string {
	return file.filename
}

func (file *blockedDumpFile) Write(data []byte) (int, error) {
	if file.writes == 0 {
		close(file.started)
		<-file.release
	}
	file.writes++
	return len(data), nil
}

func (file *blockedDumpFile) Close() error {
	file.closed = true
	return nil
}

type trackedRequestBody struct {
	reader *strings.Reader
	read   bool
	closed bool
}

func (body *trackedRequestBody) Read(data []byte) (int, error) {
	body.read = true
	return body.reader.Read(data)
}

func (body *trackedRequestBody) Close() error {
	body.closed = true
	return nil
}

func TestModelTextDumpHotReloadRotation(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "records.jsonl")
	configureFile := modelTextDumpTestConfig(t)
	target := fileSetting{Enabled: true, Filename: filename, MaxSize: 2, MaxBackups: 2}
	configureFile(target)
	var sink textFileSink
	t.Cleanup(func() { assert.NoError(t, sink.file.close()) })

	record := textRecord{RequestID: "first", OutputText: strings.Repeat("x", 600<<10)}
	require.NoError(t, sink.writeRecord(record))
	record.RequestID = "second"
	require.NoError(t, sink.writeRecord(record))
	backups, err := filepath.Glob(filename + ".*.bak")
	require.NoError(t, err)
	require.Empty(t, backups)
	original, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(original), "\n"))

	target.MaxSize = 1
	configureFile(target)
	record.RequestID = "third"
	require.NoError(t, sink.writeRecord(record))
	backups, err = filepath.Glob(filename + ".*.bak")
	require.NoError(t, err)
	require.Len(t, backups, 1)
	archived, err := os.ReadFile(backups[0])
	require.NoError(t, err)
	assert.True(t, bytes.Equal(archived, original), "rolled file must contain the previous JSONL records")

	record.RequestID = "fourth"
	require.NoError(t, sink.writeRecord(record))
	backups, err = filepath.Glob(filename + ".*.bak")
	require.NoError(t, err)
	require.Len(t, backups, 2)
	oldest, newest := backups[0], backups[1]

	target.MaxSize = 2
	target.MaxBackups = 1
	configureFile(target)
	record.RequestID = "fifth"
	require.NoError(t, sink.writeRecord(record))
	assert.NoFileExists(t, oldest)
	assert.FileExists(t, newest)
	active, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, 2, bytes.Count(active, []byte("\n")))
	assert.True(t, bytes.Contains(active, []byte(`"request_id":"fourth"`)))
	assert.True(t, bytes.Contains(active, []byte(`"request_id":"fifth"`)))
}

func readDumpRecords(t *testing.T, filename string) []textRecord {
	t.Helper()
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	data, terminated := bytes.CutSuffix(data, []byte{'\n'})
	require.True(t, terminated, "JSONL must end with a newline")
	var records []textRecord
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var record textRecord
		require.NoError(t, common.Unmarshal(line, &record))
		records = append(records, record)
	}
	return records
}

func modelTextDumpTestConfig(t *testing.T) func(fileSetting) {
	t.Helper()
	previous := captureSettings()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		setting = previous
		common.OptionMapRWMutex.Unlock()
	})
	return func(target fileSetting) {
		t.Helper()
		encoded, err := common.Marshal(target)
		require.NoError(t, err)
		common.OptionMapRWMutex.Lock()
		setting.Enabled = true
		setting.MaxTextBytes = 64 << 10
		setting.MaxPayloadSize = 4 << 20
		err = config.UpdateConfigFromMap(&setting, map[string]string{"file": string(encoded)})
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, err)
	}
}
