#!/bin/bash
# All-in-one dependency vulnerability scanner
# Usage: ./scan-deps.sh < package.json
# Usage: cat go.mod | ./scan-deps.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMANDA_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"

# Step 1: Parse deps with LLM
deps=$(cat | "$COMANDA_DIR/comanda" process "$SCRIPT_DIR/zeroday-realtime.yaml" 2>/dev/null | tail -n 1)

# Step 2: Query OSV
osv_data=$("$SCRIPT_DIR/osv-query.sh" <<< "$deps")

# Step 3: Generate report (truncate large responses)
# Extract just the summary data to avoid token limits
summary=$(echo "$osv_data" | jq -c '{
  scan_time: .scan_time,
  packages: [.dependencies[].name],
  vulns: [.osv_results[] | select(.vulns.vulns != null) | {
    pkg: .package,
    issues: [.vulns.vulns[] | {
      id: .id,
      summary: .summary,
      severity: (.severity // [] | map(select(.type == "CVSS_V3")) | .[0].score // "unknown"),
      fixed: (.affected // [] | .[0].ranges // [] | .[0].events // [] | map(select(.fixed != null)) | .[0].fixed // "unknown")
    }]
  }]
}')

echo "$summary" | "$COMANDA_DIR/comanda" process "$SCRIPT_DIR/zeroday-report.yaml" 2>/dev/null
