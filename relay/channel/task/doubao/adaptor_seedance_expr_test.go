package doubao

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

func TestRunExpr(t *testing.T) {
	const (
		expression = `
let key = matched_tier != ""
	? matched_tier
	: "resolution " + req.Resolution + ", content " + (any(req.Content, .Type == "video_url" && .VideoURL != nil && .VideoURL.URL != "") ? "with video" : "without video");

if key == "resolution 480p, content with video" {
	tier("resolution 480p, content with video", c * 3.83561643835616)
} else if key == "resolution 480p, content without video" {
	tier("resolution 480p, content without video", c * 6.3013698630137)
} else if key == "resolution 720p, content with video" {
	tier("resolution 720p, content with video", c * 3.83561643835616)
} else if key == "resolution 720p, content without video" {
	tier("resolution 720p, content without video", c * 6.3013698630137)
} else if key == "resolution 1080p, content with video" {
	tier("resolution 1080p, content with video", c * 4.24657534246575)
} else if key == "resolution 1080p, content without video" {
	tier("resolution 1080p, content without video", c * 6.98630136986301)
} else if key == "resolution 4k, content with video" {
	tier("resolution 4k, content with video", c * 2.19178082191781)
} else if key == "resolution 4k, content without video" {
	tier("resolution 4k, content without video", c * 3.56164383561644)
} else {
	0
}`
		expressionFast = `
let key = matched_tier != ""
	? matched_tier
	: "resolution 480p/720p, content " + (any(req.Content, .Type == "video_url" && .VideoURL != nil && .VideoURL.URL != "") ? "with video" : "without video");

if key == "resolution 480p/720p, content with video" {
	tier("resolution 480p/720p, content with video", c * 3.01369863013699)
} else if key == "resolution 480p/720p, content without video" {
	tier("resolution 480p/720p, content without video", c * 5.06849315068493)
} else {
	0
}`
		expressionMini = `
let key = matched_tier != ""
	? matched_tier
	: "resolution 480p/720p, content " + (any(req.Content, .Type == "video_url" && .VideoURL != nil && .VideoURL.URL != "") ? "with video" : "without video");

if key == "resolution 480p/720p, content with video" {
	tier("resolution 480p/720p, content with video", c * 1.91780821917808)
} else if key == "resolution 480p/720p, content without video" {
	tier("resolution 480p/720p, content without video", c * 3.15068493150685)
} else {
	0
}`
	)

	testcases := []struct {
		name        string
		expression  string
		request     requestPayload
		matchedTier string
		expected    billingexpr.TraceResult
	}{
		{
			name:       "480p without video on request",
			expression: expression,
			request: requestPayload{
				Content:    []ContentItem{{Type: "text", Text: "foo bar"}},
				Resolution: "480p",
			},
			expected: billingexpr.TraceResult{
				MatchedTier: "resolution 480p, content without video",
				Cost:        6.3013698630137,
			},
		}, {
			name:       "4k with video on request",
			expression: expression,
			request: requestPayload{
				Content: []ContentItem{
					{Type: "text", Text: "foo bar"},
					{Type: "video_url", VideoURL: &MediaURL{URL: "foo bar"}},
				},
				Resolution: "4k",
			},
			expected: billingexpr.TraceResult{
				MatchedTier: "resolution 4k, content with video",
				Cost:        2.19178082191781,
			},
		}, {
			name:        "720p with video on complete",
			expression:  expression,
			matchedTier: "resolution 720p, content with video",
			expected: billingexpr.TraceResult{
				MatchedTier: "resolution 720p, content with video",
				Cost:        3.83561643835616,
			},
		}, {
			name:       "invalid resolution",
			expression: expression,
			request: requestPayload{
				Content:    []ContentItem{{Type: "text", Text: "foo bar"}},
				Resolution: "660p",
			},
			matchedTier: "",
			expected:    billingexpr.TraceResult{MatchedTier: "", Cost: 0},
		}, {
			name:       "invalid video content",
			expression: expression,
			request: requestPayload{
				Content: []ContentItem{
					{Type: "text", Text: "foo bar"},
					{Type: "video_url"},
				},
				Resolution: "1080p",
			},
			matchedTier: "",
			expected: billingexpr.TraceResult{
				MatchedTier: "resolution 1080p, content without video",
				Cost:        6.98630136986301,
			},
		}, {
			name:       "fast: with video on request",
			expression: expressionFast,
			request: requestPayload{
				Content:    []ContentItem{{Type: "text", Text: "foo bar"}},
				Resolution: "480p",
			},
			expected: billingexpr.TraceResult{
				MatchedTier: "resolution 480p/720p, content without video",
				Cost:        5.06849315068493,
			},
		}, {
			name:        "mini: with video on complete",
			expression:  expressionMini,
			matchedTier: "resolution 480p/720p, content with video",
			expected: billingexpr.TraceResult{
				MatchedTier: "resolution 480p/720p, content with video",
				Cost:        1.91780821917808,
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			completionTokens := 100_0000
			result, err := runExpr(testcase.expression, testcase.request, testcase.matchedTier, completionTokens)
			assert.NoError(t, err)
			assert.Equal(t, testcase.expected.MatchedTier, result.MatchedTier)
			assert.InDelta(t, testcase.expected.Cost, result.Cost/float64(completionTokens), 0.0001)
		})
	}
}
