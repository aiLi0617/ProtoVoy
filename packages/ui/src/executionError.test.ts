import assert from "node:assert/strict";
import { describeExecutionError } from "./executionError";

assert.equal(
  describeExecutionError(new TypeError("Failed to fetch"), "加载抓包状态"),
  "加载抓包状态失败：无法连接执行服务。请确认 Local Agent 或桌面后端已启动，并检查连接设置。",
);
assert.equal(
  describeExecutionError(new Error("request timed out"), "发送请求"),
  "发送请求超时。请检查目标地址与网络连接后重试。",
);
assert.equal(
  describeExecutionError(new Error("HTTP 401 Unauthorized"), "连接"),
  "连接失败：身份验证已失效，请检查连接令牌。",
);
assert.equal(describeExecutionError(new Error("invalid bind"), "启动抓包"), "启动抓包失败：invalid bind");

console.log("execution error tests passed");
