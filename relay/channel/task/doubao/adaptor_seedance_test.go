package doubao

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/QuantumNous/new-api/model"
)

func TestRunY(t *testing.T) {
	respBody := []byte(`{
  "id" : "sd_1784600944452_1553",
  "model" : "doubao-seedance-2.0",
  "status" : "succeeded",
  "error" : null,
  "created_at" : 1784600944,
  "updated_at" : 1784601275,
  "content" : {
    "video_url" : "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com"
  },
  "seed" : 1013,
  "resolution" : "480p",
  "ratio" : "16:9",
  "duration" : 5,
  "framespersecond" : 24,
  "generate_audio" : true,
  "tools" : { },
  "safety_identifier" : "",
  "draft" : false,
  "draft_task_id" : "",
  "execution_expires_after" : 3600,
  "usage" : {
    "total_tokens" : 50638,
    "completion_tokens" : 50638
  }
}`)
	adaptor := SeedanceTaskAdaptor{}
	info, err := adaptor.ParseTaskResult(respBody)
	if err != nil {
		return
	}

	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com", info.Url)
}
