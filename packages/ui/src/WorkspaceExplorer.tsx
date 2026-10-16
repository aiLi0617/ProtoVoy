import { IMPORT_MAX_DOCUMENTS, IMPORT_MAX_FILE_BYTES, IMPORT_MAX_TOTAL_BYTES, importDocument, type PortableRequest } from "@protovoy/import-export";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useFeedback } from "./Feedback";
import { Icon } from "./Icons";
import { ModalFrame } from "./ModalFrame";
import { readInterfaceStructureMetadata } from "./interfaceStructureV2";
import { stashCaseInterfaceStructure } from "./caseStructureBridge";
import { useAppStore } from "./appStore";
import { Button, IconButton, Select } from "./Components";

export interface WorkspaceRecord { id: string; name: string; rootPath?: string | null; archived?: boolean; updatedAt?: string }
export interface ProjectRecord { id: string; workspaceId: string; name: string }
export interface ModuleRecord { id: string; projectId: string; name: string; isDefault: boolean }
export interface CollectionRecord { id: string; projectId: string; moduleId?: string; name: string; parentId?: string | null; sortOrder: number; tags?: string[] }
export interface RequestRecord { id: string; projectId: string; collectionId: string; name: string; protocolId?: string; method?: string; target: string; envelope?: { variables?: Record<string, string>; metadata?: Record<string, unknown> } }
export interface WorkspaceTree { workspaces: WorkspaceRecord[]; projects: ProjectRecord[]; modules?: ModuleRecord[]; collections: CollectionRecord[]; requests: RequestRecord[] }

export interface CollectionParentOption {
  kind: "module" | "collection";
  id: string;
  moduleId?: string;
  label: string;
  depth: number;
  parentKey: string | null;
}

function collectionParentOptionKey(option: CollectionParentOption): string {
  return `${option.kind}:${option.id}`;
}

export function buildCollectionParentOptions(modules: ModuleRecord[], collections: CollectionRecord[]): CollectionParentOption[] {
  const options: CollectionParentOption[] = [];
  for (const module of modules) {
    const moduleKey = `module:${module.id}`;
    options.push({ kind: "module", id: module.id, moduleId: module.id, label: module.name, depth: 0, parentKey: null });
    const moduleCollections = collections.filter((collection) => collection.moduleId ? collection.moduleId === module.id : module.isDefault);
    const visited = new Set<string>();
    const append = (collection: CollectionRecord, depth: number, parentKey: string) => {
      if (visited.has(collection.id)) return;
      visited.add(collection.id);
      const optionKey = `collection:${collection.id}`;
      options.push({ kind: "collection", id: collection.id, moduleId: collection.moduleId ?? (module.isDefault ? undefined : module.id), label: collection.name, depth, parentKey });
      moduleCollections
        .filter((candidate) => candidate.parentId === collection.id)
        .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name))
        .forEach((child) => append(child, depth + 1, optionKey));
    };
    const ids = new Set(moduleCollections.map((collection) => collection.id));
    moduleCollections
      .filter((collection) => !collection.parentId || !ids.has(collection.parentId))
      .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name))
      .forEach((collection) => append(collection, 1, moduleKey));
    moduleCollections.forEach((collection) => append(collection, 1, moduleKey));
  }
  return options;
}

export function filterCollectionParentOptions(options: CollectionParentOption[], collapsedKeys: ReadonlySet<string>, query: string): CollectionParentOption[] {
  const byKey = new Map(options.map((option) => [collectionParentOptionKey(option), option]));
  const normalizedQuery = query.trim().toLocaleLowerCase();
  if (normalizedQuery) {
    const visibleKeys = new Set<string>();
    for (const option of options) {
      if (!option.label.toLocaleLowerCase().includes(normalizedQuery)) continue;
      let current: CollectionParentOption | undefined = option;
      const visited = new Set<string>();
      while (current) {
        const key = collectionParentOptionKey(current);
        if (visited.has(key)) break;
        visited.add(key);
        visibleKeys.add(key);
        current = current.parentKey ? byKey.get(current.parentKey) : undefined;
      }
    }
    return options.filter((option) => visibleKeys.has(collectionParentOptionKey(option)));
  }
  return options.filter((option) => {
    let parentKey = option.parentKey;
    const visited = new Set<string>();
    while (parentKey) {
      if (visited.has(parentKey) || collapsedKeys.has(parentKey)) return false;
      visited.add(parentKey);
      parentKey = byKey.get(parentKey)?.parentKey ?? null;
    }
    return true;
  });
}

export function setModuleCollectionsCollapsed(
  current: string[],
  moduleId: string,
  collectionIds: string[],
  collapsed: boolean,
): string[] {
  const collectionKeys = new Set(collectionIds.map((id) => `collection:${id}`));
  const next = current.filter((key) => key !== `module:${moduleId}` && !collectionKeys.has(key));
  return collapsed ? [...next, ...collectionKeys] : next;
}

export function isCollectionActive(
  collectionId: string,
  selectedCollectionId?: string | null,
  selectedRequestId?: string | null,
): boolean {
  return collectionId === selectedCollectionId && !selectedRequestId;
}

