import type { NativeDecodeContext, SubmitIntent, TaskView } from "../../../../docs/plugin-api/v1"
import { getDuration, getModel, getResolution, RequestBody } from "./request"

export function createTaskDecode(ctx: NativeDecodeContext): SubmitIntent {
	if (ctx.body.kind !== "json") {
		throw new Error("json body required")
	}
	const body = ctx.body.value as RequestBody
	const model = getModel(body)
	getDuration(body)
	getResolution(model, body)
	const draftTaskIDs = body.content
		.filter((item) => item?.type === "draft_task" && item?.draft_task?.id)
		.map((item) => String(item?.draft_task?.id))
	return {
		action: "textGenerate",
		kind: "submit",
		model: model,
		originTaskIds: draftTaskIDs,
		requestBody: body,
	}
}

export function createTaskRender(_ctx: NativeDecodeContext, task: TaskView | readonly TaskView[]): unknown {
	if (Array.isArray(task)) {
		throw new Error("task should not be an array")
	}
	const taskView = task as TaskView
	const data = taskView.data as Object
	return { ...data, id: taskView.task_id }
}

export function queryTaskRender(_ctx: NativeDecodeContext, task: TaskView | readonly TaskView[]): unknown {
	if (Array.isArray(task)) {
		throw new Error("task should not be an array")
	}
	const taskView = task as TaskView
	const data = taskView.data as Object
	return { ...data, id: taskView.task_id }
}
