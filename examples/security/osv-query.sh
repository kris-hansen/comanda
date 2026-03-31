#!/bin/bash
# Query OSV.dev API for vulnerabilities
# Input: JSON array of dependencies from stdin
# Output: JSON with OSV results

deps=$(cat)

echo "{"
echo "\"scan_time\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
echo "\"dependencies\": $deps,"
echo "\"osv_results\": ["

first=true
for pkg in $(echo "$deps" | jq -r '.[] | @base64'); do
  _jq() { echo "$pkg" | base64 -d | jq -r "$1"; }
  name=$(_jq '.name')
  version=$(_jq '.version')
  ecosystem=$(_jq '.ecosystem')
  
  result=$(curl -s "https://api.osv.dev/v1/query" \
    -H "Content-Type: application/json" \
    -d "{\"package\":{\"name\":\"$name\",\"ecosystem\":\"$ecosystem\"},\"version\":\"$version\"}" 2>/dev/null || echo '{}')
  
  if [ "$first" = "true" ]; then
    first=false
  else
    echo ","
  fi
  echo "{\"package\":\"$name@$version\",\"vulns\":$result}"
done

echo "]}"
