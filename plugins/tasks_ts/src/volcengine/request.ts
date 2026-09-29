import type { DriverContext } from "../../../../docs/plugin-api/v1"

export type RequestBody = {
	model: string
	frames?: number
	duration?: number
	resolution?: string
	content: any[]
}

export type ResponseBody = {
	id?: string
	task_id?: string
	status: "pending" | "queued" | "processing" | "running" | "succeeded" | "failed" | "expired" | "cancelled"
	usage?: {
		completion_tokens?: number
		total_tokens?: number
	}
	content?: {
		video_url?: string
		last_frame_url?: string
	}
	resolution?: Resolution
}

export const models = ["doubao-seedance-2-0", "doubao-seedance-2-0-fast", "doubao-seedance-2-0-mini"] as const
export const resolutions = ["480p", "720p", "1080p", "4k"] as const
export const modelResolutions: Record<Model, readonly Resolution[]> = {
	"doubao-seedance-2-0": ["480p", "720p", "1080p", "4k"],
	"doubao-seedance-2-0-fast": ["480p", "720p"],
	"doubao-seedance-2-0-mini": ["480p", "720p"],
}

export type Model = (typeof models)[number]

export type Resolution = (typeof resolutions)[number]

export function getModel(ctx: { model: string }): Model {
	const model = ctx.model as Model
	if (!models.includes(model)) {
		throw new Error("unsupported model")
	}
	return model
}

export function getDuration(body: RequestBody): number {
	let duration = 0
	if (body.frames !== undefined) {
		if (!Number.isInteger(body.frames) || body.frames < 1 || body.frames > 86400) {
			throw new Error("frames must be an integer between 1 and 86400")
		}
		duration = Math.ceil(body.frames / 24)
	}
	if (body.duration !== undefined) {
		if (!Number.isInteger(body.duration) || body.duration < 1 || body.duration > 3600) {
			throw new Error("duration must be an integer between 1 and 3600")
		}
		duration = Math.max(duration, body.duration)
	}
	return duration || 15
}

export function getResolution(model: Model, body: RequestBody): Resolution {
	if (body.resolution === undefined) {
		return "720p"
	}
	const allowedResolutions = modelResolutions[model]
	const resolution = body.resolution.trim() as Resolution
	if (!allowedResolutions.includes(resolution)) {
		throw new Error("resolution must be one of " + allowedResolutions.join(", "))
	}
	return resolution
}

export function getRequestUrl(url: string): string {
	if (url.endsWith("/contents/generations/tasks")) {
		return url
	}
	if (url.endsWith("/v1/video/tasks")) {
		return url
	}
	return url + "/api/v3/contents/generations/tasks"
}

// 将 draft_task content 中 task id 替换为上游 task id
export function mapContent(content: any[], originTasks: DriverContext["originTasks"]): any[] {
	return content.map((item: any) => {
		if (item?.type !== "draft_task" || !item?.draft_task?.id) {
			return item
		}
		const id = text(item?.draft_task?.id)
		const origin = (originTasks || []).find(function (task) {
			return task.taskId === id
		})
		if (!origin?.upstreamTaskId) {
			throw new Error("origin task is unavailable")
		}
		return {
			type: "draft_task",
			draft_task: { id: origin.upstreamTaskId },
		}
	})
}

function text(value: any): string {
	return typeof value === "string" ? value.trim() : ""
}