function CollectionParentPicker({ options, value, onChange }: { options: CollectionParentOption[]; value: string; onChange: (option: CollectionParentOption) => void }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [collapsedKeys, setCollapsedKeys] = useState<Set<string>>(() => {
    const next = new Set(options.filter((option) => option.kind === "collection").map(collectionParentOptionKey));
    const byKey = new Map(options.map((option) => [collectionParentOptionKey(option), option]));
    let parentKey = byKey.get(value)?.parentKey;
    while (parentKey) {
      next.delete(parentKey);
      parentKey = byKey.get(parentKey)?.parentKey ?? null;
    }
    return next;
  });

  const optionsByKey = useMemo(() => new Map(options.map((option) => [collectionParentOptionKey(option), option])), [options]);
  const selected = optionsByKey.get(value);
  const childCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const option of options) {
      if (option.parentKey) counts.set(option.parentKey, (counts.get(option.parentKey) ?? 0) + 1);
    }
    return counts;
  }, [options]);
  const visibleOptions = useMemo(() => filterCollectionParentOptions(options, collapsedKeys, query), [collapsedKeys, options, query]);
  const select = (option: CollectionParentOption) => {
    setCollapsedKeys((current) => {
      const next = new Set(current);
      let parentKey = option.parentKey;
      while (parentKey) {
        next.delete(parentKey);
        parentKey = optionsByKey.get(parentKey)?.parentKey ?? null;
      }
      return next;
    });
    onChange(option);
    close();
  };
  const close = () => { setOpen(false); setQuery(""); };
  const toggle = (key: string) => setCollapsedKeys((current) => {
    const next = new Set(current);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    return next;
  });
  const selectedLabel = selected ? `${selected.kind === "module" ? "模块：" : "目录："}${selected.label}` : "请选择父级目录";
  const toggleOpen = () => {
    if (open) close();
    else {
      setOpen(true);
      window.setTimeout(() => searchInputRef.current?.focus(), 0);
    }
  };
  return <div className="collection-parent-field">
    <span id="collection-parent-label">父级目录</span>
    <div className={`collection-parent-picker${open ? " is-open" : ""}`}>
      <div className={`collection-parent-control${open ? " is-open" : ""}`}>
        <Icon name={open ? "search" : selected?.kind === "module" ? "module" : "folder"}/>
        <input ref={searchInputRef} type="text" role="combobox" readOnly={!open} value={open ? query : selectedLabel} onClick={() => { if (!open) { setOpen(true); window.setTimeout(() => searchInputRef.current?.focus(), 0); } }} onChange={(event) => { if (open) setQuery(event.target.value); }} onKeyDown={(event) => { if (event.key === "Escape" && open) close(); else if (!open && ["Enter", " ", "ArrowDown"].includes(event.key)) { event.preventDefault(); setOpen(true); } }} placeholder={open ? "搜索模块或目录" : "请选择父级目录"} aria-labelledby="collection-parent-label" aria-controls="collection-parent-options" aria-haspopup="tree" aria-expanded={open}/>
        <button type="button" className="collection-parent-control-toggle" aria-label={open ? "收起父级目录" : "展开父级目录"} onMouseDown={(event) => event.preventDefault()} onClick={toggleOpen}><Icon name="chevron"/></button>
      </div>
      {open ? <div className="collection-parent-tree">
        <div id="collection-parent-options" className="collection-parent-results" role="tree" aria-label="选择父级目录">
          {visibleOptions.length ? visibleOptions.map((option) => {
            const optionValue = collectionParentOptionKey(option);
            const isSelected = optionValue === value;
            const hasChildren = (childCounts.get(optionValue) ?? 0) > 0;
            const expanded = query.trim() ? true : !collapsedKeys.has(optionValue);
            const kindLabel = option.kind === "module" ? "模块" : "目录";
            return <div key={optionValue} className={`collection-parent-option is-${option.kind}${isSelected ? " is-selected" : ""}`} role="treeitem" aria-selected={isSelected} aria-expanded={hasChildren ? expanded : undefined} style={{ paddingLeft: 6 + option.depth * 18 }}>
              {hasChildren ? <button type="button" className="collection-parent-toggle" aria-label={`${expanded ? "收起" : "展开"}${kindLabel} ${option.label}`} onClick={() => toggle(optionValue)} disabled={Boolean(query.trim())}><Icon name="chevron"/></button> : <span className="collection-parent-toggle-spacer"/>}
              <button type="button" className="collection-parent-select" onClick={() => select(option)}><Icon name={option.kind === "module" ? "module" : "folder"}/><span>{option.label}</span>{option.kind === "module" ? <small>模块</small> : null}</button>
            </div>;
          }) : <div className="collection-parent-empty">没有匹配的模块或目录</div>}
        </div>
      </div> : null}
    </div>
  </div>;
}

function isDebugCase(request: RequestRecord): boolean {
  return Boolean(request.envelope?.variables?.__protovoyCaseOf) && request.envelope?.metadata?.__protovoyCaseType !== "test";
}
const PRIMARY_CREATE_PROTOCOLS = [["http", "HTTP", "globe"], ["grpc", "gRPC", "network"], ["websocket", "WebSocket", "activity"], ["sse", "SSE", "activity"], ["sql", "SQL", "database"]] as const;
const MORE_CREATE_PROTOCOLS = [["tcp", "TCP", "network"], ["udp", "UDP", "network"], ["mqtt", "MQTT", "network"], ["amqp", "AMQP", "network"], ["kafka", "Kafka", "network"], ["redis", "Redis", "database"]] as const;

const CREATE_MENU_WIDTH = 320;

export interface WorkspaceExplorerProps {
  tree: WorkspaceTree | null;
  loading?: boolean;
  error?: string | null;
  onRetry?: () => void;
  selectedCollectionId?: string | null;
  selectedRequestId?: string | null;
  selectedProjectId?: string | null;
  onSelectCollection: (projectId: string, collectionId: string) => void;
  onOpenRequest: (id: string) => void;
  onCreateWorkspace: (name: string) => Promise<void>;
  onRenameWorkspace: (id: string, name: string) => Promise<void>;
  onArchiveWorkspace: (id: string, archived: boolean) => Promise<void>;
  onDeleteWorkspace: (id: string) => Promise<void>;
  onTouchWorkspace: (id: string) => Promise<void>;
  onCreateProject: (workspaceId: string, name: string) => Promise<void>;
  onCreateModule?: (projectId: string, name: string) => Promise<void>;
  onRenameModule?: (id: string, name: string) => Promise<void>;
  onRenameProject: (id: string, name: string) => Promise<void>;
  onDeleteProject: (id: string) => Promise<void>;
  onCreateCollection: (projectId: string, parentId: string | null, name: string, moduleId?: string) => Promise<void>;
  onRenameCollection: (collection: CollectionRecord, name: string) => Promise<void>;
  onUpdateCollectionTags: (collection: CollectionRecord, tags: string[]) => Promise<void>;
  onDeleteCollection: (id: string) => Promise<void>;
  onDeleteRequest: (id: string) => Promise<void>;
  onRenameRequest?: (request: RequestRecord, name: string) => Promise<void>;
  onDuplicateRequest?: (request: RequestRecord) => Promise<void>;
  onCopyRequestAsCurl?: (request: RequestRecord) => Promise<string>;
  onMoveCollection: (collection: CollectionRecord, projectId: string, parentId: string | null) => Promise<void>;
  onSwapCollections: (first: CollectionRecord, second: CollectionRecord) => Promise<void>;
  onMoveRequest: (id: string, projectId: string, collectionId: string) => Promise<void>;
  onImportRequests: (projectId: string, collectionId: string, requests: PortableRequest[]) => Promise<void>;
  onExportProject: (project: ProjectRecord) => Promise<PortableRequest[]>;
}

