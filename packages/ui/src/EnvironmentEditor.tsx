import { useEffect, useState } from "react";
import { createEnvironmentResource, deleteEnvironmentResource, listEnvironmentResources, updateEnvironmentResource, type EnvironmentResource } from "./agentResources";
import { GLOBAL_VARIABLES_ENVIRONMENT_ID, LOCAL_ENVIRONMENT_ID, LOCAL_ENVIRONMENT_NAME } from "./environmentPresentation";
import { Icon } from "./Icons";
import { Button } from "./Components";
import { useFeedback } from "./Feedback";
import { RovingTabList } from "./RovingTabList";

export type { EnvironmentResource } from "./agentResources";

export interface EnvironmentEditorProps {
  onLoad: () => Promise<{ variables: Record<string, string>; globalParameters?: GlobalParameter[]; secretRefs?: string[] }>;
  onSave: (variables: Record<string, string>, secretRefs: string[], globalParameters: GlobalParameter[]) => Promise<void>;
  onPutSecret?: (name: string, value: string) => Promise<void>;
  onSecretExists?: (name: string) => Promise<boolean>;
  onDeleteSecret?: (name: string) => Promise<void>;
  onListEnvironments?: () => Promise<EnvironmentResource[]>;
  onCreateEnvironment?: (value: Partial<EnvironmentResource>) => Promise<EnvironmentResource>;
  onUpdateEnvironment?: (value: EnvironmentResource) => Promise<EnvironmentResource>;
  onDeleteEnvironment?: (id: string) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}

export type GlobalParameterLocation = "header" | "cookie" | "query" | "body";
export type GlobalParameterType = "string" | "number" | "integer" | "boolean" | "array" | "object";
export interface GlobalParameter { location: GlobalParameterLocation; name: string; parameterType?: GlobalParameterType; value: string; enabled: boolean; required?: boolean; description: string }

type Section = "global-variables" | "global-parameters" | "vault" | string;

function formatKv(values: Record<string, string>): string {
  return Object.entries(values).map(([key, value]) => `${key}=${value}`).join("\n");
}

function parseKv(text: string): Record<string, string> {
  const result: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const index = trimmed.indexOf("=");
    if (index > 0) result[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1);
  }
  return result;
}

function environmentBadge(name: string): string {
  return name.trim().slice(0, 1).toUpperCase() || "环";
}

function parseSecretRefs(value: string): string[] {
  return Array.from(new Set(value.split(",").map((item) => item.trim()).filter(Boolean)));
}

function duplicateKvKey(value: string): string | null {
  const seen = new Set<string>();
  for (const line of value.split("\n")) {
    const key = line.slice(0, Math.max(0, line.indexOf("="))).trim();
    if (!key) continue;
    if (seen.has(key)) return key;
    seen.add(key);
  }
  return null;
}

function globalParameterValidation(value: GlobalParameter[]): string | null {
  const seen = new Set<string>();
  for (const parameter of value) {
    const name = parameter.name.trim();
    if (!name && (parameter.value || parameter.description)) return "全局参数名称不能为空";
    if (!name) continue;
    const key = `${parameter.location}:${name.toLocaleLowerCase()}`;
    if (seen.has(key)) return `${locationLabels[parameter.location]} 中存在重复参数：${name}`;
    seen.add(key);
  }
  return null;
}

interface EnvironmentKvRow { id: string; key: string; value: string }
function createEnvironmentKvRow(key = "", value = ""): EnvironmentKvRow { return { id: crypto.randomUUID(), key, value }; }
function environmentKvRows(value: string): EnvironmentKvRow[] { return [...Object.entries(parseKv(value)).map(([key, itemValue]) => createEnvironmentKvRow(key, itemValue)), createEnvironmentKvRow()]; }
function environmentKvText(rows: EnvironmentKvRow[]): string { return rows.filter((row) => row.key.trim()).map((row) => `${row.key}=${row.value}`).join("\n"); }
function environmentKvRowHasContent(row: EnvironmentKvRow): boolean { return Boolean(row.key || row.value); }

