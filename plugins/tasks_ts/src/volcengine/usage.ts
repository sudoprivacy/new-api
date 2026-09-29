import { Resolution } from "./request"
import type { UsageExample, UsageFieldSchema } from "../../../../docs/plugin-api/v1"

type UsageFields<T extends Record<keyof T, string | number | boolean>> = Partial<T>

export type Usage = UsageFields<{
	tokens: number
	resolution: Resolution
	video_input: VideoInput
}>

const videoInputs = ["with video", "without video"] as const
type VideoInput = (typeof videoInputs)[number]

export function usageSchema(resolutions: readonly Resolution[]): Readonly<Record<string, UsageFieldSchema>> {
	return {
		tokens: {
			type: "number",
			unit: "token",
			description: { en: "Video generation token unit price", zh: "视频生成 Token 单价" },
		},
		resolution: {
			enum: resolutions,
			description: { en: "Output video resolution", zh: "输出视频分辨率" },
		},
		video_input: {
			enum: videoInputs,
			enumLabels: {
				"with video": { en: "With reference video input", zh: "包含参考视频输入" },
				"without video": { en: "Without reference video input", zh: "不包含参考视频输入" },
			},
			description: { en: "Reference video input", zh: "参考视频输入" },
		},
	}
}

export function usageExamples(resolutions: readonly Resolution[]): readonly UsageExample[] {
	const duration = 5
	return resolutions.map((resolution) => {
		return {
			label: resolution + " * 5s",
			facts: {
				tokens: estimatedTokens(duration, resolution),
				resolution: resolution,
				video_input: "without video",
			} satisfies Usage,
		}
	})
}

export function estimatedTokens(seconds: number, resolution: Resolution) {
	const pixels = ((): number => {
		switch (resolution) {
			case "480p":
				return 480 * 854
			case "720p":
				return 720 * 1280
			case "1080p":
				return 1080 * 1920
			case "4k":
				return 3840 * 2160
			default:
				return 3840 * 2160
		}
	})()
	// token 用量 = (宽 * 高 * 帧率 * 时长) / 1024
	// see https://console.volcengine.com/ark/region:cn-beijing/model/detail?name=doubao-seedance-2-0 价格示例
	return Math.round((seconds * pixels * 24) / 1024)
}
