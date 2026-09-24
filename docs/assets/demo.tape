# Records docs/assets/demo.gif against live stores (read-only commands only).
# DEMO_DIR is a directory holding app-release.aab (publish only runs with --dry-run).
#   DEMO_DIR=/path/to/dir vhs docs/assets/demo.tape
# vhs 0.12 writes no GIF with ffmpeg 9; then record with "Output frames/" and
# assemble the text and cursor layers yourself.
Output docs/assets/demo.gif

Set Shell zsh
Set FontSize 14
Set Width 1400
Set Height 340
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms
Set WindowBar Colorful
Set BorderRadius 10

Hide
Type `cd "$DEMO_DIR" && clear` Enter
Show

Type "droidship status com.gdebenz.win" Sleep 400ms Enter
Wait@40s
Sleep 4s

Hide
Type "clear" Enter
Show
Type "droidship reviews com.gdebenz.win --stars 5 --limit 3" Sleep 400ms Enter
Wait@40s
Sleep 4.5s

Hide
Type "clear" Enter
Show
Type "droidship publish com.gdebenz.win --store all --aab app-release.aab --dry-run" Sleep 500ms Enter
Wait@60s
Sleep 6s

Hide
Type "clear" Enter
Show
Type "droidship reviews com.gdebenz.win --json | jq -c '.[:4][] | {store, stars, replied}'" Sleep 400ms Enter
Wait@40s
Sleep 4.5s