function KeyValueEditor({ value, onChange, disabled, kind }: { value: string; onChange: (value: string) => void; disabled: boolean; kind: "变量" | "参数" }) {
  const [rows, setRows] = useState<EnvironmentKvRow[]>(() => environmentKvRows(value));
  useEffect(() => {
    setRows((current) => environmentKvText(current) === value ? current : environmentKvRows(value));
  }, [value]);
  const commit = (next: EnvironmentKvRow[]) => {
    setRows(next);
    onChange(environmentKvText(next));
  };
  const update = (id: string, patch: Partial<Pick<EnvironmentKvRow, "key" | "value">>) => {
    const next = rows.map((row) => row.id === id ? { ...row, ...patch } : row);
    const last = next[next.length - 1];
    commit(last && environmentKvRowHasContent(last) ? [...next, createEnvironmentKvRow()] : next);
  };
  const remove = (id: string) => {
    const next = rows.filter((row) => row.id !== id);
    const last = next[next.length - 1];
    commit(last && !environmentKvRowHasContent(last) ? next : [...next, createEnvironmentKvRow()]);
  };
  return <div className="environment-kv-editor">
    <div className="environment-kv-header"><span>{kind}名称</span><span>{kind}值</span><span/></div>
    {rows.map((row, index) => { const hasContent = environmentKvRowHasContent(row); return <div className={`environment-kv-row${hasContent ? " has-content" : " environment-kv-new"}`} key={row.id}><input aria-label={`${hasContent ? kind : `新${kind}`}名称${hasContent ? ` ${index + 1}` : ""}`} value={row.key} onChange={(event) => update(row.id, { key: event.target.value })} placeholder={hasContent ? "" : `添加${kind}`} disabled={disabled}/><input aria-label={`${hasContent ? kind : `新${kind}`}值${hasContent ? ` ${index + 1}` : ""}`} value={row.value} onChange={(event) => update(row.id, { value: event.target.value })} placeholder={hasContent ? "" : `${kind}值`} disabled={disabled}/>{hasContent ? <button type="button" aria-label={`删除${kind} ${row.key || index + 1}`} title="删除" disabled={disabled} onClick={() => remove(row.id)}><Icon name="trash"/></button> : <span aria-hidden="true"/>}</div>; })}
  </div>;
}

