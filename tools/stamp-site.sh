#!/bin/sh
# Stamps site/*.html with a content hash of the CSS/JS so browsers fetch new
# versions after every deploy. Run before deploying: tools/stamp-site.sh
set -eu
cd "$(dirname "$0")/../site"
v=$(cat assets/site.js assets/site.css assets/theme.js | shasum -a 256 | cut -c1-10)
for f in *.html; do
  sed -i.bak -E "s#/assets/(site\.css|site\.js|theme\.js)(\?v=[a-f0-9]+)?\"#/assets/\1?v=$v\"#g" "$f" && rm "$f.bak"
done
echo "stamped assets ?v=$v"
