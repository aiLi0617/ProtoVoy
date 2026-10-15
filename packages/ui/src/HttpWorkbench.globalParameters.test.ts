import assert from "node:assert/strict";
import test from "node:test";
import {
  GLOBAL_PARAMETER_OVERRIDES_METADATA_KEY,
  globalParameterEnabled,
  globalParameterOverrideKey,
  readGlobalParameterOverrides,
  withGlobalParameterOverride,
  type HttpGlobalParameter,
} from "./HttpWorkbench";

const parameter: HttpGlobalParameter = {
  location: "header",
  name: "X-Region",
  parameterType: "string",
  value: "cn",
  enabled: true,
  description: "区域",
};

test("uses the environment enabled state until a request overrides it", () => {
  assert.equal(globalParameterOverrideKey(parameter), "header:x-region");
  assert.equal(globalParameterEnabled(parameter, {}), true);

  const disabled = withGlobalParameterOverride({}, parameter, false);
  assert.deepEqual(readGlobalParameterOverrides(disabled), { "header:x-region": false });
  assert.equal(globalParameterEnabled(parameter, disabled), false);
});

test("restoring the default removes only this request's matching override", () => {
  const metadata = {
    keep: "value",
    [GLOBAL_PARAMETER_OVERRIDES_METADATA_KEY]: {
      "header:x-region": false,
      "header:x-other": true,
      invalid: "ignored",
    },
  };
  const restored = withGlobalParameterOverride(metadata, parameter, undefined);
  assert.equal(restored.keep, "value");
  assert.deepEqual(readGlobalParameterOverrides(restored), { "header:x-other": true });
});