const locationLabels: Record<GlobalParameterLocation, string> = { header: "Header", cookie: "Cookie", query: "Query", body: "Body" };
interface GlobalParameterRow extends Omit<GlobalParameter, "parameterType" | "required"> { id: string; parameterType: GlobalParameterType; required: boolean; isEntry: boolean }
function createGlobalParameterRow(location: GlobalParameterLocation, parameter?: Partial<GlobalParameter>, isEntry = Boolean(parameter)): GlobalParameterRow { return { id: crypto.randomUUID(), location, name: parameter?.name ?? "", parameterType: parameter?.parameterType ?? "string", value: parameter?.value ?? "", enabled: parameter?.enabled ?? true, required: parameter?.required ?? false, description: parameter?.description ?? "", isEntry }; }
function globalParameterRows(value: GlobalParameter[]): GlobalParameterRow[] { return [...value.map((item) => createGlobalParameterRow(item.location, item)), ...(Object.keys(locationLabels) as GlobalParameterLocation[]).map((location) => createGlobalParameterRow(location))]; }
function globalParameterValues(rows: GlobalParameterRow[]): GlobalParameter[] { return rows.filter((row) => row.isEntry).map(({ id: _id, isEntry: _isEntry, ...row }) => row); }
function GlobalParameterEditor({ value, onChange, disabled }: { value: GlobalParameter[]; onChange: (value: GlobalParameter[]) => void; disabled: boolean }) {
  const [activeLocation, setActiveLocation] = useState<GlobalParameterLocation>("header");
  const [rows, setRows] = useState<GlobalParameterRow[]>(() => globalParameterRows(value));
  useEffect(() => {
    setRows((current) => JSON.stringify(globalParameterValues(current)) === JSON.stringify(value) ? current : globalParameterRows(value));
  }, [value]);
  const update = (id: string, patch: Partial<GlobalParameter>) => {
    const target = rows.find((row) => row.id === id);
    if (!target) return;
    const next = rows.map((row) => row.id === id ? { ...row, ...patch, isEntry: true } : row);
    if (!target.isEntry) next.push(createGlobalParameterRow(target.location));
    setRows(next);
    onChange(globalParameterValues(next));
  };
  const activate = (id: string) => {
    const target = rows.find((row) => row.id === id);
    if (!target || target.isEntry) return;
    const next = [...rows.map((row) => row.id === id ? { ...row, isEntry: true } : row), createGlobalParameterRow(target.location)];
    setRows(next);
    onChange(globalParameterValues(next));
  };
  const remove = (id: string) => {
    const next = rows.filter((row) => row.id !== id);
    setRows(next);
    onChange(globalParameterValues(next));
  };
  const visible = rows.filter((row) => row.location === activeLocation);
  return <div className="environment-parameter-editor">
    <div className="environment-parameter-callout"><Icon name="info"/><span>只注入 HTTP 请求；请求自身已存在同名 Header、Cookie、Query 或 Body 字段时，以请求值为准。值支持 <code>{"{{variable}}"}</code>。</span></div>
    <RovingTabList className="environment-parameter-tabs" ariaLabel="全局参数位置">
      {(Object.entries(locationLabels) as Array<[GlobalParameterLocation, string]>).map(([location, label]) => <button key={location} type="button" role="tab" tabIndex={activeLocation === location ? 0 : -1} aria-selected={activeLocation === location} className={activeLocation === location ? "is-active" : ""} onClick={() => setActiveLocation(location)} disabled={disabled}><span>{label}</span><small>{value.filter((item) => item.location === location && item.name.trim()).length}</small></button>)}
    </RovingTabList>
    <div className="environment-parameter-table">
    <div className="environment-parameter-header"><span>参数名</span><span>类型</span><span>默认值</span><span>默认启用</span><span>说明</span><span/></div>
    {visible.map((row, index) => { const isEntry = row.isEntry; return <div className={`environment-parameter-row${isEntry ? " has-content" : " is-new"}`} key={row.id}>
      <input aria-label={`${isEntry ? "全局参数" : `新${locationLabels[activeLocation]}全局参数`}名称${isEntry ? ` ${index + 1}` : ""}`} value={row.name} onFocus={() => activate(row.id)} onChange={(event) => update(row.id, { name: event.target.value })} placeholder={isEntry ? (activeLocation === "header" ? "Header 名称" : `${locationLabels[activeLocation]} 参数名`) : "添加参数"} disabled={disabled}/>
      <div className="environment-parameter-type"><select aria-label={`全局参数类型 ${row.name || index + 1}`} value={row.parameterType} onPointerDown={() => activate(row.id)} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") activate(row.id); }} onChange={(event) => update(row.id, { parameterType: event.target.value as GlobalParameterType })} disabled={disabled}><option value="string">string</option><option value="number">number</option><option value="integer">integer</option><option value="boolean">boolean</option><option value="array">array</option><option value="object">object</option></select><button type="button" className={`environment-parameter-required${row.required ? " is-active" : ""}`} aria-pressed={row.required} aria-label={`${row.required ? "取消必填" : "设为必填"} ${row.name || index + 1}`} title={row.required ? "必填" : "非必填"} onClick={() => update(row.id, { required: !row.required })} disabled={disabled}>*</button></div>
      <input aria-label={`${isEntry ? "全局参数" : `新${locationLabels[activeLocation]}全局参数`}值${isEntry ? ` ${index + 1}` : ""}`} value={row.value} onFocus={() => activate(row.id)} onChange={(event) => update(row.id, { value: event.target.value })} placeholder={isEntry ? "值或 {{变量}}" : ""} disabled={disabled}/>
      <button type="button" role="switch" className="environment-parameter-switch" aria-checked={row.enabled} aria-label={`${row.enabled ? "停用" : "启用"}全局参数 ${row.name || index + 1}`} onClick={() => update(row.id, { enabled: !row.enabled })} disabled={disabled}><span/></button>
      <input aria-label={`${isEntry ? "全局参数" : `新${locationLabels[activeLocation]}全局参数`}说明${isEntry ? ` ${index + 1}` : ""}`} value={row.description} onFocus={() => activate(row.id)} onChange={(event) => update(row.id, { description: event.target.value })} placeholder={isEntry ? "可选说明" : ""} disabled={disabled}/>
      {isEntry ? <button type="button" className="environment-parameter-delete" aria-label={`删除全局参数 ${row.name || index + 1}`} title="删除参数" onClick={() => remove(row.id)} disabled={disabled}><Icon name="minus-circle"/></button> : <span className="environment-parameter-delete-placeholder" aria-hidden="true"/>}
    </div>; })}
    </div>
  </div>;
}

