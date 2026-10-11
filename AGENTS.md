# Tynel development rules

Tynel is a learning-oriented TypeScript-to-native compiler using a fork of the Go compiler and the Go runtime.

- Read root `SPEC.md` as the project-wide technical specification. README, roadmap and notes summarize it, not override it. Record design decisions in ADRs and actual verification in `docs/progress.md`.
- Each learning chapter has exactly one development stage, one local commit and one annotated `stage/NN-topic` tag. Keep chapters and stages consecutively numbered in the same order. Include the article, code, fixtures, lock and validation record in the tagged commit. The first stage is a root commit; later stages follow the preceding stage. Follow `docs/reproducing.md` and `replay.json`.
- Concept definitions live in `docs/reference/concepts.md`. Reference tables, glossary updates and internal GC/ABI probes do not receive separate stage commits or tags; include them with the chapter they support.
- Finish authorized stages with implementation, runnable examples, positive and negative validation, article and result record, review, local commit and annotated tag. Remote push, release and remote history rewriting require a separate request.
- Before drafting or revising any learning note, read and follow `docs/writing-guide.md`. Follow one runnable experiment from input through key implementation steps to observed output. Introduce only concepts needed for the next step; move reference material and full verification details out of the narrative. Keep examples and results adjacent, explain behavior positively, and state limits once. Do not invent execution results, personal experiences, or dates. Keep editorial/history operations out of the articles.
- Preserve user work. Test-only features remain explicitly gated and are never advertised as general source-language support.
- Never generate business `.go` source or embed a JavaScript engine as the compilation path.
- Keep the host Go installation unchanged. Toolchains, experiments and generated output stay inside this repository.
- `compiler/nir` has no dependency on the TypeScript checker or Go compiler internals. Access Go compiler internals only through the bridge in the legal `cmd/compile` tree.
- Preserve binary64 behavior, object identity and explicit unsupported-feature diagnostics. Reject unsupported operations rather than shipping runtime capability traps.
- Read and SHA-guard edits; run tests and inspect changes. Distinguish planned design, implemented code and executed verification. Document pinned upstream versions and limits; never claim full TypeScript, Node or Go ABI compatibility.
