// CodeMirror 6 editor of the playground. Loaded dynamically in the browser only: CodeMirror touches
// DOM globals when imported, which would break the server-side rendering done by VitePress.
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { yaml } from "@codemirror/lang-yaml";
import {
  HighlightStyle,
  bracketMatching,
  indentOnInput,
  syntaxHighlighting,
} from "@codemirror/language";
import { type Diagnostic, lintGutter, setDiagnostics } from "@codemirror/lint";
import { Compartment, EditorState } from "@codemirror/state";
import {
  EditorView,
  drawSelection,
  highlightActiveLine,
  highlightActiveLineGutter,
  keymap,
  lineNumbers,
} from "@codemirror/view";
import { tags as t } from "@lezer/highlight";
import type { LintError } from "./wasm";

// Colors come from CSS variables defined in Playground.vue so that the theme follows the site theme.
const highlight = HighlightStyle.define([
  { tag: t.comment, color: "var(--jal-comment)", fontStyle: "italic" },
  { tag: [t.string, t.special(t.string)], color: "var(--jal-string)" },
  { tag: [t.number, t.bool, t.null, t.atom], color: "var(--jal-number)" },
  { tag: [t.propertyName, t.definition(t.propertyName)], color: "var(--jal-key)" },
  { tag: [t.labelName, t.typeName, t.meta], color: "var(--jal-meta)" },
  { tag: [t.keyword, t.operator], color: "var(--jal-keyword)" },
  { tag: [t.separator, t.punctuation, t.squareBracket, t.brace], color: "var(--jal-punct)" },
]);

const theme = EditorView.theme({
  "&": {
    color: "var(--vp-c-text-1)",
    backgroundColor: "var(--vp-c-bg)",
    fontSize: "14px",
    minHeight: "420px",
  },
  "&.cm-focused": { outline: "2px solid var(--vp-c-brand-1)", outlineOffset: "-1px" },
  ".cm-scroller": { fontFamily: "var(--vp-font-family-mono)", lineHeight: "1.55" },
  ".cm-content": { caretColor: "var(--vp-c-text-1)", padding: "8px 0" },
  "&.cm-focused .cm-cursor, .cm-cursor": { borderLeftColor: "var(--vp-c-text-1)" },
  ".cm-gutters": {
    backgroundColor: "var(--vp-c-bg-alt)",
    color: "var(--vp-c-text-3)",
    border: "none",
    borderRight: "1px solid var(--vp-c-divider)",
  },
  ".cm-activeLine": { backgroundColor: "var(--jal-active-line)" },
  ".cm-activeLineGutter": { backgroundColor: "var(--jal-active-line)", color: "var(--vp-c-text-1)" },
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, ::selection":
    { backgroundColor: "var(--jal-selection)" },
  ".cm-tooltip": {
    backgroundColor: "var(--vp-c-bg-elv)",
    color: "var(--vp-c-text-1)",
    border: "1px solid var(--vp-c-divider)",
    borderRadius: "6px",
  },
  ".cm-diagnostic": { whiteSpace: "pre-wrap" },
});

export interface EditorOptions {
  parent: HTMLElement;
  doc: string;
  dark: boolean;
  /** Called on every document change. `paste` is true when the change was a paste. */
  onChange(paste: boolean): void;
}

export interface Editor {
  getValue(): string;
  setValue(doc: string): void;
  setDark(dark: boolean): void;
  setErrors(errors: LintError[]): void;
  clearErrors(): void;
  goTo(line: number, column: number): void;
  destroy(): void;
}

export function createEditor(opts: EditorOptions): Editor {
  const darkCompartment = new Compartment();

  const view = new EditorView({
    parent: opts.parent,
    state: EditorState.create({
      doc: opts.doc,
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        lintGutter(),
        history(),
        drawSelection(),
        indentOnInput(),
        bracketMatching(),
        highlightActiveLine(),
        EditorView.lineWrapping,
        keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
        yaml(),
        syntaxHighlighting(highlight),
        theme,
        darkCompartment.of(EditorView.darkTheme.of(opts.dark)),
        EditorView.contentAttributes.of({ "aria-label": "Workflow YAML" }),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) {
            opts.onChange(u.transactions.some((tr) => tr.isUserEvent("input.paste")));
          }
        }),
      ],
    }),
  });

  function position(line: number, column: number): number {
    const l = view.state.doc.line(Math.min(Math.max(line, 1), view.state.doc.lines));
    return l.from + Math.min(Math.max(column - 1, 0), l.length);
  }

  return {
    getValue: () => view.state.doc.toString(),
    setValue(doc) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: doc } });
    },
    setDark(dark) {
      view.dispatch({ effects: darkCompartment.reconfigure(EditorView.darkTheme.of(dark)) });
    },
    setErrors(errors) {
      const diagnostics: Diagnostic[] = errors.map((e) => {
        const from = position(e.line, e.column);
        const word = view.state.wordAt(from);
        const to = word && word.from <= from ? word.to : Math.min(from + 1, view.state.doc.length);
        return { from, to: Math.max(to, from), severity: "error", message: e.message, source: e.kind };
      });
      view.dispatch(setDiagnostics(view.state, diagnostics));
    },
    clearErrors() {
      view.dispatch(setDiagnostics(view.state, []));
    },
    goTo(line, column) {
      const pos = position(line, column);
      view.dispatch({ selection: { anchor: pos }, effects: EditorView.scrollIntoView(pos, { y: "center" }) });
      view.focus();
    },
    destroy: () => view.destroy(),
  };
}
