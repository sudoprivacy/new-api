// noinspection JSUnusedGlobalSymbols

import { modelResolutions, models, resolutions } from "./request"
import { Meta, NativeDecodeContext, TaskIntent, TaskView } from "../../../../docs/plugin-api/v1"
import { createTaskDecode, createTaskRender, queryTaskRender } from "./native"
import { usageExamples, usageSchema } from "./usage"

export * from "./driver"

export const meta: Meta = {
	apiVersion: 1,
	key: "seedance",
	name: "Seedance",
	icon: "Doubao.Color",
	description: { en: "Volcengine Seedance video generation", zh: "火山引擎 Seedance 视频生成" },
	version: "1.0.0",
	author: { name: "black-06" },
	upstreams: ["vendor", "new_api"],
	fetchMode: "per_task",
	models: models,
	routes: [
		{
			method: "POST",
			path: "/volcengine/api/v3/contents/generations/tasks",
			type: "submit",
			decode: "createTaskDecode",
			render: "createTaskRender",
		},
		{
			method: "GET",
			path: "/volcengine/api/v3/contents/generations/tasks/:task_id",
			type: "query",
			render: "queryTaskRender",
		},
	],
	usageSchema: usageSchema(resolutions),
	usageExamples: usageExamples(resolutions),
	usageProfiles: [
		{
			models: ["doubao-seedance-2-0-fast", "doubao-seedance-2-0-mini"],
			schema: usageSchema(modelResolutions["doubao-seedance-2-0-fast"]),
			examples: usageExamples(modelResolutions["doubao-seedance-2-0-fast"]),
		},
	],
}

export const native: Record<
	string,
	| ((ctx: NativeDecodeContext) => TaskIntent)
	| ((ctx: NativeDecodeContext, task: TaskView | readonly TaskView[]) => unknown)
> & {
	error?: (
		ctx: NativeDecodeContext,
		error: { code: string; message: string; httpStatus: number; retryable: boolean },
	) => unknown
} = {
	createTaskDecode: createTaskDecode,
	createTaskRender: createTaskRender,
	queryTaskRender: queryTaskRender,
}

const maxInt = 2147483647

export function int(value: any): number {
	return value && Number.isInteger(value) && value > 0 && value <= maxInt ? Math.round(value) : 0
}
