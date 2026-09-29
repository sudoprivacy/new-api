// noinspection JSUnusedGlobalSymbols

import type {
	DriverContext,
	HookHTTPResponse,
	NormalizedTaskResult,
	RequestDescriptor,
	TaskArtifact,
	TaskQueryContext,
	UpstreamResponse,
} from "../../../../docs/plugin-api/v1"
import { getDuration, getModel, getRequestUrl, getResolution, mapContent, RequestBody, ResponseBody } from "./request"
import { estimatedTokens, Usage } from "./usage"
import { int } from "./index"

export function extractUsage(
	ctx: DriverContext & { usagePurpose?: "facts" | "billing_ratios" },
): Readonly<Record<string, string | number | boolean>> | null {
	if (ctx.usagePurpose === "billing_ratios") {
		return null
	}
	const model = getModel(ctx)
	const body = (ctx.requestBody || {}) as RequestBody
	const duration = getDuration(body)
	const resolution = getResolution(model, body)
	return {
		tokens: estimatedTokens(duration, resolution),
		resolution,
		video_input: body.content.some((c) => c && c.type === "video_url") ? "with video" : "without video",
	} satisfies Usage
}

export function extractUsageOnComplete(
	_task: TaskQueryContext,
	result: NormalizedTaskResult,
	body: ResponseBody,
): Readonly<Record<string, string | number | boolean>> | null {
	if (result.status !== "SUCCESS") {
		return {}
	}
	const tokens = int(body.usage?.completion_tokens) || int(body.usage?.total_tokens)
	return {
		...(tokens === undefined ? {} : { tokens: tokens }),
		...(body.resolution === undefined ? {} : { resolution: body.resolution }),
	} satisfies Usage
}

export function buildSubmitRequest(ctx: DriverContext): RequestDescriptor {
	const model = getModel(ctx)
	const body = (ctx.requestBody || {}) as RequestBody
	getDuration(body)
	getResolution(model, body)
	return {
		url: getRequestUrl(ctx.baseUrl),
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Accept: "application/json",
			Authorization: "Bearer " + ctx.apiKey,
		},
		body: {
			...body,
			model: ctx.upstreamModel || model,
			content: mapContent(body.content, ctx.originTasks),
		},
		action: ctx.action,
		rewriteModel: model,
	}
}

export function parseSubmitResponse(
	_ctx: DriverContext,
	response: UpstreamResponse,
): { taskId: string; taskData?: unknown; immediate?: NormalizedTaskResult; state?: unknown } {
	const body = response.body as ResponseBody
	const id = body.id || body.task_id
	if (!id) {
		throw new Error("task id not found")
	}
	return { taskId: id, taskData: response.body }
}

export function buildQueryRequest(ctx: TaskQueryContext): RequestDescriptor {
	if (!ctx.taskId) {
		throw new Error("task id not found")
	}
	return {
		url: getRequestUrl(ctx.baseUrl) + "/" + ctx.taskId,
		method: "GET",
		headers: { Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
	}
}

export function parseTaskResult(
	_ctx: TaskQueryContext,
	body: ResponseBody,
	_response: HookHTTPResponse,
): NormalizedTaskResult {
	switch (body.status) {
		case "pending":
		case "queued":
			return { status: "QUEUED", progress: "10%" }
		case "processing":
		case "running":
			return { status: "IN_PROGRESS", progress: "50%" }
		case "failed":
		case "expired":
		case "cancelled":
			return { status: "FAILURE", progress: "100%" }
		case "succeeded":
			return {
				status: "SUCCESS",
				progress: "100%",
				completionTokens: int(body.usage?.completion_tokens),
				totalTokens: int(body.usage?.total_tokens),
				url: body.content?.video_url,
			}
		default:
			return { status: "UNKNOWN", reason: "unrecognized status: " + body.status }
	}
}

export function listArtifacts(task: {
	taskId: string
	status: string
	action: string
	data: unknown
	producerVersion: string
}): readonly TaskArtifact[] {
	const content = (task.data as ResponseBody).content
	const artifacts: TaskArtifact[] = []
	if (!content) {
		return artifacts
	}
	if (content.video_url) {
		artifacts.push({ key: "video", type: "video" })
	}
	if (content.last_frame_url) {
		artifacts.push({ key: "last_frame", type: "image" })
	}
	return artifacts
}

export function buildContentRequest(
	ctx: DriverContext & {
		artifactKey: string
		data: unknown
		state?: unknown
		upstreamTaskId: string
		clientRequest: { method: "GET" | "HEAD"; headers: Readonly<Record<string, string>> }
	},
): RequestDescriptor {
	const content = (ctx.data as ResponseBody).content
	if (!content) {
		throw new Error("content not found")
	}
	const url = { video: content?.video_url, last_frame: content?.last_frame_url }[ctx.artifactKey]
	if (!url) {
		throw new Error("artifact not found")
	}
	return { url: url, credentialless: true }
}
