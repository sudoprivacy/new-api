// src/volcengine/request.ts
var models = ["doubao-seedance-2-0", "doubao-seedance-2-0-fast", "doubao-seedance-2-0-mini"];
var resolutions = ["480p", "720p", "1080p", "4k"];
var modelResolutions = {
  "doubao-seedance-2-0": ["480p", "720p", "1080p", "4k"],
  "doubao-seedance-2-0-fast": ["480p", "720p"],
  "doubao-seedance-2-0-mini": ["480p", "720p"]
};
function getModel(ctx) {
  const model = ctx.model;
  if (!models.includes(model)) {
    throw new Error("unsupported model");
  }
  return model;
}
function getDuration(body) {
  let duration = 0;
  if (body.frames !== void 0) {
    if (!Number.isInteger(body.frames) || body.frames < 1 || body.frames > 86400) {
      throw new Error("frames must be an integer between 1 and 86400");
    }
    duration = Math.ceil(body.frames / 24);
  }
  if (body.duration !== void 0) {
    if (!Number.isInteger(body.duration) || body.duration < 1 || body.duration > 3600) {
      throw new Error("duration must be an integer between 1 and 3600");
    }
    duration = Math.max(duration, body.duration);
  }
  return duration || 15;
}
function getResolution(model, body) {
  if (body.resolution === void 0) {
    return "720p";
  }
  const allowedResolutions = modelResolutions[model];
  const resolution = body.resolution.trim();
  if (!allowedResolutions.includes(resolution)) {
    throw new Error("resolution must be one of " + allowedResolutions.join(", "));
  }
  return resolution;
}
function getRequestUrl(url) {
  if (url.endsWith("/contents/generations/tasks")) {
    return url;
  }
  if (url.endsWith("/v1/video/tasks")) {
    return url;
  }
  return url + "/api/v3/contents/generations/tasks";
}
function mapContent(content, originTasks) {
  return content.map((item) => {
    if (item?.type !== "draft_task" || !item?.draft_task?.id) {
      return item;
    }
    const id = text(item?.draft_task?.id);
    const origin = (originTasks || []).find(function(task) {
      return task.taskId === id;
    });
    if (!origin?.upstreamTaskId) {
      throw new Error("origin task is unavailable");
    }
    return {
      type: "draft_task",
      draft_task: { id: origin.upstreamTaskId }
    };
  });
}
function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

// src/volcengine/native.ts
function createTaskDecode(ctx) {
  if (ctx.body.kind !== "json") {
    throw new Error("json body required");
  }
  const body = ctx.body.value;
  const model = getModel(body);
  getDuration(body);
  getResolution(model, body);
  const draftTaskIDs = body.content.filter((item) => item?.type === "draft_task" && item?.draft_task?.id).map((item) => String(item?.draft_task?.id));
  return {
    action: "textGenerate",
    kind: "submit",
    model,
    originTaskIds: draftTaskIDs,
    requestBody: body
  };
}
function createTaskRender(_ctx, task) {
  if (Array.isArray(task)) {
    throw new Error("task should not be an array");
  }
  const taskView = task;
  const data = taskView.data;
  return { ...data, id: taskView.task_id };
}
function queryTaskRender(_ctx, task) {
  if (Array.isArray(task)) {
    throw new Error("task should not be an array");
  }
  const taskView = task;
  const data = taskView.data;
  return { ...data, id: taskView.task_id };
}

// src/volcengine/usage.ts
var videoInputs = ["with video", "without video"];
function usageSchema(resolutions2) {
  return {
    tokens: {
      type: "number",
      unit: "token",
      description: { en: "Video generation token unit price", zh: "视频生成 Token 单价" }
    },
    resolution: {
      enum: resolutions2,
      description: { en: "Output video resolution", zh: "输出视频分辨率" }
    },
    video_input: {
      enum: videoInputs,
      enumLabels: {
        "with video": { en: "With reference video input", zh: "包含参考视频输入" },
        "without video": { en: "Without reference video input", zh: "不包含参考视频输入" }
      },
      description: { en: "Reference video input", zh: "参考视频输入" }
    }
  };
}
function usageExamples(resolutions2) {
  const duration = 5;
  return resolutions2.map((resolution) => {
    return {
      label: resolution + " * 5s",
      facts: {
        tokens: estimatedTokens(duration, resolution),
        resolution,
        video_input: "without video"
      }
    };
  });
}
function estimatedTokens(seconds, resolution) {
  const pixels = (() => {
    switch (resolution) {
      case "480p":
        return 480 * 854;
      case "720p":
        return 720 * 1280;
      case "1080p":
        return 1080 * 1920;
      case "4k":
        return 3840 * 2160;
      default:
        return 3840 * 2160;
    }
  })();
  return Math.round(seconds * pixels * 24 / 1024);
}

