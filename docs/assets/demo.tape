# Records docs/assets/demo.gif against live stores (read-only commands only).
#   vhs docs/assets/demo.tape
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
Type "clear" Enter
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
Type "droidship listing com.gdebenz.win --store gplay,appgallery" Sleep 400ms Enter
Wait@40s
Sleep 3s
Type "droidship notes com.gdebenz.win --store rustore --text 'Bug fixes'; echo exit $?" Sleep 400ms Enter
Wait@40s
Sleep 3.5s

Hide
Type "clear" Enter
Show
Type "droidship reviews com.gdebenz.win --json | jq -c '.[:4][] | {store, stars, replied}'" Sleep 400ms Enter
Wait@40s
Sleep 4.5s
