# Workflow
- Listen and fully understand what the user wants to discuss before suggesting next steps or jumping into work. Confidence: 0.80
- Before implementing, audit the repository to determine what is real, active, broken, duplicated, mock, and stale. Confidence: 0.85
- Do NOT add major features, invent architecture, redesign everything, or blindly copy reference repos until current reality is fully understood. Confidence: 0.85
- Follow the phase order: RECOVERY → AUDIT → CONSOLIDATION → PARITY → PRODUCTION READINESS, not feature expansion. Confidence: 0.85
- Map every frontend action through the full stack: Frontend → API Client → Handler → Service → Store → Database. Confidence: 0.80
- Mark each component as: REAL, PARTIAL, MOCK, BROKEN, or MISSING. Confidence: 0.80
- Produce a report before deleting anything identified as stale or duplicate. Confidence: 0.75
- Systematically convert reference PHP files one-by-one to Next.js rather than building abstractions or rewriting components from scratch — read each file, convert it, move to the next. Confidence: 0.70
