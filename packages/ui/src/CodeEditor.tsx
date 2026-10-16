import { forwardRef, lazy, Suspense, useEffect, useImperativeHandle, useRef, useState, type ReactNode } from "react";
import type { editor } from "monaco-editor";
import { useAppStore } from "./appStore";

type MonacoEditorModule = typeof import("./MonacoEditorImpl");
let monacoEditorModulePromise: Promise<MonacoEditorModule> | undefined;
let preloadedMonacoEditor: MonacoEditorModule["default"] | undefined;

function loadMonacoEditor(): Promise<MonacoEditorModule> {
  monacoEditorModulePromise ??= import("./MonacoEditorImpl").then((module) => {
    preloadedMonacoEditor = module.default;
    return module;
  }).catch((error) => {
    monacoEditorModulePromise = undefined;
    throw error;
  });
  return monacoEditorModulePromise;
}

const LazyMonacoEditor = lazy(loadMonacoEditor);

export async function preloadCodeEditor(): Promise<void> {
  await loadMonacoEditor();
}

export interface CodeEditorProps {
  value: string;
  onChange: (value: string) => void;
  language?: string;
  height?: number | string;
  readOnly?: boolean;
  bare?: boolean;
  wordWrap?: boolean;
  revealLine?: number;
  findTrigger?: number;
}

export interface CodeEditorHandle {
  openFind: () => void;
}

function highlightFallbackLine(line: string, language: string, dark: boolean): ReactNode {
  if (language !== "json" && language !== "jsonc") return line;
  const colors = dark
    ? { key: "#9CDCFE", string: "#CE9178", number: "#B5CEA8", literal: "#C586C0", delimiter: "#D4D4D4" }
    : { key: "#005FB8", string: "#A4470A", number: "#16736B", literal: "#8B3F9B", delimiter: "#333333" };
  const tokens: ReactNode[] = [];
  const pattern = /"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|\b(?:true|false|null)\b|[{}\[\],:]/g;
  let offset = 0;
  for (const match of line.matchAll(pattern)) {
    const index = match.index ?? 0;
    if (index > offset) tokens.push(line.slice(offset, index));
    const token = match[0];
    const isString = token.startsWith('"');
    const isKey = isString && line.slice(index + token.length).trimStart().startsWith(":");
    const color = isKey ? colors.key : isString ? colors.string : /^-?\d/.test(token) ? colors.number : /^(true|false|null)$/.test(token) ? colors.literal : colors.delimiter;
    tokens.push(<span key={`${index}:${token}`} style={{ color }}>{token}</span>);
    offset = index + token.length;
  }
  if (offset < line.length) tokens.push(line.slice(offset));
  return tokens;
}

function CodeEditorFallback({ value, readOnly, onChange, language, theme, wordWrap }: Pick<CodeEditorProps, "value" | "readOnly" | "onChange" | "language" | "wordWrap"> & { theme: string }) {
  const dark = theme === "protovoy-dark";
  const lines = value.split("\n");
  const highlightedContentRef = useRef<HTMLDivElement | null>(null);
  const highlightedContent = <div
      ref={highlightedContentRef}
      className="code-editor-fallback"
      role={readOnly ? "textbox" : undefined}
      aria-label={readOnly ? "代码编辑器正在加载" : undefined}
      aria-readonly={readOnly ? "true" : undefined}
      aria-hidden={readOnly ? undefined : "true"}
      style={{ boxSizing: "border-box", width: "100%", height: "100%", overflow: "auto", padding: "6px 0 10px", background: "var(--protovoy-bg-elevated)", color: dark ? "#D4D4D4" : "#172033", font: "12px/19px var(--protovoy-mono)" }}
    >
      {lines.map((line, index) => <div className="code-editor-fallback-line" key={index} style={{ display: "flex", minHeight: 19 }}>
        <span className="code-editor-fallback-line-number" aria-hidden="true" style={{ width: 34, flex: "0 0 34px", boxSizing: "border-box", paddingRight: 12, color: dark ? "#858585" : "#6E7681", textAlign: "right", userSelect: "none" }}>{index + 1}</span>
        <code style={{ minWidth: 0, flex: 1, paddingLeft: 14, paddingRight: 12, color: "inherit", font: "inherit", whiteSpace: wordWrap ? "pre-wrap" : "pre", overflowWrap: wordWrap ? "anywhere" : "normal" }}>{highlightFallbackLine(line, language ?? "plaintext", dark)}</code>
      </div>)}
    </div>;
  if (readOnly) return highlightedContent;
  return <div style={{ position: "relative", width: "100%", height: "100%" }}>
    {highlightedContent}
    <textarea
      className="code-editor-fallback-input"
      aria-label="代码编辑器正在加载"
      value={value}
      onChange={(event) => onChange(event.target.value)}
      onScroll={(event) => {
        if (!highlightedContentRef.current) return;
        highlightedContentRef.current.scrollTop = event.currentTarget.scrollTop;
        highlightedContentRef.current.scrollLeft = event.currentTarget.scrollLeft;
      }}
      spellCheck={false}
      style={{ position: "absolute", inset: 0, boxSizing: "border-box", width: "100%", height: "100%", resize: "none", margin: 0, border: 0, outline: 0, padding: "6px 12px 10px 48px", overflow: "auto", background: "transparent", color: "transparent", WebkitTextFillColor: "transparent", caretColor: dark ? "#D4D4D4" : "#172033", font: "12px/19px var(--protovoy-mono)", whiteSpace: wordWrap ? "pre-wrap" : "pre", overflowWrap: wordWrap ? "anywhere" : "normal" }}
    />
  </div>;
}