// src/volcengine/driver.ts
function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") {
    return null;
  }
  const model = getModel(ctx);
  const body = ctx.requestBody || {};
  const duration = getDuration(body);
  const resolution = getResolution(model, body);
  return {
    tokens: estimatedTokens(duration, resolution),
    resolution,
    video_input: body.content.some((c) => c && c.type === "video_url") ? "with video" : "without video"
  };
}
function extractUsageOnComplete(_task, result, body) {
  if (result.status !== "SUCCESS") {
    return {};
  }
  const tokens = int(body.usage?.completion_tokens) || int(body.usage?.total_tokens);
  return {
    ...tokens === void 0 ? {} : { tokens },
    ...body.resolution === void 0 ? {} : { resolution: body.resolution }
  };
}
function buildSubmitRequest(ctx) {
  const model = getModel(ctx);
  const body = ctx.requestBody || {};
  getDuration(body);
  getResolution(model, body);
  return {
    url: getRequestUrl(ctx.baseUrl),
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      Authorization: "Bearer " + ctx.apiKey
    },
    body: {
      ...body,
      model: ctx.upstreamModel || model,
      content: mapContent(body.content, ctx.originTasks)
    },
    action: ctx.action,
    rewriteModel: model
  };
}
function parseSubmitResponse(_ctx, response) {
  const body = response.body;
  const id = body.id || body.task_id;
  if (!id) {
    throw new Error("task id not found");
  }
  return { taskId: id, taskData: response.body };
}
function buildQueryRequest(ctx) {
  if (!ctx.taskId) {
    throw new Error("task id not found");
  }
  return {
    url: getRequestUrl(ctx.baseUrl) + "/" + ctx.taskId,
    method: "GET",
    headers: { Accept: "application/json", Authorization: "Bearer " + ctx.apiKey }
  };
}
function parseTaskResult(_ctx, body, _response) {
  switch (body.status) {
    case "pending":
    case "queued":
      return { status: "QUEUED", progress: "10%" };
    case "processing":
    case "running":
      return { status: "IN_PROGRESS", progress: "50%" };
    case "failed":
    case "expired":
    case "cancelled":
      return { status: "FAILURE", progress: "100%" };
    case "succeeded":
      return {
        status: "SUCCESS",
        progress: "100%",
        completionTokens: int(body.usage?.completion_tokens),
        totalTokens: int(body.usage?.total_tokens),
        url: body.content?.video_url
      };
    default:
      return { status: "UNKNOWN", reason: "unrecognized status: " + body.status };
  }
}
function listArtifacts(task) {
  const content = task.data.content;
  const artifacts = [];
  if (!content) {
    return artifacts;
  }
  if (content.video_url) {
    artifacts.push({ key: "video", type: "video" });
  }
  if (content.last_frame_url) {
    artifacts.push({ key: "last_frame", type: "image" });
  }
  return artifacts;
}
function buildContentRequest(ctx) {
  const content = ctx.data.content;
  if (!content) {
    throw new Error("content not found");
  }
  const url = { video: content?.video_url, last_frame: content?.last_frame_url }[ctx.artifactKey];
  if (!url) {
    throw new Error("artifact not found");
  }
  return { url, credentialless: true };
}

// src/volcengine/index.ts
var meta = {
  apiVersion: 1,
  key: "seedance",
  name: "Seedance",
  icon: "Doubao.Color",
  description: { en: "Volcengine Seedance video generation", zh: "火山引擎 Seedance 视频生成" },
  version: "1.0.0",
  author: { name: "black-06" },
  upstreams: ["vendor", "new_api"],
  fetchMode: "per_task",
  models,
  routes: [
    {
      method: "POST",
      path: "/volcengine/api/v3/contents/generations/tasks",
      type: "submit",
      decode: "createTaskDecode",
      render: "createTaskRender"
    },
    {
      method: "GET",
      path: "/volcengine/api/v3/contents/generations/tasks/:task_id",
      type: "query",
      render: "queryTaskRender"
    }
  ],
  usageSchema: usageSchema(resolutions),
  usageExamples: usageExamples(resolutions),
  usageProfiles: [
    {
      models: ["doubao-seedance-2-0-fast", "doubao-seedance-2-0-mini"],
      schema: usageSchema(modelResolutions["doubao-seedance-2-0-fast"]),
      examples: usageExamples(modelResolutions["doubao-seedance-2-0-fast"])
    }
  ]
};
var native = {
  createTaskDecode,
  createTaskRender,
  queryTaskRender
};
var maxInt = 2147483647;
function int(value) {
  return value && Number.isInteger(value) && value > 0 && value <= maxInt ? Math.round(value) : 0;
}
export {
  buildContentRequest,
  buildQueryRequest,
  buildSubmitRequest,
  extractUsage,
  extractUsageOnComplete,
  int,
  listArtifacts,
  meta,
  native,
  parseSubmitResponse,
  parseTaskResult
};
