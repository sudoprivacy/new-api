// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type partialWriteHandle struct {
	rollingFileHandle
	afterClose func()
}

func (handle *partialWriteHandle) Write(data []byte) (int, error) {
	written, err := handle.rollingFileHandle.Write(data[:len(data)/2])
	return written, errors.Join(err, io.ErrShortWrite)
}

func (handle *partialWriteHandle) Close() error {
	err := handle.rollingFileHandle.Close()
	if handle.afterClose != nil {
		handle.afterClose()
	}
	return err
}

func TestRollingFileRollsBackPartialWrite(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "records.jsonl")
	var file rollingFile
	t.Cleanup(func() { assert.NoError(t, file.close()) })

	require.NoError(t, file.write([]byte("{\"request_id\":\"first\"}\n"), filename, 1, 1))
	file.handle = &partialWriteHandle{rollingFileHandle: file.handle}
	require.ErrorIs(t, file.write([]byte("{\"request_id\":\"partial\"}\n"), filename, 1, 1), io.ErrShortWrite)

	require.NoError(t, file.write([]byte("{\"request_id\":\"next\"}\n"), filename, 1, 1))
	assert.Equal(t, []textRecord{{RequestID: "first"}, {RequestID: "next"}}, readDumpRecords(t, filename))
}

func TestRollingFileIsolatesFileAfterFailedRollback(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "records.jsonl")
	displaced := filepath.Join(dir, "displaced")
	var file rollingFile
	t.Cleanup(func() { assert.NoError(t, file.close()) })

	first := []byte("{\"request_id\":\"first\"}\n")
	partial := []byte("{\"request_id\":\"partial\"}\n")
	require.NoError(t, file.write(first, filename, 1, 1))
	file.handle = &partialWriteHandle{
		rollingFileHandle: file.handle,
		afterClose: func() {
			require.NoError(t, os.Rename(filename, displaced))
			require.NoError(t, os.Mkdir(filename, 0700))
		},
	}
	err := file.write(partial, filename, 1, 1)
	require.ErrorIs(t, err, io.ErrShortWrite)
	require.ErrorContains(t, err, "roll back partial write")
	require.NoError(t, os.Remove(filename))
	require.NoError(t, os.Rename(displaced, filename))

	require.NoError(t, file.write([]byte("{\"request_id\":\"next\"}\n"), filename, 1, 1))
	assert.Equal(t, []textRecord{{RequestID: "next"}}, readDumpRecords(t, filename))
	backups, err := filepath.Glob(filename + ".*.bak")
	require.NoError(t, err)
	require.Len(t, backups, 1)
	archived, err := os.ReadFile(backups[0])
	require.NoError(t, err)
	assert.Equal(t, append(first, partial[:len(partial)/2]...), archived)
}
