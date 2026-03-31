---
name: summarize
description: Summarize text or documents concisely
when_to_use: When asked to summarize, condense, or get key points from content
arguments:
  - length
argument-hint: "summarize [--arg length=short|medium|detailed]"
---

# Summarize

Create a ${length:-medium} summary of the following content.

Focus on:
- Key points and main arguments
- Important facts and figures
- Actionable conclusions

${USER_INPUT}
