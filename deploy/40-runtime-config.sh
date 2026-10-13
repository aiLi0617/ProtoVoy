#!/bin/sh
set -eu
printf 'window.__PROTOVOY_CONFIG__ = ' > /usr/share/nginx/html/config.js
jq -cn \
  --arg agentUrl "${PROTOVOY_PUBLIC_AGENT_PATH}" \
  --arg agentToken "${PROTOVOY_AGENT_TOKEN}" \
  --arg collaborationUrl "${PROTOVOY_PUBLIC_COLLABORATION_PATH}" \
  '{agentUrl:$agentUrl,agentToken:$agentToken,collaborationUrl:$collaborationUrl}' \
  >> /usr/share/nginx/html/config.js
printf ';\n' >> /usr/share/nginx/html/config.js
