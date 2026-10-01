#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

# Wraps the ocr binary in a double-clickable macOS .app that starts
# `ocr viewer` in the background (or reuses a running one) and opens it.
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <binary> <output.app> <version>" >&2
  exit 2
fi
binary=$1 app=$2 version=$3
[[ -x "$binary" ]] || { echo "binary not found: $binary" >&2; exit 1; }

rm -rf "$app"
# Stay-open AppleScript applet: the viewer lives as its child (macOS tears down
# whatever a plain script app leaves behind), a second double-click reopens the
# browser, and Quit stops the viewer it started.
# The login interactive shell restores PATH, git and OCR_LLM_* settings that
# Finder does not pass to apps.
# ponytail: assumes a POSIX login shell ($0); fish users need SHELL=/bin/zsh.
osacompile -s -o "$app" <<'OSA'
global viewerURL, viewerPid

on viewerUp()
  try
    do shell script "/usr/bin/curl -fs --max-time 1 -o /dev/null " & viewerURL & "/api/reviews"
    return true
  on error
    return false
  end try
end viewerUp

on run
  set viewerURL to "http://localhost:5483"
  set viewerPid to ""
  if not viewerUp() then
    set bin to POSIX path of (path to me) & "Contents/Resources/opencodereview"
    set logDir to POSIX path of (path to library folder from user domain) & "Logs/OpenCodeReview"
    set viewerPid to do shell script "mkdir -p " & quoted form of logDir & "; \"${SHELL:-/bin/zsh}\" -lic 'exec \"$0\" viewer --open=never' " & quoted form of bin & " </dev/null >>" & quoted form of (logDir & "/viewer.log") & " 2>&1 & echo $!"
    repeat 50 times
      if viewerUp() then exit repeat
      delay 0.2
    end repeat
    if not viewerUp() then
      display alert "Open Code Review viewer did not start" message "See " & logDir & "/viewer.log"
      quit
      return
    end if
  end if
  open location viewerURL
end run

on reopen
  open location viewerURL
end reopen

on quit
  if viewerPid is not "" then do shell script "kill " & viewerPid & " 2>/dev/null; true"
  continue quit
end quit
OSA
cp "$binary" "$app/Contents/Resources/opencodereview"
plist="$app/Contents/Info.plist"
plutil -replace CFBundleName -string "Open Code Review" "$plist"
plutil -replace CFBundleDisplayName -string "Open Code Review" "$plist"
plutil -replace CFBundleIdentifier -string com.alibaba.opencodereview.viewer "$plist"
plutil -replace CFBundleShortVersionString -string "${version#v}" "$plist"

# ponytail: icon rendered from imgs/logo.svg via sips; skipped when sips cannot read SVG.
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
if sips -s format png -z 1024 1024 "$repo_root/imgs/logo.svg" --out "$tmp/logo.png" >/dev/null 2>&1; then
  mkdir "$tmp/AppIcon.iconset"
  for size in 16 32 128 256 512; do
    sips -z $size $size "$tmp/logo.png" --out "$tmp/AppIcon.iconset/icon_${size}x${size}.png" >/dev/null
    sips -z $((size * 2)) $((size * 2)) "$tmp/logo.png" --out "$tmp/AppIcon.iconset/icon_${size}x${size}@2x.png" >/dev/null
  done
  iconutil -c icns "$tmp/AppIcon.iconset" -o "$app/Contents/Resources/applet.icns"
fi

# Edits above break the applet's ad-hoc seal; re-sign so Gatekeeper accepts it.
codesign --force --sign - "$app"
echo "built $app"