function useResolvedEditorTheme() {
  const themeMode = useAppStore((state) => state.themeMode);
  const [systemDark, setSystemDark] = useState(() => typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  useEffect(() => {
    if (themeMode !== "system") return;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const update = () => setSystemDark(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [themeMode]);
  return themeMode === "dark" || (themeMode === "system" && systemDark) ? "protovoy-dark" : "protovoy-light";
}

export const CodeEditor = forwardRef<CodeEditorHandle, CodeEditorProps>(function CodeEditor({ value, onChange, language = "plaintext", height = 180, readOnly = false, bare = false, wordWrap = true, revealLine, findTrigger }, ref) {
  const canLoadEditor = typeof window !== "undefined";
  const hasPreloadedEditor = Boolean(preloadedMonacoEditor);
  const MonacoEditor = preloadedMonacoEditor ?? LazyMonacoEditor;
  const editorTheme = useResolvedEditorTheme();
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const readyFramesRef = useRef<number[]>([]);
  const [editorReady, setEditorReady] = useState(false);
  const handledFindTriggerRef = useRef(0);
  const runFind = () => {
    const editorInstance = editorRef.current;
    if (!editorInstance) return;
    editorInstance.focus();
    const action = editorInstance.getAction("actions.find");
    if (action) void action.run();
    editorInstance.trigger("protovoy-toolbar", "editor.action.startFindReplaceAction", {});
    const findController = editorInstance.getContribution("editor.contrib.findController") as { start?: (options: { forceRevealReplace: boolean; seedSearchStringFromSelection: boolean }) => void } | null;
    findController?.start?.({ forceRevealReplace: false, seedSearchStringFromSelection: false });
  };
  useImperativeHandle(ref, () => ({ openFind: runFind }), []);
  const openFind = (trigger = findTrigger ?? 0) => {
    const editorInstance = editorRef.current;
    if (!editorInstance || !trigger || trigger === handledFindTriggerRef.current) return;
    handledFindTriggerRef.current = trigger;
    runFind();
  };
  useEffect(() => { if (revealLine) { editorRef.current?.revealLineInCenter(revealLine); editorRef.current?.setPosition({ lineNumber: revealLine, column: 1 }); } }, [revealLine]);
  useEffect(() => { openFind(); }, [findTrigger]);
  useEffect(() => () => { readyFramesRef.current.forEach((frame) => cancelAnimationFrame(frame)); }, []);
  const fallback = <CodeEditorFallback value={value} onChange={onChange} readOnly={readOnly} language={language} theme={editorTheme} wordWrap={wordWrap}/>;
  return <div className={bare ? "code-editor code-editor-bare" : "code-editor"} style={{ position: "relative", height, marginTop: bare ? 0 : 6, overflow: "hidden", border: bare ? 0 : "1px solid var(--protovoy-border)", borderRadius: bare ? 0 : 9, background: "var(--protovoy-bg-elevated)" }}>
    {canLoadEditor ? <div aria-hidden={!editorReady} style={{ height: "100%", visibility: editorReady || hasPreloadedEditor ? "visible" : "hidden" }}>
      <Suspense fallback={null}>
        <MonacoEditor
          height="100%"
          language={language}
          value={value}
          loading={null}
          onMount={(editorInstance) => {
            editorRef.current = editorInstance;
            openFind();
            editorInstance.layout();
            editorInstance.render(true);
            readyFramesRef.current.forEach((frame) => cancelAnimationFrame(frame));
            const firstFrame = requestAnimationFrame(() => {
              editorInstance.render(true);
              const secondFrame = requestAnimationFrame(() => setEditorReady(true));
              readyFramesRef.current = [secondFrame];
            });
            readyFramesRef.current = [firstFrame];
          }}
          onChange={(next) => onChange(next ?? "")}
          theme={editorTheme}
          options={{ automaticLayout: true, minimap: { enabled: false }, wordWrap: wordWrap ? "on" : "off", scrollBeyondLastLine: false, fontSize: 12, lineHeight: 19, tabSize: 2, readOnly, folding: true, showFoldingControls: "always", lineNumbersMinChars: 3, glyphMargin: false, padding: { top: 6, bottom: 10 }, renderLineHighlight: "line", overviewRulerBorder: false, scrollbar: { handleMouseWheel: true, alwaysConsumeMouseWheel: false } }}
        />
      </Suspense>
    </div> : null}
    {!editorReady && !hasPreloadedEditor ? <div style={{ position: "absolute", inset: 0, zIndex: 1 }}>{fallback}</div> : null}
  </div>;
});
