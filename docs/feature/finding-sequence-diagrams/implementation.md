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
