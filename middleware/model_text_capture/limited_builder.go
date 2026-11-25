package model_text_capture

import (
	"strings"
	"unicode/utf8"
)

func NewLimitedBuilder(maxTextBytes int) LimitedBuilder {
	return LimitedBuilder{builder: strings.Builder{}, remaining: maxTextBytes}
}

type LimitedBuilder struct {
	builder   strings.Builder
	remaining int
}

func (builder *LimitedBuilder) Full() bool {
	return builder.remaining <= 0
}

func (builder *LimitedBuilder) WriteString(text string) {
	if builder.Full() {
		return
	}
	if len(text) > builder.remaining {
		idx := builder.remaining
		for idx > 0 && !utf8.RuneStart(text[idx]) {
			idx--
		}
		builder.remaining = 0
		builder.builder.WriteString(text[:idx])
	} else {
		builder.remaining -= len(text)
		builder.builder.WriteString(text)
	}
}

func (builder *LimitedBuilder) String() string {
	return builder.builder.String()
}
