import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { setLocale, translate } from "./i18n";

test("shared workbench copy is available in both locales", () => {
  setLocale("en-US");
  assert.equal(translate("field.interfaceName"), "Interface name");
  assert.equal(translate("status.running"), "Running…");
  assert.equal(translate("status.savedToCollection"), "Saved to collection");
  assert.equal(translate("region.requestConfig"), "Request configuration");
  assert.equal(translate("workbench.operationMode", { protocol: "MQTT" }), "MQTT operation mode");
  assert.equal(translate("request.loading"), "Sending request");

  setLocale("zh-CN");
  assert.equal(translate("field.interfaceName"), "接口名称");
  assert.equal(translate("status.savedToCollection"), "已保存到集合");
});

test("protocol workbenches do not hard-code the shared save confirmation", () => {
  for (const file of ["AmqpWorkbench.tsx", "KafkaWorkbench.tsx", "MqttWorkbench.tsx", "RedisWorkbench.tsx", "RpcWorkbench.tsx", "SqlWorkbench.tsx"]) {
    const source = fs.readFileSync(path.resolve("src", file), "utf8");
    assert.match(source, /useI18n/);
    assert.doesNotMatch(source, /setMessage\("已保存到集合"\)/);
  }
});
