import { useEffect, useState } from "react";
import { createEnvironmentResource, deleteEnvironmentResource, listEnvironmentResources, updateEnvironmentResource, type EnvironmentResource } from "./agentResources";
import { Icon } from "./Icons";
import { Button } from "./Components";
import { useFeedback } from "./Feedback";
import { RovingTabList } from "./RovingTabList";

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
}

export type GlobalParameterLocation = "header" | "cookie" | "query" | "body";
export interface GlobalParameter { location: GlobalParameterLocation; name: string; value: string; enabled: boolean; description: string }

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

function KeyValueEditor({ value, onChange, disabled, kind }: { value: string; onChange: (value: string) => void; disabled: boolean; kind: "变量" | "参数" }) {
  const entries = Object.entries(parseKv(value));
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");
  const update = (index: number, field: "key" | "value", nextValue: string) => {
    const next = entries.map(([key, itemValue], itemIndex) => itemIndex === index ? [field === "key" ? nextValue : key, field === "value" ? nextValue : itemValue] as [string, string] : [key, itemValue] as [string, string]);
    onChange(next.filter(([key]) => key.trim()).map(([key, itemValue]) => `${key}=${itemValue}`).join("\n"));
  };
  const add = (key: string, itemValue: string) => {
    if (!key.trim()) return;
    onChange([...entries, [key, itemValue] as [string, string]].filter(([itemKey]) => itemKey.trim()).map(([itemKey, value]) => `${itemKey}=${value}`).join("\n"));
    setNewKey("");
    setNewValue("");
  };
  return <div className="environment-kv-editor">
    <div className="environment-kv-header"><span>{kind}名称</span><span>{kind}值</span><span>说明</span><span/></div>
    {entries.map(([key, itemValue], index) => <div className="environment-kv-row" key={`${key}-${index}`}><input aria-label={`${kind}名称 ${index + 1}`} value={key} onChange={(event) => update(index, "key", event.target.value)} disabled={disabled}/><input aria-label={`${kind}值 ${index + 1}`} value={itemValue} onChange={(event) => update(index, "value", event.target.value)} disabled={disabled}/><input aria-label={`${kind}说明 ${index + 1}`} placeholder="可选说明" disabled={disabled}/><button type="button" aria-label={`删除${kind} ${key}`} title="删除" disabled={disabled} onClick={() => onChange(entries.filter((_, itemIndex) => itemIndex !== index).map(([itemKey, value]) => `${itemKey}=${value}`).join("\n"))}><Icon name="trash"/></button></div>)}
    <div className="environment-kv-row environment-kv-new"><input aria-label={`新${kind}名称`} value={newKey} onChange={(event) => setNewKey(event.target.value)} placeholder={`添加${kind}`} disabled={disabled}/><input aria-label={`新${kind}值`} value={newValue} onChange={(event) => setNewValue(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") add(newKey, newValue); }} placeholder={`${kind}值`} disabled={disabled}/><input aria-label={`新${kind}说明`} placeholder="可选说明" disabled={disabled}/><button type="button" aria-label={`添加${kind}`} title="添加" disabled={disabled || !newKey.trim()} onClick={() => add(newKey, newValue)}><Icon name="plus"/></button></div>
  </div>;
}

const locationLabels: Record<GlobalParameterLocation, string> = { header: "Header", cookie: "Cookie", query: "Query", body: "Body" };
function GlobalParameterEditor({ value, onChange, disabled }: { value: GlobalParameter[]; onChange: (value: GlobalParameter[]) => void; disabled: boolean }) {
  const [activeLocation, setActiveLocation] = useState<GlobalParameterLocation>("header");
  const update = (index: number, patch: Partial<GlobalParameter>) => onChange(value.map((item, itemIndex) => itemIndex === index ? { ...item, ...patch } : item));
  const visible = value.map((item, index) => ({ item, index })).filter(({ item }) => item.location === activeLocation);
  const addFromPlaceholder = (patch: Partial<GlobalParameter>) => onChange([...value, { location: activeLocation, name: "", value: "", enabled: true, description: "", ...patch }]);
  return <div className="environment-parameter-editor">
    <div className="environment-parameter-callout"><Icon name="info"/><span>只注入 HTTP 请求；请求自身已存在同名 Header、Cookie、Query 或 Body 字段时，以请求值为准。值支持 <code>{"{{variable}}"}</code>。</span></div>
    <RovingTabList className="environment-parameter-tabs" ariaLabel="全局参数位置">
      {(Object.entries(locationLabels) as Array<[GlobalParameterLocation, string]>).map(([location, label]) => <button key={location} type="button" role="tab" tabIndex={activeLocation === location ? 0 : -1} aria-selected={activeLocation === location} className={activeLocation === location ? "is-active" : ""} onClick={() => setActiveLocation(location)} disabled={disabled}><span>{label}</span><small>{value.filter((item) => item.location === location && item.name.trim()).length}</small></button>)}
    </RovingTabList>
    <div className="environment-parameter-table">
    <div className="environment-parameter-header"><span/><span>名称</span><span>值</span><span>说明</span><span/></div>
    {visible.map(({ item, index }) => <div className="environment-parameter-row has-content" key={`${item.location}-${index}`}>
      <input type="checkbox" aria-label={`启用全局参数 ${item.name || index + 1}`} checked={item.enabled} onChange={(event) => update(index, { enabled: event.target.checked })} disabled={disabled}/>
      <input aria-label={`全局参数名称 ${index + 1}`} value={item.name} onChange={(event) => update(index, { name: event.target.value })} placeholder={item.location === "header" ? "X-Client-ID" : "参数名"} disabled={disabled}/>
      <input aria-label={`全局参数值 ${index + 1}`} value={item.value} onChange={(event) => update(index, { value: event.target.value })} placeholder="值或 {{变量}}" disabled={disabled}/>
      <input aria-label={`全局参数说明 ${index + 1}`} value={item.description} onChange={(event) => update(index, { description: event.target.value })} placeholder="可选说明" disabled={disabled}/>
      <button type="button" aria-label={`删除全局参数 ${item.name}`} title="删除" onClick={() => onChange(value.filter((_, itemIndex) => itemIndex !== index))} disabled={disabled}><Icon name="trash"/></button>
    </div>)}
    <div className="environment-parameter-row is-new">
      <input type="checkbox" aria-label={`启用新${locationLabels[activeLocation]}全局参数`} checked disabled={disabled} onChange={() => undefined}/>
      <input aria-label={`新${locationLabels[activeLocation]}全局参数名称`} placeholder={activeLocation === "header" ? "添加 Header" : `添加 ${locationLabels[activeLocation]} 参数`} disabled={disabled} onChange={(event) => { if (event.target.value) addFromPlaceholder({ name: event.target.value }); }}/>
      <input aria-label={`新${locationLabels[activeLocation]}全局参数值`} placeholder="值或 {{变量}}" disabled={disabled} onChange={(event) => { if (event.target.value) addFromPlaceholder({ value: event.target.value }); }}/>
      <input aria-label={`新${locationLabels[activeLocation]}全局参数说明`} placeholder="可选说明" disabled={disabled} onChange={(event) => { if (event.target.value) addFromPlaceholder({ description: event.target.value }); }}/>
      <span aria-hidden="true"/>
    </div>
    </div>
  </div>;
}

export function EnvironmentEditor({ onLoad, onSave, onPutSecret, onSecretExists, onDeleteSecret, onListEnvironments = listEnvironmentResources, onCreateEnvironment = createEnvironmentResource, onUpdateEnvironment = updateEnvironmentResource, onDeleteEnvironment = deleteEnvironmentResource }: EnvironmentEditorProps) {
  const { confirm } = useFeedback();
  const [environments, setEnvironments] = useState<EnvironmentResource[]>([]);
  const [defaultVariables, setDefaultVariables] = useState<Record<string, string>>({});
  const [defaultSecretRefs, setDefaultSecretRefs] = useState<string[]>([]);
  const [active, setActive] = useState<Section>("global-variables");
  const [text, setText] = useState("");
  const [globalParameters, setGlobalParameters] = useState<GlobalParameter[]>([]);
  const [secretRefs, setSecretRefs] = useState("");
  const [secretName, setSecretName] = useState("");
  const [secretValue, setSecretValue] = useState("");
  const [environmentName, setEnvironmentName] = useState("");
  const [message, setMessage] = useState("");
  const [secretAvailability, setSecretAvailability] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);

  const selectedEnvironment = environments.find((item) => item.id === active);

  async function reload() {
    setBusy(true);
    setMessage("");
    try {
      const [list, defaults] = await Promise.all([onListEnvironments(), onLoad()]);
      setEnvironments(list.filter((item) => item.id !== "default-env"));
      setDefaultVariables(defaults.variables ?? {});
      setGlobalParameters(defaults.globalParameters ?? []);
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
  useEffect(() => {
    if (active === "global-variables") setText(formatKv(defaultVariables));
    else if (active === "vault") setSecretRefs(defaultSecretRefs.join(", "));
    else if (selectedEnvironment) { setText(formatKv(selectedEnvironment.variables)); setEnvironmentName(selectedEnvironment.name); }
  }, [active, defaultVariables, defaultSecretRefs, selectedEnvironment]);

  async function saveCurrent() {
    setBusy(true);
    setMessage("");
    try {
      if (active === "global-parameters") {
        const parameters = globalParameters.filter((item) => item.name.trim()).map((item) => ({ ...item, name: item.name.trim() }));
        await onSave(defaultVariables, defaultSecretRefs, parameters);
        setGlobalParameters(parameters);
      } else if (active === "vault") {
        const refs = secretRefs.split(",").map((item) => item.trim()).filter(Boolean);
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

  async function removeEnvironment() {
    if (!selectedEnvironment || !await confirm({ title: "删除环境", description: `删除环境“${selectedEnvironment.name}”后无法恢复。`, confirmLabel: "删除环境", tone: "danger" })) return;
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

  const editorTitle = active === "global-variables" ? "全局变量" : active === "global-parameters" ? "全局参数" : active === "vault" ? "Vault Secrets（密钥库）" : selectedEnvironment?.name ?? "环境";
  const editorHint = active === "global-variables" ? "所有环境和协议请求均可引用的公共变量。" : active === "global-parameters" ? "为 HTTP 请求统一注入 Header、Cookie、Query 和 Body 字段。" : active === "vault" ? "密钥值只写入系统安全存储，项目中仅保存引用名称。" : "当前环境的变量会覆盖同名全局变量。";

  return <div className="environment-manager">
    <aside className="environment-manager-nav" aria-label="环境配置导航">
      <section><div className="environment-nav-label">全局</div>
        <button type="button" className={active === "global-variables" ? "is-active" : undefined} onClick={() => setActive("global-variables")}><Icon name="globe"/><span>全局变量</span></button>
        <button type="button" className={active === "global-parameters" ? "is-active" : undefined} onClick={() => setActive("global-parameters")}><Icon name="sliders"/><span>全局参数</span></button>
        <button type="button" className={active === "vault" ? "is-active" : undefined} onClick={() => setActive("vault")}><Icon name="archive"/><span>Vault Secrets（密钥库）</span></button>
      </section>
      <section className="environment-list"><div className="environment-nav-label">环境</div>
        {environments.map((environment) => <button type="button" key={environment.id} className={active === environment.id ? "is-active" : undefined} onClick={() => setActive(environment.id)}><b>{environmentBadge(environment.name)}</b><span>{environment.name || "未命名环境"}</span></button>)}
        <button type="button" className="environment-add" disabled={busy} onClick={() => void addEnvironment()}><Icon name="plus"/><span>新建环境</span></button>
      </section>
    </aside>
    <main className="environment-manager-content">
      <header><div><span className="environment-title-badge">{active === "vault" ? "密" : active.startsWith("global-") ? "全" : environmentBadge(editorTitle)}</span><div>{selectedEnvironment ? <input className="environment-name-input" aria-label="环境名称" value={environmentName} onChange={(event) => setEnvironmentName(event.target.value)} placeholder="填写环境名称" disabled={busy} autoFocus={selectedEnvironment.id.startsWith("draft-")}/> : <h3>{editorTitle}</h3>}<p>{editorHint}</p></div></div></header>
      <div className="environment-editor-panel">
        {active === "vault" ? <>
          <label className="settings-field"><span>已关联的密钥引用</span><input value={secretRefs} onChange={(event) => setSecretRefs(event.target.value)} placeholder="例如：api_token, client_secret" disabled={busy}/><small>引用名会保存到工作区；密钥值不会写入数据库、导出文件或团队快照。</small></label>
          {defaultSecretRefs.length ? <div className="environment-secret-list" aria-label="密钥状态">{defaultSecretRefs.map((name) => <div key={name}><code>{name}</code><span className={secretAvailability[name] === false ? "is-missing" : "is-available"}>{secretAvailability[name] === false ? "安全存储中缺失" : "已安全存储"}</span>{onDeleteSecret ? <button type="button" aria-label={`删除密钥 ${name}`} onClick={() => void deleteSecret(name)} disabled={busy}><Icon name="trash"/></button> : null}</div>)}</div> : null}
          {onPutSecret ? <div className="environment-vault-card"><div><strong>添加或更新密钥</strong><small>保存后无法读取明文，只能通过引用名称使用。</small></div><div className="environment-secret-row"><input aria-label="密钥名称" value={secretName} onChange={(event) => setSecretName(event.target.value)} placeholder="密钥名称" disabled={busy}/><input aria-label="密钥值" type="password" value={secretValue} onChange={(event) => setSecretValue(event.target.value)} placeholder="密钥值" disabled={busy}/><Button variant="secondary" disabled={busy || !secretName.trim() || !secretValue} onClick={() => void putSecret()}>保存密钥</Button></div></div> : null}
        </> : active === "global-parameters" ? <GlobalParameterEditor value={globalParameters} onChange={setGlobalParameters} disabled={busy}/> : <KeyValueEditor kind="变量" value={text} onChange={setText} disabled={busy}/>}
      </div>
      <footer><div className="environment-footer-leading">{selectedEnvironment ? <Button variant="danger" icon="trash" disabled={busy} onClick={() => void removeEnvironment()}>删除环境</Button> : null}<span role="status">{message}</span></div><div>{selectedEnvironment ? <Button variant="secondary" icon="copy" disabled={busy || selectedEnvironment.id.startsWith("draft-")} onClick={() => void copyEnvironment()}>复制</Button> : null}<Button variant="secondary" disabled={busy} onClick={() => void reload()}>重新加载</Button><Button variant="primary" disabled={busy} onClick={() => void saveCurrent()}>保存</Button></div></footer>
    </main>
  </div>;
}
