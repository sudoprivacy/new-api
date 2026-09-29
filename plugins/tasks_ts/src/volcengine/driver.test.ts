import { describe, expect, test } from "vitest"
import { Usage } from "./usage"
import { ResponseBody } from "./request"
import { extractUsageOnComplete } from "./driver"
import { NormalizedTaskResult, TaskQueryContext } from "../../../../docs/plugin-api/v1"

describe("extractUsageOnComplete", () => {
	test("should has tokens", () => {
		const ctx: TaskQueryContext = {} as TaskQueryContext
		const result: NormalizedTaskResult = { status: "SUCCESS" }

		type Testcase = {
			usage: {
				completion_tokens?: number
				total_tokens?: number
			}
			expected: Usage
		}

		const testcases: Testcase[] = [
			{
				usage: { completion_tokens: 1 },
				expected: { tokens: 1 },
			},
			{
				usage: { total_tokens: 2 },
				expected: { tokens: 2 },
			},
			{
				usage: { completion_tokens: undefined, total_tokens: 3 },
				expected: { tokens: 3 },
			},
			{
				usage: { completion_tokens: 0, total_tokens: 4 },
				expected: { tokens: 4 },
			},
		]
		testcases.forEach((testcase) => {
			const body: ResponseBody = { status: "succeeded", usage: testcase.usage }
			expect(extractUsageOnComplete(ctx, result, body)).toEqual(testcase.expected)
		})
	})
})