export function WorkspaceExplorer(props: WorkspaceExplorerProps) {
  const [draft, setDraft] = useState<{ kind: "project"; owner: string } | { kind: "module"; owner: string } | { kind: "collection"; owner: string; parentId: string | null; moduleId?: string } | null>(null);
  const [name, setName] = useState("");
  const [query, setQuery] = useState("");
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [batchMode, setBatchMode] = useState(false);
  const [batchTarget, setBatchTarget] = useState("");
  const [openMenuId, setOpenMenuId] = useState<string | null>(null);
  const [menuPos, setMenuPos] = useState<{ top: number; left: number } | null>(null);
  const [createMenuOpen, setCreateMenuOpen] = useState(false);
  const [moreProtocolsOpen, setMoreProtocolsOpen] = useState(false);
  const [createMenuPos, setCreateMenuPos] = useState<{ top: number; left: number; width: number } | null>(null);
  const menuButtonRefs = useRef(new Map<string, HTMLButtonElement>());
  const createMenuRef = useRef<HTMLDivElement>(null);
  const createMenuButtonRef = useRef<HTMLButtonElement>(null);
  const collapsedNodes = useAppStore((state) => state.collapsedExplorerNodes);
  const toggleExplorerNode = useAppStore((state) => state.toggleExplorerNode);
  const importInput = useRef<HTMLInputElement>(null);
  const { notify, confirm, prompt } = useFeedback();
  useEffect(() => {
    const workspaceTree = props.tree;
    if (!workspaceTree || !props.selectedRequestId) return;
    const request = workspaceTree.requests.find((item) => item.id === props.selectedRequestId);
    if (!request) return;
    const expandedKeys = new Set<string>();
    let collection = workspaceTree.collections.find((item) => item.id === request.collectionId);
    while (collection) {
      expandedKeys.add(`collection:${collection.id}`);
      collection = collection.parentId ? workspaceTree.collections.find((item) => item.id === collection!.parentId) : undefined;
    }
    const moduleId = workspaceTree.collections.find((item) => item.id === request.collectionId)?.moduleId;
    if (moduleId) expandedKeys.add(`module:${moduleId}`);
    const parentId = request.envelope?.variables?.__protovoyCaseOf;
    if (parentId) expandedKeys.add(`request:${parentId}`);
    if (collapsedNodes.some((key) => expandedKeys.has(key))) {
      useAppStore.setState({ collapsedExplorerNodes: collapsedNodes.filter((key) => !expandedKeys.has(key)) });
    }
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-request-id="${CSS.escape(request.id)}"]`)?.scrollIntoView({ block: "nearest" }));
  }, [collapsedNodes, props.selectedRequestId, props.tree]);
  function onTreeKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
    if (!["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const root = event.currentTarget;
    const items = Array.from(root.querySelectorAll<HTMLElement>('[role="treeitem"]')).filter((item) => item.offsetParent !== null);
    if (!items.length) return;
    const activeItem = (document.activeElement as HTMLElement | null)?.closest<HTMLElement>('[role="treeitem"]');
    const current = Math.max(0, activeItem ? items.indexOf(activeItem) : 0);
    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      const chevron = items[current].querySelector<HTMLButtonElement>(".tree-chevron, .tree-module-chevron");
      if (chevron && ((event.key === "ArrowLeft" && chevron.getAttribute("aria-expanded") === "true") || (event.key === "ArrowRight" && chevron.getAttribute("aria-expanded") === "false"))) { event.preventDefault(); chevron.click(); }
      return;
    }
    event.preventDefault();
    const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : event.key === "ArrowDown" ? Math.min(items.length - 1, current + 1) : Math.max(0, current - 1);
    (items[next].querySelector<HTMLElement>(".tree-main, .tree-chevron, .tree-module-chevron") ?? items[next]).focus();
  }

  useEffect(() => {
    const openImport = () => importInput.current?.click();
    window.addEventListener("protovoy-import-requests", openImport);
    return () => window.removeEventListener("protovoy-import-requests", openImport);
  }, []);
  useEffect(() => {
    if (!openMenuId) return;
    const dismiss = () => { setOpenMenuId(null); setMenuPos(null); };
    const onPointerDown = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.closest(`[data-tree-menu="${openMenuId}"]`)) return;
      dismiss();
    };
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") dismiss(); };
    window.addEventListener("mousedown", onPointerDown);
    window.addEventListener("keydown", onKey);
    window.addEventListener("resize", dismiss);
    window.addEventListener("scroll", dismiss, true);
    return () => {
      window.removeEventListener("mousedown", onPointerDown);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("resize", dismiss);
      window.removeEventListener("scroll", dismiss, true);
    };
  }, [openMenuId]);
  useEffect(() => {
    if (!createMenuOpen) return;
    const close = (event: MouseEvent) => { const target = event.target as HTMLElement | null; if (!createMenuRef.current?.contains(target) && !target?.closest("[data-workspace-create-menu]")) setCreateMenuOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") setCreateMenuOpen(false); };
    const dismiss = () => setCreateMenuOpen(false);
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", escape);
    window.addEventListener("resize", dismiss);
    window.addEventListener("scroll", dismiss, true);
    return () => { window.removeEventListener("mousedown", close); window.removeEventListener("keydown", escape); window.removeEventListener("resize", dismiss); window.removeEventListener("scroll", dismiss, true); };
  }, [createMenuOpen]);
  useLayoutEffect(() => {
    if (!createMenuOpen) { setCreateMenuPos(null); return; }
    const rect = createMenuButtonRef.current?.getBoundingClientRect();
    if (!rect) return;
    setCreateMenuPos({ top: rect.bottom + 4, left: rect.left, width: Math.min(CREATE_MENU_WIDTH, window.innerWidth - rect.left - 8) });
  }, [createMenuOpen]);
  useLayoutEffect(() => {
    if (!openMenuId) { setMenuPos(null); return; }
    const button = menuButtonRefs.current.get(openMenuId);
    if (!button) return;
    const rect = button.getBoundingClientRect();
    const width = 168;
    const estimatedHeight = 260;
    const left = Math.min(Math.max(8, rect.right - width), window.innerWidth - width - 8);
    const below = rect.bottom + 4;
    const top = below + estimatedHeight > window.innerHeight - 8
      ? Math.max(8, rect.top - estimatedHeight - 4)
      : below;
    setMenuPos({ top, left });
  }, [openMenuId]);

  function openCollectionMenu(collectionId: string) {
    setOpenMenuId((current) => {
      if (current === collectionId) { setMenuPos(null); return null; }
      return collectionId;
    });
  }

  function renderCollectionMenu(collection: CollectionRecord, siblingIndex: number, siblings: CollectionRecord[]): ReactNode {
    if (openMenuId !== collection.id || !menuPos || typeof document === "undefined") return null;
    return createPortal(
      <div className="tree-more-panel" role="menu" data-tree-menu={collection.id} style={{ top: menuPos.top, left: menuPos.left }}>
        <button type="button" role="menuitem" onClick={() => { props.onSelectCollection(collection.projectId, collection.id); importInput.current?.click(); setOpenMenuId(null); }}><Icon name="download" />导入到此集合</button>
        <button type="button" role="menuitem" disabled={siblingIndex <= 0} onClick={() => { void props.onSwapCollections(collection, siblings[siblingIndex - 1]); setOpenMenuId(null); }}><Icon name="arrow-up" />上移</button>
        <button type="button" role="menuitem" disabled={siblingIndex < 0 || siblingIndex >= siblings.length - 1} onClick={() => { void props.onSwapCollections(collection, siblings[siblingIndex + 1]); setOpenMenuId(null); }}><Icon name="arrow-down" />下移</button>
        <button type="button" role="menuitem" onClick={() => { setDraft({ kind: "collection", owner: collection.projectId, parentId: collection.id }); setOpenMenuId(null); }}><Icon name="plus" />新建子集合</button>
        <button type="button" role="menuitem" onClick={async () => { setOpenMenuId(null); const value = (await prompt({ title: "重命名集合", initialValue: collection.name }))?.trim(); if (value) void props.onRenameCollection(collection, value); }}><Icon name="edit" />重命名</button>
        <button type="button" role="menuitem" onClick={async () => { setOpenMenuId(null); const value = await prompt({ title: "设置集合标签", initialValue: collection.tags?.join(", ") ?? "", placeholder: "标签以逗号分隔" }); if (value !== null) void props.onUpdateCollectionTags(collection, value.split(",").map((tag) => tag.trim()).filter(Boolean)); }}><Icon name="tag" />标签</button>
        {collection.id !== "default-collection" ? <button type="button" role="menuitem" onClick={async () => { setOpenMenuId(null); if (await confirm({ title: "删除集合", description: `删除集合“${collection.name}”及其中全部内容？`, tone: "danger", confirmLabel: "删除" })) void props.onDeleteCollection(collection.id); }}><Icon name="trash" />删除</button> : null}
      </div>,
      document.body,
    );
  }

  function renderModuleMenu(module: ModuleRecord): ReactNode {
    const menuId = `module:${module.id}:menu`;
    if (openMenuId !== menuId || !menuPos || typeof document === "undefined") return null;
    return createPortal(
      <div className="tree-more-panel" role="menu" data-tree-menu={menuId} style={{ top: menuPos.top, left: menuPos.left }}>
        <button type="button" role="menuitem" onClick={() => { setDraft({ kind: "collection", owner: module.projectId, parentId: null, moduleId: module.id }); setOpenMenuId(null); }}><Icon name="plus"/>新建目录</button>
        <button type="button" role="menuitem" disabled={!props.onRenameModule} onClick={async () => {
          setOpenMenuId(null);
          const value = (await prompt({ title: "重命名模块", initialValue: module.name }))?.trim();
          if (!value || value === module.name || !props.onRenameModule) return;
          try {
            await props.onRenameModule(module.id, value);
            notify(`模块已重命名为“${value}”`, "success");
          } catch (error) {
            notify(`重命名模块失败：${error instanceof Error ? error.message : String(error)}`, "danger");
          }
        }}><Icon name="edit"/>重命名</button>
      </div>,
      document.body,
    );
  }

  function renderRequestMenu(request: RequestRecord): ReactNode {
    if (openMenuId !== `request:${request.id}` || !menuPos || typeof document === "undefined") return null;
    const canCopyCurl = (request.protocolId ?? "http") === "http";
    return createPortal(
      <div className="tree-more-panel" role="menu" data-tree-menu={`request:${request.id}`} style={{ top: menuPos.top, left: menuPos.left }}>
        <button type="button" role="menuitem" disabled={!props.onRenameRequest} onClick={async () => { setOpenMenuId(null); const value = (await prompt({ title: "重命名接口", initialValue: request.name }))?.trim(); if (value) await props.onRenameRequest?.(request, value); }}><Icon name="edit" />重命名</button>
        <button type="button" role="menuitem" disabled={!props.onDuplicateRequest} onClick={async () => { setOpenMenuId(null); if (props.onDuplicateRequest) { await props.onDuplicateRequest(request); notify(`已复制接口“${request.name}”`, "success"); } }}><Icon name="copy" />复制</button>
        <button type="button" role="menuitem" disabled={!canCopyCurl || !props.onCopyRequestAsCurl} title={canCopyCurl ? undefined : "仅 HTTP 接口支持复制 cURL"} onClick={async () => { setOpenMenuId(null); const curl = await props.onCopyRequestAsCurl?.(request); if (curl) { await navigator.clipboard.writeText(curl); notify("已复制 cURL", "success"); } }}><Icon name="code" />复制 cURL</button>
        <div className="tree-more-panel-divider" role="separator" />
        <button type="button" className="is-danger" role="menuitem" onClick={async () => { setOpenMenuId(null); if (await confirm({ title: "删除接口", description: `确定删除接口“${request.name}”吗？`, tone: "danger", confirmLabel: "删除" })) await deleteRequest(request.id, request.name); }}><Icon name="trash" />删除</button>
      </div>,
      document.body,
    );
  }

  async function importFiles(files?: FileList | null) {
    if (!files?.length) return;
    const selected = [...files];
    const file = selected[0];
    const collection = tree?.collections.find((item) => item.id === props.selectedCollectionId);
    if (!collection) { notify("请先选择一个目标集合", "warning"); return; }
    try {
      if (selected.length > IMPORT_MAX_DOCUMENTS) throw new Error(`导入文档数量超过 ${IMPORT_MAX_DOCUMENTS} 个的上限`);
      const oversized = selected.find((item) => item.size > IMPORT_MAX_FILE_BYTES);
      if (oversized) throw new Error(`${oversized.name} 超过 ${IMPORT_MAX_FILE_BYTES} 字节的导入上限`);
      const totalBytes = selected.reduce((total, item) => total + item.size, 0);
      if (totalBytes > IMPORT_MAX_TOTAL_BYTES) throw new Error(`导入文档总大小超过 ${IMPORT_MAX_TOTAL_BYTES} 字节的上限`);
      const dependencyEntries = await Promise.all(selected.slice(1).map(async (dependency) => {
        const text = await dependency.text();
        return [dependency.webkitRelativePath || dependency.name, text] as const;
      }));
      const result = await importDocument(await file.text(), {
        baseUri: file.webkitRelativePath || file.name,
        documents: Object.fromEntries(dependencyEntries),
      });
      await props.onImportRequests(collection.projectId, collection.id, result.requests);
      notify(`已从 ${result.source} 导入 ${result.requests.length} 个请求${result.warnings.length ? `\\n${result.warnings.join("\\n")}` : ""}`, "success");
    } catch (error) { notify(error instanceof Error ? error.message : String(error), "danger"); }
    if (importInput.current) importInput.current.value = "";
  }

  async function submit() {
    const value = name.trim();
    if (!draft || !value) return;
    if (draft.kind === "project") await props.onCreateProject(draft.owner, value);
    else if (draft.kind === "module") await props.onCreateModule?.(draft.owner, value);
    else await props.onCreateCollection(draft.owner, draft.parentId, value, draft.moduleId);
    setDraft(null);
    setName("");
  }

  const tree = props.tree;
  if (props.loading) return <div className="workspace-tree-empty" role="status" aria-live="polite">正在加载工作区…</div>;
  if (props.error) return <div className="workspace-load-error" role="alert"><Icon name="archive"/><strong>无法加载工作区</strong><span>{props.error}</span><div><button type="button" className="ui-button primary" onClick={props.onRetry}>重试</button><button type="button" className="ui-button secondary" onClick={() => window.dispatchEvent(new CustomEvent("protovoy-open-settings"))}>检查 Local Agent 设置</button></div></div>;
  if (!tree) return <div className="workspace-tree-empty" role="status">尚未加载工作区</div>;
  const workspace = tree.workspaces[0];
  const workspaceProjects = tree.projects.filter((project) => project.workspaceId === workspace?.id);
  const collectionCountCache = new Map<string, number>();
  function collectionRequestCount(collectionId: string, ancestors = new Set<string>()): number {
    const cached = collectionCountCache.get(collectionId);
    if (cached != null) return cached;
    if (ancestors.has(collectionId)) return 0;
    const nextAncestors = new Set(ancestors).add(collectionId);
    const direct = tree!.requests.filter((request) => request.collectionId === collectionId && !request.envelope?.variables?.__protovoyCaseOf).length;
    const descendants = tree!.collections.filter((collection) => collection.parentId === collectionId).reduce((count, child) => count + collectionRequestCount(child.id, nextAncestors), 0);
    const total = direct + descendants;
    collectionCountCache.set(collectionId, total);
    return total;
  }
  const selectedProjectIds = [...new Set(tree.requests.filter((item) => selectedIds.includes(item.id)).map((item) => item.projectId))];
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const selectedProject = tree.projects.find((project) => project.id === props.selectedProjectId) ?? workspaceProjects[0];
  const storedProjectModules = selectedProject ? tree.modules?.filter((module) => module.projectId === selectedProject.id) : undefined;
  const projectModules = selectedProject ? (storedProjectModules?.length ? storedProjectModules : [{ id: `default-module-${selectedProject.id}`, projectId: selectedProject.id, name: "默认模块", isDefault: true }]) : [];
  const collectionParentOptions = selectedProject ? buildCollectionParentOptions(projectModules, tree.collections.filter((collection) => collection.projectId === selectedProject.id)) : [];
  const collectionInModule = (collection: CollectionRecord, module: ModuleRecord) => collection.moduleId ? collection.moduleId === module.id : module.isDefault;
  function collectionMatches(collection: CollectionRecord): boolean {
    if (!normalizedQuery) return true;
    if (collection.name.toLocaleLowerCase().includes(normalizedQuery)) return true;
    if (tree!.requests.some((item) => item.collectionId === collection.id && `${item.name} ${item.target}`.toLocaleLowerCase().includes(normalizedQuery))) return true;
    return tree!.collections.filter((item) => item.parentId === collection.id).some(collectionMatches);
  }
  function readDrag(event: DragEvent): { type: "request" | "collection"; id: string } | null {
    try {
      return JSON.parse(
        event.dataTransfer.getData("application/x-protovoy-item"),
      );
    } catch { return null; }
  }
  function setDrag(event: DragEvent, type: "request" | "collection", id: string) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("application/x-protovoy-item", JSON.stringify({ type, id }));
  }
  function dropOnCollection(event: DragEvent, target: CollectionRecord) {
    event.preventDefault();
    const item = readDrag(event);
    if (!item) return;
    if (item.type === "request") void props.onMoveRequest(item.id, target.projectId, target.id);
    else {
      const source = tree!.collections.find((collection) => collection.id === item.id);
      if (source && source.id !== target.id && source.projectId === target.projectId) {
        if ((source.parentId ?? null) === (target.parentId ?? null) && (source.moduleId ?? null) === (target.moduleId ?? null)) void props.onSwapCollections(source, target);
        else void props.onMoveCollection(source, target.projectId, target.id);
      }
    }
  }
  function toggleSelected(id: string) {
    setSelectedIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  }
  async function deleteRequest(id: string, name: string, showSuccess = true): Promise<boolean> {
    try {
      await props.onDeleteRequest(id);
      setSelectedIds((current) => current.filter((item) => item !== id));
      if (showSuccess) notify(`已删除请求“${name}”`, "success");
      return true;
    } catch (error) {
      notify(`删除请求失败：${error instanceof Error ? error.message : String(error)}`, "danger");
      return false;
    }
  }
  async function batchDelete() {
    if (!selectedIds.length || !(await confirm({ title: "删除请求", description: `确定删除选中的 ${selectedIds.length} 个请求吗？`, tone: "danger", confirmLabel: "删除" }))) return;
    const targets = tree!.requests.filter((item) => selectedIds.includes(item.id));
    let deleted = 0;
    for (const request of targets) if (await deleteRequest(request.id, request.name, false)) deleted += 1;
    if (deleted) notify(`已删除 ${deleted} 个请求`, "success");
    setBatchMode(false);
  }
  async function batchMove() {
    const target = tree!.collections.find((item) => item.id === batchTarget);
    if (!target) return;
    for (const id of selectedIds) await props.onMoveRequest(id, target.projectId, target.id);
    setSelectedIds([]);
    setBatchMode(false);
  }
  function renderCollection(collection: CollectionRecord, depth = 0) {
    const collectionRequests = tree!.requests.filter((item) => item.collectionId === collection.id);
    const requests = collectionRequests.filter((item) => !item.envelope?.variables?.__protovoyCaseOf).flatMap((request) => {
      const cases = collectionRequests.filter((item) => item.envelope?.variables?.__protovoyCaseOf === request.id && isDebugCase(item));
      return collapsedNodes.includes(`request:${request.id}`) ? [request] : [request, ...cases];
    });
    const siblings = tree!.collections.filter((item) => item.projectId === collection.projectId && (item.parentId ?? null) === (collection.parentId ?? null)).sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name));
    const siblingIndex = siblings.findIndex((item) => item.id === collection.id);
    const children = tree!.collections.filter((item) => item.parentId === collection.id).sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name));
    const isCollapsed = collapsedNodes.includes(`collection:${collection.id}`);
    return <div key={collection.id} className="tree-collection-node" data-depth={depth} role="treeitem" aria-level={depth + 1} aria-setsize={siblings.length} aria-posinset={siblingIndex + 1} aria-expanded={!isCollapsed} aria-label={`集合 ${collection.name}`}>
      <div draggable onDragStart={(event) => setDrag(event, "collection", collection.id)} onDragOver={(event) => event.preventDefault()} onDrop={(event) => dropOnCollection(event, collection)} className={`tree-row tree-collection-row${isCollectionActive(collection.id, props.selectedCollectionId, props.selectedRequestId) ? " is-active" : ""}${openMenuId === collection.id ? " is-menu-open" : ""}`} style={{ paddingLeft: 12 + depth * 20 }}>
        <button type="button" aria-label={`${isCollapsed ? "展开" : "折叠"}集合 ${collection.name}`} aria-expanded={!isCollapsed} className={`tree-chevron${isCollapsed ? "" : " is-expanded"}`} onClick={() => toggleExplorerNode(`collection:${collection.id}`)}><Icon name="chevron" /></button>
        <button type="button" className="tree-main tree-collection-main" onClick={() => props.onSelectCollection(collection.projectId, collection.id)} title={collection.name}>
          <Icon className="tree-collection-icon" name="folder" />
          <span className="tree-label">{collection.name}</span>
          <small className="tree-count" title={`${requests.length} 个直接请求，${collectionRequestCount(collection.id)} 个请求（含子集合）`}>{collectionRequestCount(collection.id)}</small>
        </button>
        <span className="tree-actions">
          <span className="tree-more" data-tree-menu={collection.id}>
            <button
              type="button"
              ref={(node) => { if (node) menuButtonRefs.current.set(collection.id, node); else menuButtonRefs.current.delete(collection.id); }}
              className="tree-action-button"
              title="更多操作"
              aria-expanded={openMenuId === collection.id}
              aria-haspopup="menu"
              onClick={(event) => { event.stopPropagation(); openCollectionMenu(collection.id); }}
            ><Icon name="menu" /></button>
            {renderCollectionMenu(collection, siblingIndex, siblings)}
          </span>
        </span>
      </div>
      {!isCollapsed && children.filter(collectionMatches).map((child) => renderCollection(child, depth + 1))}
      {!isCollapsed && !!collection.tags?.length && <div className="tree-tags" style={{ paddingLeft: 31 + depth * 20 }}>{collection.tags.map((tag) => <span key={tag}>{tag}</span>)}</div>}
      {!isCollapsed && requests.filter((request) => !normalizedQuery || `${request.name} ${request.target}`.toLocaleLowerCase().includes(normalizedQuery)).map((request, requestIndex, visibleRequests) => <div data-request-id={request.id} draggable={!batchMode} onDragStart={(event) => setDrag(event, "request", request.id)} className={`tree-row tree-request-row${request.envelope?.variables?.__protovoyCaseOf ? " tree-case-row" : ""}${batchMode ? " is-batch-mode" : ""}${selectedIds.includes(request.id) ? " is-selected" : ""}${props.selectedRequestId === request.id ? " is-active" : ""}${openMenuId === `request:${request.id}` ? " is-menu-open" : ""}`} role="treeitem" aria-selected={batchMode ? selectedIds.includes(request.id) : undefined} aria-level={depth + (request.envelope?.variables?.__protovoyCaseOf ? 3 : 2)} aria-setsize={visibleRequests.length} aria-posinset={requestIndex + 1} key={request.id} style={{ paddingLeft: (request.envelope?.variables?.__protovoyCaseOf ? 73 : 38) + depth * 20 }}>
        {!request.envelope?.variables?.__protovoyCaseOf ? (collectionRequests.some((item) => item.envelope?.variables?.__protovoyCaseOf === request.id && isDebugCase(item)) ? <span className="tree-actions-leading"><button type="button" className={`tree-chevron tree-request-chevron${collapsedNodes.includes(`request:${request.id}`) ? "" : " is-expanded"}`} aria-label={`${collapsedNodes.includes(`request:${request.id}`) ? "展开" : "收起"}调试用例 ${request.name}`} aria-expanded={!collapsedNodes.includes(`request:${request.id}`)} onClick={() => toggleExplorerNode(`request:${request.id}`)}><Icon name="chevron"/></button></span> : <span className="tree-actions-leading tree-chevron-placeholder" aria-hidden="true"><span className="tree-chevron tree-request-chevron"><Icon name="chevron"/></span></span>) : null}
        <button type="button" className="tree-main tree-request-main" onClick={() => { if (batchMode) { toggleSelected(request.id); return; } const parentId = request.envelope?.variables?.__protovoyCaseOf; const parentStructure = parentId ? readInterfaceStructureMetadata(collectionRequests.find((item) => item.id === parentId)?.envelope?.metadata) : null; if (parentStructure) stashCaseInterfaceStructure(request.id, parentStructure); props.onOpenRequest(request.id); }} title={batchMode ? (selectedIds.includes(request.id) ? "取消选择" : "选择此项") : request.target}>{request.envelope?.variables?.__protovoyCaseOf ? <span className="tree-case-icon" title="调试用例"><Icon name="bolt"/></span> : request.protocolId === "websocket" ? <span className="tree-protocol-icon tree-protocol-websocket" title="WebSocket"><Icon name="websocket" /></span> : <b className="tree-request-method" data-method={(request.method ?? request.protocolId ?? "HTTP").toUpperCase()}>{request.method ?? request.protocolId?.toUpperCase() ?? "HTTP"}</b>}<span className="tree-label">{request.name}</span>{!request.envelope?.variables?.__protovoyCaseOf && collectionRequests.some((item) => item.envelope?.variables?.__protovoyCaseOf === request.id && isDebugCase(item)) ? <small className="tree-interface-case-count" title="调试用例数量">（{collectionRequests.filter((item) => item.envelope?.variables?.__protovoyCaseOf === request.id && isDebugCase(item)).length}）</small> : null}</button>
        <span className="tree-actions tree-more" data-tree-menu={`request:${request.id}`}><button type="button" className="tree-action-button" ref={(node) => { const id = `request:${request.id}`; if (node) menuButtonRefs.current.set(id, node); else menuButtonRefs.current.delete(id); }} title="更多操作" aria-label={`${request.name} 更多操作`} aria-expanded={openMenuId === `request:${request.id}`} aria-haspopup="menu" onClick={(event) => { event.stopPropagation(); openCollectionMenu(`request:${request.id}`); }}><Icon name="more" /></button>{renderRequestMenu(request)}</span>
      </div>)}
    </div>;
  }
  return <section className="workspace-explorer-tree" role="tree" aria-label="工作区资源树" aria-keyshortcuts="ArrowUp ArrowDown ArrowLeft ArrowRight Home End" tabIndex={0} onKeyDown={onTreeKeyDown}>
    <div className="workspace-tree-heading">
      <span>接口管理</span>
      <input ref={importInput} hidden multiple type="file" accept=".json,.yaml,.yml,.har,.protovoy" onChange={(event) => void importFiles(event.target.files)} />
    </div>
    <div ref={createMenuRef} className="workspace-quick-actions">
      <div className="workspace-search-row">
        <span className="workspace-search-field"><Icon name="search"/><input aria-label="搜索接口" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索接口、集合或 URL" /></span>
        <button type="button" className={`workspace-batch-trigger${batchMode ? " is-active" : ""}`} aria-pressed={batchMode} title={batchMode ? "退出多选" : "进入多选"} onClick={() => { setBatchMode((value) => !value); setSelectedIds([]); setBatchTarget(""); }}>多选</button>
        <button ref={createMenuButtonRef} className="workspace-create-trigger" type="button" aria-label="新建资源" title="新建资源" aria-haspopup="menu" aria-expanded={createMenuOpen} onClick={() => setCreateMenuOpen((value) => { if (!value) setMoreProtocolsOpen(false); return !value; })}><Icon name="plus" /></button>
      </div>
      {createMenuOpen && createMenuPos && typeof document !== "undefined" ? createPortal(<div className="workspace-protocol-menu" data-workspace-create-menu data-expanded={moreProtocolsOpen ? "true" : "false"} role="menu" aria-label="选择接口协议" style={{ top: createMenuPos.top, left: createMenuPos.left, width: createMenuPos.width }}>
        <div className="workspace-create-menu-title">新建接口</div>
        <div className="workspace-create-menu-grid">{PRIMARY_CREATE_PROTOCOLS.map(([id, label, icon]) => <button key={id} type="button" role="menuitem" onClick={() => { setCreateMenuOpen(false); window.dispatchEvent(new CustomEvent("protovoy-create-workbench", { detail: id })); }}><Icon name={icon} /><span>{label === "HTTP" ? "HTTP 接口" : label}</span></button>)}{moreProtocolsOpen ? MORE_CREATE_PROTOCOLS.map(([id, label, icon]) => <button key={id} type="button" role="menuitem" onClick={() => { setCreateMenuOpen(false); window.dispatchEvent(new CustomEvent("protovoy-create-workbench", { detail: id })); }}><Icon name={icon} /><span>{label}</span></button>) : <button type="button" className="workspace-create-menu-more" role="menuitem" aria-expanded="false" onClick={() => setMoreProtocolsOpen(true)}><Icon name="more"/><span>更多…</span></button>}</div>
        <div className="workspace-create-menu-divider"/>
        <div className="workspace-create-menu-title">项目资源</div>
        <div className="workspace-create-menu-grid">
          <button type="button" role="menuitem" disabled={!selectedProject || !props.onCreateModule} onClick={() => { if (selectedProject) setDraft({ kind: "module", owner: selectedProject.id }); setCreateMenuOpen(false); }}><Icon name="module"/><span>新建模块</span></button>
          <button type="button" role="menuitem" disabled={!selectedProject || !projectModules.length} onClick={() => { if (selectedProject && projectModules[0]) setDraft({ kind: "collection", owner: selectedProject.id, parentId: null, moduleId: projectModules[0].isDefault && !storedProjectModules?.length ? undefined : projectModules[0].id }); setCreateMenuOpen(false); }}><Icon name="folder"/><span>新建目录</span></button>
          <button type="button" role="menuitem" onClick={() => { setCreateMenuOpen(false); window.dispatchEvent(new CustomEvent("protovoy-open-script-library")); }}><Icon name="code"/><span>脚本库</span></button>
        </div>
        <div className="workspace-create-menu-divider"/>
        <div className="workspace-create-menu-title">其他</div>
        <div className="workspace-create-menu-grid">
          <button type="button" role="menuitem" onClick={() => { setCreateMenuOpen(false); importInput.current?.click(); }}><Icon name="download"/><span>导入文件</span></button>
          <button type="button" role="menuitem" onClick={() => { setCreateMenuOpen(false); window.dispatchEvent(new CustomEvent("protovoy-open-curl-import", { detail: { projectId: selectedProject?.id, collectionId: props.selectedCollectionId ?? undefined } })); }}><Icon name="code"/><span>导入 cURL</span></button>
        </div>
      </div>, document.body) : null}
    </div>
    {batchMode && <div className="workspace-batch-bar">
      <strong>{selectedIds.length} 已选</strong>
      <Select aria-label="批量移动目标集合" value={batchTarget} onChange={(event) => setBatchTarget(event.target.value)} disabled={!selectedIds.length || selectedProjectIds.length !== 1}><option value="">{!selectedIds.length ? "点击资源选择" : selectedProjectIds.length === 1 ? "移动到…" : "跨项目不可批量移动"}</option>{tree.collections.filter((item) => item.projectId === selectedProjectIds[0]).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</Select>
      <Button size="compact" disabled={!batchTarget} onClick={() => void batchMove()}>移动</Button>
      <Button size="compact" variant="danger" disabled={!selectedIds.length} onClick={() => void batchDelete()}>删除</Button>
      <Button size="compact" variant="ghost" onClick={() => { setBatchMode(false); setSelectedIds([]); setBatchTarget(""); }}>取消</Button>
    </div>}
    {selectedProject ? projectModules.map((module) => {
      const moduleCollapsed = collapsedNodes.includes(`module:${module.id}`);
      const moduleCollectionIds = tree.collections
        .filter((collection) => collection.projectId === module.projectId && collectionInModule(collection, module))
        .map((collection) => collection.id);
      const allCollectionsExpanded = !moduleCollapsed && moduleCollectionIds.every((id) => !collapsedNodes.includes(`collection:${id}`));
      const toggleAllLabel = allCollectionsExpanded ? "收起" : "展开";
      return <div key={module.id} className="tree-module" role="treeitem" aria-expanded={!moduleCollapsed} aria-label={`模块 ${module.name}`}>
      <div className={`tree-row tree-module-row${openMenuId === `module:${module.id}:menu` ? " is-menu-open" : ""}`}>
        <Icon className="tree-module-icon" name="module"/>
        <strong className="tree-label">{module.name}</strong>
        <button type="button" className={`tree-action-button tree-module-chevron${moduleCollapsed ? "" : " is-expanded"}`} aria-label={`${moduleCollapsed ? "展开" : "折叠"}模块 ${module.name}`} aria-expanded={!moduleCollapsed} onClick={() => toggleExplorerNode(`module:${module.id}`)}><Icon name="triangle"/></button>
        <span className="tree-actions tree-module-actions">
          <button
            type="button"
            className="tree-action-button tree-module-toggle-all"
            disabled={!moduleCollectionIds.length}
            title={`全部${toggleAllLabel}`}
            aria-label={`${toggleAllLabel}模块 ${module.name} 的全部目录`}
            onClick={() => useAppStore.setState({ collapsedExplorerNodes: setModuleCollectionsCollapsed(collapsedNodes, module.id, moduleCollectionIds, allCollectionsExpanded) })}
          ><Icon name={allCollectionsExpanded ? "collapse-all" : "expand-all"}/></button>
          <span className="tree-more" data-tree-menu={`module:${module.id}:menu`}>
            <button
              type="button"
              className="tree-action-button"
              ref={(node) => { const id = `module:${module.id}:menu`; if (node) menuButtonRefs.current.set(id, node); else menuButtonRefs.current.delete(id); }}
              title="更多操作"
              aria-label={`${module.name} 模块更多操作`}
              aria-expanded={openMenuId === `module:${module.id}:menu`}
              aria-haspopup="menu"
              onClick={(event) => { event.stopPropagation(); openCollectionMenu(`module:${module.id}:menu`); }}
            ><Icon name="more"/></button>
            {renderModuleMenu(module)}
          </span>
        </span>
      </div>
      {!moduleCollapsed ? <div className="tree-module-content">

        {tree.collections.filter((item) => item.projectId === selectedProject.id && collectionInModule(item, module) && !item.parentId && collectionMatches(item)).sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name)).map((collection) => renderCollection(collection, 1))}


      </div> : null}
    </div>; }) : null}
    {draft?.kind === "module" && typeof document !== "undefined" ? createPortal(<ModalFrame open onClose={() => setDraft(null)} closeOnBackdrop={!name.trim()} closeOnEscape={!name.trim()} className="collection-create-dialog" ariaLabelledBy="module-create-title"><header><div><h2 id="module-create-title">新建模块</h2><p>模块用于划分独立业务域，可包含多个目录。</p></div><IconButton label="关闭" icon="close" onClick={() => setDraft(null)} /></header><label><span>模块名称</span><input autoFocus value={name} onChange={(event) => setName(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void submit(); }} placeholder="例如：用户中心"/></label><footer><Button variant="secondary" onClick={() => setDraft(null)}>取消</Button><Button variant="primary" disabled={!name.trim() || !props.onCreateModule} onClick={() => void submit()}>创建模块</Button></footer></ModalFrame>, document.body) : null}
    {draft?.kind === "collection" && typeof document !== "undefined" ? createPortal(<ModalFrame open onClose={() => setDraft(null)} closeOnBackdrop={!name.trim()} closeOnEscape={!name.trim()} className="collection-create-dialog" ariaLabelledBy="collection-create-title"><header><div><h2 id="collection-create-title">新建目录</h2><p>选择模块或已有目录作为父级。</p></div><IconButton label="关闭" icon="close" onClick={() => setDraft(null)} /></header><label><span>目录名称</span><input autoFocus value={name} onChange={(event) => setName(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void submit(); }} placeholder="请输入目录名称"/></label><CollectionParentPicker options={collectionParentOptions} value={draft.parentId ? `collection:${draft.parentId}` : `module:${draft.moduleId ?? projectModules.find((module) => module.isDefault)?.id ?? projectModules[0]?.id ?? ""}`} onChange={(option) => setDraft({ ...draft, parentId: option.kind === "collection" ? option.id : null, moduleId: option.moduleId })}/><footer><Button variant="secondary" onClick={() => { setDraft(null); setName(""); }}>取消</Button><Button variant="primary" disabled={!name.trim() || !collectionParentOptions.length} onClick={() => void submit()}>创建目录</Button></footer></ModalFrame>, document.body) : null}
  </section>;
}
