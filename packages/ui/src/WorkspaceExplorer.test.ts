import assert from "node:assert/strict";
import { buildCollectionParentOptions, filterCollectionParentOptions, type CollectionRecord, type ModuleRecord } from "./WorkspaceExplorer";

const modules: ModuleRecord[] = [
  { id: "default", projectId: "project", name: "默认模块", isDefault: true },
  { id: "orders", projectId: "project", name: "订单模块", isDefault: false },
];
const collections: CollectionRecord[] = [
  { id: "orders-root", projectId: "project", moduleId: "orders", name: "订单", parentId: null, sortOrder: 0 },
  { id: "orders-child", projectId: "project", moduleId: "orders", name: "退款", parentId: "orders-root", sortOrder: 0 },
  { id: "default-root", projectId: "project", name: "公共", parentId: null, sortOrder: 0 },
];

const options = buildCollectionParentOptions(modules, collections);
assert.deepEqual(options, [
  { kind: "module", id: "default", moduleId: "default", label: "默认模块", depth: 0, parentKey: null },
  { kind: "collection", id: "default-root", moduleId: undefined, label: "公共", depth: 1, parentKey: "module:default" },
  { kind: "module", id: "orders", moduleId: "orders", label: "订单模块", depth: 0, parentKey: null },
  { kind: "collection", id: "orders-root", moduleId: "orders", label: "订单", depth: 1, parentKey: "module:orders" },
  { kind: "collection", id: "orders-child", moduleId: "orders", label: "退款", depth: 2, parentKey: "collection:orders-root" },
]);

assert.deepEqual(filterCollectionParentOptions(options, new Set(["module:orders"]), "").map((option) => option.label), ["默认模块", "公共", "订单模块"]);
assert.deepEqual(filterCollectionParentOptions(options, new Set(["module:orders", "collection:orders-root"]), "退款").map((option) => option.label), ["订单模块", "订单", "退款"]);

console.log("WorkspaceExplorer tests passed");