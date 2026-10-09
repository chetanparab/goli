#!/bin/bash

# Raycast Script Command: open a goli link. Add this folder in
# Raycast → Settings → Extensions → Script Commands → Add Directories.
#
# @raycast.schemaVersion 1
# @raycast.title go/
# @raycast.mode silent
# @raycast.packageName goli
# @raycast.icon 🟠
# @raycast.argument1 { "type": "text", "placeholder": "key or ?search" }
# @raycast.description Open a go/ link, or search with ?words

PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"
q="$1"
if [[ "$q" == \?* ]]; then
  words="${q#\?}"; open "http://go/?${words// /%20}"
else
  goli "$q" >/dev/null || open "http://go/${q}"
fi
