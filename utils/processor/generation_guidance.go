package processor

// QMDGenerationGuidance keeps generated retrieval steps useful without
// carrying project-specific examples from one generation into another.
const QMDGenerationGuidance = `QMD CONTEXT RETRIEVAL:
Use qmd only when the request needs existing local documentation, source code, or prior project context. Build every retrieval query solely from concepts named in the user's request. Never reuse project names, collection names, architecture details, or search terms from an earlier generation.

Prefer a qmd_search workflow step when the workflow needs retrieved context. Set its collection only when the user explicitly supplies one; otherwise search the available collections without a restriction.

If a shell command is required, use neutral, request-derived queries such as:
- qmd query "<feature> architecture and boundaries" -c <collection>
- qmd query "<feature> lifecycle, errors, and idempotency" -c <collection>
- qmd search "<feature>" -c <collection>

Replace placeholders only with values from the user's request. If no collection is named, omit the -c flag. Do not invent a collection name or include examples from unrelated projects.`
