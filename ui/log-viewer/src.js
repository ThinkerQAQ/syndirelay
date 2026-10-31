import { EditorState, RangeSetBuilder, StateField } from "@codemirror/state";
import {
  Decoration, EditorView, drawSelection, keymap, lineNumbers
} from "@codemirror/view";
import { openSearchPanel, search, searchKeymap } from "@codemirror/search";

const logSeverity = StateField.define({
  create(state) {
    return severityRanges(state.doc);
  },
  update(value, transaction) {
    return transaction.docChanged ? severityRanges(transaction.state.doc) : value;
  },
  provide(field) {
    return EditorView.decorations.from(field);
  },
});
function severityRanges(doc) {
  const builder = new RangeSetBuilder();
  for (let n = 1; n <= doc.lines; n++) {
    const line = doc.line(n);
    const match = /^\[[^\]]+\]\s+(DEBUG|INFO|WARN|ERROR)\b/.exec(line.text);
    if (match) {
      builder.add(line.from, line.from, Decoration.line({
        attributes: { class: "blogctl-log-" + match[1].toLowerCase() },
      }));
    }
  }
  return builder.finish();
}

const theme = EditorView.theme({
  "&": {
    height: "440px",
    borderRadius: "8px",
    backgroundColor: "#111827",
    color: "#e5e7eb",
    fontSize: "12px",
    lineHeight: "1.75",
  },
  ".cm-content": {
    fontFamily: 'Consolas, "Cascadia Code", "SFMono-Regular", monospace',
    padding: "10px 0",
    caretColor: "#cbd5e1",
  },
  ".cm-line": { padding: "0 12px" },
  ".cm-gutters": {
    backgroundColor: "#0b1220",
    color: "#64748b",
    borderRight: "1px solid #334155",
  },
  ".cm-activeLineGutter": { backgroundColor: "#17253e" },
  ".cm-scroller": { overflow: "auto" },
  ".cm-search": { backgroundColor: "#263449", color: "#fff" },
  ".cm-searchMatch": { backgroundColor: "#945a24" },
  ".cm-searchMatch-selected": { backgroundColor: "#c47634" },
  ".blogctl-log-error": { color: "#fda4af" },
  ".blogctl-log-warn": { color: "#fdba74" },
  ".blogctl-log-info": { color: "#bfdbfe" },
  ".blogctl-log-debug": { color: "#94a3b8" },
});

export function create(parent) {
  const view = new EditorView({
    parent,
    doc: "暂无日志",
    extensions: [
      lineNumbers(),
      // Web Console keeps strict CSP; nonce is fresh for every Go-served
      // document. Extension pages do not need the nonce facet.
      EditorView.cspNonce.of(document.querySelector('meta[name="blogctl-style-nonce"]')?.content || ""),
      drawSelection(),
      EditorState.readOnly.of(true),
      EditorView.editable.of(false),
      EditorView.contentAttributes.of({ tabindex: "0", role: "log", "aria-label": "BlogCTL 运行日志" }),
      EditorView.lineWrapping,
      keymap.of(searchKeymap),
      search({ top: true }),
      logSeverity,
      theme,
    ],
  });
  return {
    setText(text, follow = true) {
      const previous = view.state.doc.toString();
      if (previous === text) return;
      const nearBottom = view.scrollDOM.scrollHeight - view.scrollDOM.scrollTop -
        view.scrollDOM.clientHeight < 40;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
      if (follow && nearBottom) view.scrollDOM.scrollTop = view.scrollDOM.scrollHeight;
    },
    getText() { return view.state.doc.toString(); },
    selectedText() {
      const { from, to } = view.state.selection.main;
      return view.state.doc.sliceString(from, to);
    },
    selectAll() {
      view.dispatch({ selection: { anchor: 0, head: view.state.doc.length } });
      view.focus();
    },
    find() { view.focus(); openSearchPanel(view); },
    destroy() { view.destroy(); },
    get element() { return view.dom; },
  };
}
