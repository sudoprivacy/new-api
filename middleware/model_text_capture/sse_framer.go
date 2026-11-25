// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"bytes"
)

type sseFrameState uint8

const (
	sseReading sseFrameState = iota
	sseDiscardingLine
	sseDiscardingEvent
)

type sseFramer struct {
	maxPayloadSize int

	line          []byte
	event         []byte
	state         sseFrameState
	seenFirstLine bool
	skipLF        bool
}

func (framer *sseFramer) nextEvent(data []byte) (event, rest []byte) {
	for len(data) > 0 {
		if framer.skipLF {
			framer.skipLF = false
			if data[0] == '\n' {
				data = data[1:]
				continue
			}
		}
		lineEnd := bytes.IndexAny(data, "\r\n")
		if lineEnd < 0 {
			framer.acceptLinePart(data, false)
			return nil, nil
		}
		framer.skipLF = data[lineEnd] == '\r'
		if event := framer.acceptLinePart(data[:lineEnd], true); len(event) > 0 {
			return event, data[lineEnd+1:]
		}
		data = data[lineEnd+1:]
	}
	return nil, nil
}

func (framer *sseFramer) acceptLinePart(part []byte, complete bool) []byte {
	switch framer.state {
	case sseDiscardingLine:
		if complete {
			framer.state = sseDiscardingEvent
		}
		return nil
	case sseDiscardingEvent:
		if !complete {
			framer.state = sseDiscardingLine
		} else if len(part) == 0 {
			framer.state = sseReading
		}
		return nil
	}

	if len(part) > framer.maxPayloadSize-len(framer.line) {
		framer.line = nil
		framer.event = nil
		framer.state = sseDiscardingEvent
		if !complete {
			framer.state = sseDiscardingLine
		}
		return nil
	}
	if !complete {
		framer.line = append(framer.line, part...)
		return nil
	}
	if len(framer.line) > 0 {
		part = append(framer.line, part...)
		framer.line = nil
	}
	return framer.finishLine(part)
}

func (framer *sseFramer) finishLine(line []byte) []byte {
	if !framer.seenFirstLine {
		framer.seenFirstLine = true
		line = bytes.TrimPrefix(line, []byte("\uFEFF"))
	}
	if len(line) == 0 {
		event := framer.event
		framer.event = nil
		return event
	}
	field, value, _ := bytes.Cut(line, []byte{':'})
	if !bytes.Equal(field, []byte("data")) {
		return nil
	}
	value = bytes.TrimPrefix(value, []byte{' '})
	if len(value) >= framer.maxPayloadSize-len(framer.event) {
		framer.event = nil
		framer.state = sseDiscardingEvent
		return nil
	}
	framer.event = append(framer.event, value...)
	framer.event = append(framer.event, '\n')
	return nil
}
