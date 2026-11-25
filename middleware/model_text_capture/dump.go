// sudoapi: Model request and response text capture.

package model_text_capture

import "github.com/QuantumNous/new-api/common"

type textRecord struct {
	UserID     int    `json:"user_id"`
	KeyID      int    `json:"key_id"`
	RequestID  string `json:"request_id"`
	InputText  string `json:"input_text"`
	OutputText string `json:"output_text"`
}

func (record textRecord) textBytes() int {
	return len(record.RequestID) + len(record.InputText) + len(record.OutputText)
}

type textFileSink struct {
	file rollingFile
}

func (sink *textFileSink) writeRecord(record textRecord) error {
	if !captureSettings().fileCaptureEnabled() {
		return sink.file.close()
	}
	data, err := common.Marshal(record)
	if err != nil {
		return err
	}
	line := append(data, '\n')
	settings := captureSettings()
	if !settings.fileCaptureEnabled() {
		return sink.file.close()
	}
	target := settings.File
	return sink.file.write(line, target.Filename, target.MaxSize, target.MaxBackups)
}

func (sink *textFileSink) closeIfInactive() error {
	if sink.file.handle == nil || captureSettings().fileCaptureEnabled() {
		return nil
	}
	return sink.file.close()
}
