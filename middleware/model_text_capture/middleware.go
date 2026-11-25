// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func Middleware(format types.RelayFormat) gin.HandlerFunc {
	return modelTextDump.middleware(format)
}

func (dump *textDump) middleware(format types.RelayFormat) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings := captureSettings()
		if !settings.fileCaptureEnabled() {
			dump.notifyInactive()
			c.Next()
			return
		}
		if format == types.RelayFormatGemini {
			_, action, _ := strings.Cut(c.Param("path"), ":")
			if action != "generateContent" && action != "streamGenerateContent" {
				c.Next()
				return
			}
		}

		writer := &captureWriter{
			maxTextBytes:   settings.MaxTextBytes,
			maxPayloadSize: settings.MaxPayloadSize,
			ResponseWriter: c.Writer,
			format:         format,
		}
		c.Writer = writer
		defer func() { c.Writer = writer.ResponseWriter }()
		c.Next()
		if !captureSettings().fileCaptureEnabled() {
			dump.notifyInactive()
			return
		}

		var output string
		if writer.capturer != nil {
			output, _ = writer.capturer.result()
		}
		record := textRecord{
			UserID:     common.GetContextKeyInt(c, constant.ContextKeyUserId),
			KeyID:      common.GetContextKeyInt(c, constant.ContextKeyTokenId),
			RequestID:  c.GetString(common.RequestIdKey),
			OutputText: output,
		}

		cached, _ := c.Get(common.KeyBodyStorage)
		if storage, ok := cached.(common.BodyStorage); ok && storage.Size() <= int64(settings.MaxPayloadSize) {
			record.InputText, _ = modelTextInput(storage, format, settings.MaxTextBytes)
		}
		dump.enqueue(record)
	}
}

type captureWriter struct {
	format         types.RelayFormat
	maxTextBytes   int
	maxPayloadSize int

	gin.ResponseWriter
	capturer textCapturer
}

func (writer *captureWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (writer *captureWriter) capturerForWrite(written int) textCapturer {
	if written <= 0 || writer.Status() >= http.StatusBadRequest {
		return nil
	}

	if writer.capturer == nil {
		mediaType, _, _ := strings.Cut(writer.Header().Get("Content-Type"), ";")
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
		if mediaType == "text/event-stream" {
			writer.capturer = newStreamCapturer(writer.format, writer.maxTextBytes, writer.maxPayloadSize)
		} else {
			writer.capturer = newNonStreamCapturer(writer.format, writer.maxTextBytes, writer.maxPayloadSize)
		}
	}
	return writer.capturer
}

func (writer *captureWriter) Write(data []byte) (int, error) {
	written, err := writer.ResponseWriter.Write(data)
	if capturer := writer.capturerForWrite(written); capturer != nil {
		capturer.capture(data[:written])
	}
	return written, err
}

func (writer *captureWriter) WriteString(data string) (int, error) {
	written, err := writer.ResponseWriter.WriteString(data)
	if capturer := writer.capturerForWrite(written); capturer != nil {
		capturer.captureString(data[:written])
	}
	return written, err
}