export function EnvironmentEditor({ onLoad, onSave, onPutSecret, onSecretExists, onDeleteSecret, onListEnvironments = listEnvironmentResources, onCreateEnvironment = createEnvironmentResource, onUpdateEnvironment = updateEnvironmentResource, onDeleteEnvironment = deleteEnvironmentResource, onDirtyChange }: EnvironmentEditorProps) {
  const { confirm } = useFeedback();
  const [environments, setEnvironments] = useState<EnvironmentResource[]>([]);
  const [defaultVariables, setDefaultVariables] = useState<Record<string, string>>({});
  const [defaultSecretRefs, setDefaultSecretRefs] = useState<string[]>([]);
  const [active, setActive] = useState<Section>("global-variables");
  const [text, setText] = useState("");
  const [globalParameters, setGlobalParameters] = useState<GlobalParameter[]>([]);
  const [savedGlobalParameters, setSavedGlobalParameters] = useState<GlobalParameter[]>([]);
  const [secretRefs, setSecretRefs] = useState("");
  const [secretName, setSecretName] = useState("");
  const [secretValue, setSecretValue] = useState("");
  const [environmentName, setEnvironmentName] = useState("");
  const [message, setMessage] = useState("");
  const [validationError, setValidationError] = useState("");
  const [secretAvailability, setSecretAvailability] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);

  const selectedEnvironment = environments.find((item) => item.id === active);
  const isLocalEnvironment = selectedEnvironment?.id === LOCAL_ENVIRONMENT_ID;
  const hasUnsavedChanges = active === "global-variables"
    ? text !== formatKv(defaultVariables)
    : active === "global-parameters"
      ? JSON.stringify(globalParameters) !== JSON.stringify(savedGlobalParameters)
      : active === "vault"
        ? JSON.stringify(parseSecretRefs(secretRefs)) !== JSON.stringify(defaultSecretRefs)
        : Boolean(selectedEnvironment && (selectedEnvironment.id.startsWith("draft-") || (!isLocalEnvironment && environmentName !== selectedEnvironment.name) || text !== formatKv(selectedEnvironment.variables)));

  async function reload() {
    setBusy(true);
    setMessage("");
    try {
      const [list, defaults] = await Promise.all([onListEnvironments(), onLoad()]);
      setEnvironments(list.filter((item) => item.id !== GLOBAL_VARIABLES_ENVIRONMENT_ID));
      setDefaultVariables(defaults.variables ?? {});
      setGlobalParameters(defaults.globalParameters ?? []);
      setSavedGlobalParameters(defaults.globalParameters ?? []);
      setDefaultSecretRefs(defaults.secretRefs ?? []);
      if (onSecretExists) {
        const statuses = await Promise.all((defaults.secretRefs ?? []).map(async (name) => [name, await onSecretExists(name)] as const));
        setSecretAvailability(Object.fromEntries(statuses));
      }
      setMessage("环境配置已加载");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => { void reload(); }, []);
  useEffect(() => { onDirtyChange?.(hasUnsavedChanges); }, [hasUnsavedChanges, onDirtyChange]);
  useEffect(() => {
    if (!hasUnsavedChanges) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [hasUnsavedChanges]);
  useEffect(() => {
    if (active === "global-variables") setText(formatKv(defaultVariables));
    else if (active === "vault") setSecretRefs(defaultSecretRefs.join(", "));
    else if (selectedEnvironment) { setText(formatKv(selectedEnvironment.variables)); setEnvironmentName(selectedEnvironment.name); }
  }, [active, defaultVariables, defaultSecretRefs, selectedEnvironment]);

  function validateCurrent(): string | null {
    if (active === "global-parameters") return globalParameterValidation(globalParameters);
    if (active === "vault") return null;
    if (selectedEnvironment && !environmentName.trim()) return "请输入环境名称";
    const duplicate = duplicateKvKey(text);
    return duplicate ? `变量名称不能重复：${duplicate}` : null;
  }

  function discardCurrentChanges() {
    if (active === "global-parameters") setGlobalParameters(savedGlobalParameters);
    else if (active === "global-variables") setText(formatKv(defaultVariables));
    else if (active === "vault") setSecretRefs(defaultSecretRefs.join(", "));
    else if (selectedEnvironment?.id.startsWith("draft-")) setEnvironments((items) => items.filter((item) => item.id !== selectedEnvironment.id));
    else if (selectedEnvironment) {
      setEnvironmentName(selectedEnvironment.name);
      setText(formatKv(selectedEnvironment.variables));
    }
    setValidationError("");
  }

  async function selectSection(next: Section): Promise<boolean> {
    if (next === active) return true;
    if (hasUnsavedChanges && !await confirm({ title: "放弃未保存的更改？", description: "切换后，当前页面尚未保存的内容将丢失。", confirmLabel: "放弃更改", tone: "danger" })) return false;
    discardCurrentChanges();
    setMessage("");
    setActive(next);
    return true;
  }

  async function saveCurrent() {
    const error = validateCurrent();
    if (error) {
      setValidationError(error);
      setMessage("");
      return;
    }
    setBusy(true);
    setMessage("");
    setValidationError("");
    try {
      if (active === "global-parameters") {
        const parameters = globalParameters.filter((item) => item.name.trim()).map((item) => ({ ...item, name: item.name.trim() }));
        await onSave(defaultVariables, defaultSecretRefs, parameters);
        setGlobalParameters(parameters);
        setSavedGlobalParameters(parameters);
        window.dispatchEvent(new CustomEvent("protovoy-environment-updated"));
      } else if (active === "vault") {
        const refs = parseSecretRefs(secretRefs);
        await onSave(defaultVariables, refs, globalParameters);
        setDefaultSecretRefs(refs);
      } else if (selectedEnvironment) {
        const payload = { ...selectedEnvironment, name: environmentName.trim() || "未命名环境", variables: parseKv(text) };
        if (selectedEnvironment.id.startsWith("draft-")) {
          const created = await onCreateEnvironment({ projectId: payload.projectId, name: payload.name, variables: payload.variables, secretRefs: payload.secretRefs });
          setEnvironments((items) => items.map((item) => item.id === selectedEnvironment.id ? created : item));
          setActive(created.id);
        } else {
          const updated = await onUpdateEnvironment(payload);
          setEnvironments((items) => items.map((item) => item.id === updated.id ? updated : item));
        }
      } else {
        const values = parseKv(text);
        await onSave(values, defaultSecretRefs, globalParameters);
        setDefaultVariables(values);
      }
      setMessage("配置已保存");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }

  async function addEnvironment() {
    if (selectedEnvironment?.id.startsWith("draft-")) return;
    if (hasUnsavedChanges && !await confirm({ title: "放弃未保存的更改？", description: "新建环境前，需要先保存或放弃当前页面的修改。", confirmLabel: "放弃并新建", tone: "danger" })) return;
    discardCurrentChanges();
    const draft: EnvironmentResource = { id: `draft-${crypto.randomUUID()}`, projectId: "default-project", name: "", variables: {}, secretRefs: [], updatedAt: new Date().toISOString() };
    setEnvironments((items) => [...items, draft]);
    setActive(draft.id);
    setEnvironmentName("");
    setText("");
    setMessage("请填写环境名称和变量，完成后点击保存");
  }

  async function copyEnvironment() {
    if (!selectedEnvironment) return;
    setBusy(true);
    try {
      const created = await onCreateEnvironment({ ...selectedEnvironment, id: undefined, name: `${selectedEnvironment.name} 副本` });
      setEnvironments((items) => [...items, created]);
      setActive(created.id);
    } finally { setBusy(false); }
  }

  async function requestReload() {
    if (hasUnsavedChanges && !await confirm({ title: "重新加载环境配置？", description: "尚未保存的修改会被远端已保存内容覆盖。", confirmLabel: "重新加载", tone: "danger" })) return;
    discardCurrentChanges();
    await reload();
  }

  async function removeEnvironment() {
    if (!selectedEnvironment || isLocalEnvironment || !await confirm({ title: "删除环境", description: `删除环境“${selectedEnvironment.name}”后无法恢复。`, confirmLabel: "删除环境", tone: "danger" })) return;
    if (selectedEnvironment.id.startsWith("draft-")) {
      setEnvironments((items) => items.filter((item) => item.id !== selectedEnvironment.id));
      setActive("global-variables");
      return;
    }
    setBusy(true);
    try {
      await onDeleteEnvironment(selectedEnvironment.id);
      setEnvironments((items) => items.filter((item) => item.id !== selectedEnvironment.id));
      setActive("global-variables");
    } finally { setBusy(false); }
  }

  async function putSecret() {
    const name = secretName.trim();
    if (!onPutSecret || !name || !secretValue) return;
    setBusy(true);
    try {
      await onPutSecret(name, secretValue);
      const refs = Array.from(new Set([...defaultSecretRefs, name]));
      await onSave(defaultVariables, refs, globalParameters);
      setDefaultSecretRefs(refs);
      setSecretAvailability((items) => ({ ...items, [name]: true }));
      setSecretRefs(refs.join(", "));
      setSecretName("");
      setSecretValue("");
      setMessage("密钥已写入安全存储");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : String(error));
    } finally { setBusy(false); }
  }

  async function deleteSecret(name: string) {
    if (!onDeleteSecret || !await confirm({ title: "删除密钥", description: `将从系统安全存储中永久删除“${name}”，引用该密钥的请求将无法执行。`, confirmLabel: "删除密钥", tone: "danger" })) return;
    setBusy(true);
    try {
      const refs = defaultSecretRefs.filter((item) => item !== name);
      await onSave(defaultVariables, refs, globalParameters);
      await onDeleteSecret(name);
      setDefaultSecretRefs(refs);
      setSecretRefs(refs.join(", "));
      setSecretAvailability((items) => ({ ...items, [name]: false }));
      setMessage("密钥已从安全存储删除，引用已解除");
    } catch (error) { setMessage(error instanceof Error ? error.message : String(error)); }
    finally { setBusy(false); }
  }

  const editorTitle = active === "global-variables" ? "全局变量" : active === "global-parameters" ? "全局参数" : active === "vault" ? "Vault Secrets（密钥库）" : isLocalEnvironment ? LOCAL_ENVIRONMENT_NAME : selectedEnvironment?.name ?? "环境";
  const editorHint = active === "global-variables" ? "所有环境和协议请求均可引用的公共变量。" : active === "global-parameters" ? "为 HTTP 请求统一注入 Header、Cookie、Query 和 Body 字段。" : active === "vault" ? "密钥值只写入系统安全存储，项目中仅保存引用名称。" : isLocalEnvironment ? "默认用于本地调试；切换到其他环境时将使用对应的环境变量。" : "当前环境的变量会覆盖同名全局变量。";

  return <div className="environment-manager">
    <aside className="environment-manager-nav" aria-label="环境配置导航">
      <section><div className="environment-nav-label">全局</div>
        <button type="button" className={active === "global-variables" ? "is-active" : undefined} onClick={() => void selectSection("global-variables")}><Icon name="globe"/><span>全局变量</span></button>
        <button type="button" className={active === "global-parameters" ? "is-active" : undefined} onClick={() => void selectSection("global-parameters")}><Icon name="sliders"/><span>全局参数</span></button>
        <button type="button" className={active === "vault" ? "is-active" : undefined} onClick={() => void selectSection("vault")}><Icon name="archive"/><span>Vault Secrets（密钥库）</span></button>
      </section>
      <section className="environment-list"><div className="environment-nav-label">环境</div>
        {environments.map((environment) => { const name = environment.id === LOCAL_ENVIRONMENT_ID ? LOCAL_ENVIRONMENT_NAME : environment.name || "未命名环境"; return <button type="button" key={environment.id} className={active === environment.id ? "is-active" : undefined} onClick={() => void selectSection(environment.id)}><b>{environmentBadge(name)}</b><span>{name}</span></button>; })}
        <button type="button" className="environment-add" disabled={busy} onClick={() => void addEnvironment()}><Icon name="plus"/><span>新建环境</span></button>
      </section>
    </aside>
    <main className="environment-manager-content">
      <header><div><span className="environment-title-badge">{active === "vault" ? "密" : active.startsWith("global-") ? "全" : environmentBadge(editorTitle)}</span><div>{selectedEnvironment && !isLocalEnvironment ? <input className="environment-name-input" aria-label="环境名称" value={environmentName} onChange={(event) => { setEnvironmentName(event.target.value); setValidationError(""); }} placeholder="填写环境名称" disabled={busy} autoFocus={selectedEnvironment.id.startsWith("draft-")}/> : <h3>{editorTitle}</h3>}<p>{editorHint}</p></div></div></header>
      <div className="environment-editor-panel">
        {active === "vault" ? <>
          <label className="settings-field"><span>已关联的密钥引用</span><input value={secretRefs} onChange={(event) => { setSecretRefs(event.target.value); setValidationError(""); }} placeholder="例如：api_token, client_secret" disabled={busy}/><small>引用名会保存到工作区；密钥值不会写入数据库、导出文件或团队快照。</small></label>
          {defaultSecretRefs.length ? <div className="environment-secret-list" aria-label="密钥状态">{defaultSecretRefs.map((name) => <div key={name}><code>{name}</code><span className={secretAvailability[name] === false ? "is-missing" : "is-available"}>{secretAvailability[name] === false ? "安全存储中缺失" : "已安全存储"}</span>{onDeleteSecret ? <button type="button" aria-label={`删除密钥 ${name}`} onClick={() => void deleteSecret(name)} disabled={busy}><Icon name="trash"/></button> : null}</div>)}</div> : null}
          {onPutSecret ? <div className="environment-vault-card"><div><strong>添加或更新密钥</strong><small>保存后无法读取明文，只能通过引用名称使用。</small></div><div className="environment-secret-row"><input aria-label="密钥名称" value={secretName} onChange={(event) => setSecretName(event.target.value)} placeholder="密钥名称" disabled={busy}/><input aria-label="密钥值" type="password" value={secretValue} onChange={(event) => setSecretValue(event.target.value)} placeholder="密钥值" disabled={busy}/><Button variant="secondary" disabled={busy || !secretName.trim() || !secretValue} onClick={() => void putSecret()}>保存密钥</Button></div></div> : null}
        </> : active === "global-parameters" ? <GlobalParameterEditor value={globalParameters} onChange={(next) => { setGlobalParameters(next); setValidationError(""); }} disabled={busy}/> : <KeyValueEditor key={active} kind="变量" value={text} onChange={(next) => { setText(next); setValidationError(""); }} disabled={busy}/>}
      </div>
      <footer><div className="environment-footer-leading">{selectedEnvironment && !isLocalEnvironment ? <Button variant="danger" icon="trash" disabled={busy} onClick={() => void removeEnvironment()}>删除环境</Button> : null}<span role="status" className={validationError ? "is-error" : hasUnsavedChanges ? "is-dirty" : undefined}>{validationError || (hasUnsavedChanges ? "有未保存的更改" : message)}</span></div><div>{selectedEnvironment ? <Button variant="secondary" icon="copy" disabled={busy || hasUnsavedChanges || selectedEnvironment.id.startsWith("draft-")} onClick={() => void copyEnvironment()}>复制</Button> : null}<Button variant="secondary" disabled={busy} onClick={() => void requestReload()}>重新加载</Button><Button variant="primary" disabled={busy || !hasUnsavedChanges} onClick={() => void saveCurrent()}>保存</Button></div></footer>
    </main>
  </div>;
}
