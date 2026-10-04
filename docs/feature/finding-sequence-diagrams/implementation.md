# Finding sequence diagrams

## F0 intake / workflow gate
User explicitly asks to add issue sequence diagrams to the project; prior main push/local port1234 deployment authorization remains. Clean main at f0bcd6b. Phase P8 backend integration with existing finding/result JSON, Eino, React detail and verification tests. Missing contracts can be safely inferred below. Implementation allowed after design. Feature branch codex/finding-sequence-diagrams.
Scope: conditional generation for findings, code-citation validation, risk highlighting, inference labels, diagram source/image export. No language adapter, runtime execution or additional external diagram service. Required QA: empty/legacy/error states, citation rejection, model integration and browser desktop/mobile.
Maintainability: types/agent/detail under800 lines; add separate diagram validation/generation and presentational modules, no broad refactor.

## F1 requirements (user-authorized)
R1: Each new validated finding automatically receives a sequence diagram attempt; zero findings means zero diagram model calls.
R2: Risk steps highlight where the issue occurs, participants/order/conditions make the scenario legible, uncertainty is explicit.
R3: Every supplied citation matches pinned base/head file/line/snippet; base references annotate deleted before-change evidence rather than falsely depicting current execution.
R4: Frontend displays SVG with evidence inspection, Mermaid source and SVG download; bounded rendering and old-result compatibility.
R5: Diagram failure preserves primary findings, with unavailable/partial reasons and model token observations; settings persisted to config.yaml. Tests, main and existing local deploy.

## F2 consumer contract / design
Finding gains optional sequence_diagram {status: ready|partial|unavailable, reason, participants:[{id,label}], steps:[{from,to,label,kind:call|return|note,certainty:cited|inferred,risk,evidence:[{side,file,line,snippet,sha}]}], limitations, mermaid}. Primary audit JSON stays unchanged: model never supplies final SVG or directives. Separate generation after primary finding validation uses Eino read tools and captured config, <=8 steps/20s per finding and <=90s combined, preserving 5s of parent deadline. Shared tool budget remains. Generation defaults enabled, configurable in system settings; disabled findings explicitly unavailable. Old findings have no diagram; UI suggests re-audit.
Diagram inputs strictly parsed, <=32KiB, 2–8 participants, 1–16 steps, bounded labels/citations/limitations. Participant ids restricted; every citation validated on snapshot, SHA assigned by backend. A risk step must anchor the primary changed-line finding. Cited steps require references; inferred relationships retain dashed/explicit labels. BASE evidence must be note-kind, clearly shown as before-change/deleted. Validation confirms snippets, not call semantics or exploitability.
Backend derives status and safe Mermaid from structured data; model cannot inject generated source/status. Frontend renders native React SVG elements, no innerHTML/raw SVG/third-party rendering service; safe text nodes and bounded layout. Evidence selection is keyboard accessible, wide diagrams scroll, export serializes only the owned SVG. Mermaid is source export generated with escaped labels/known participants, not browser-executed model text. Syntax reference: https://mermaid.js.org/syntax/sequenceDiagram.html.
Existing SQLite result JSON is additive, no table migration. Policy key increments to prevent completed old audits suppressing new diagrams. Diagram failure is supplemental and does not downgrade primary successful audit; cancellation still follows worker lifecycle.
