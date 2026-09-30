# pubkit demo — rendered with VHS (https://github.com/charmbracelet/vhs).
#
# The library it records against is SYNTHETIC, built by `go run ./tools/demolib`:
# every publication title, article and paragraph on screen is invented. The one
# exception is the Bible passage, which is real — Juan allowed that specifically
# on 2026-09-30, and DECISIONS.md records it. Nothing else from a publication is
# in this recording, and nothing at all is committed except the rendered GIF.
#
# Regenerate:
#   go build -o bin/pubkit . && go run ./tools/demolib --library /tmp/pubkit-demo
#   pubkit --library /tmp/pubkit-demo --language S sync nwtsty   # the real Bible
#   vhs .demo/demo.tape

Output assets/demo.gif

Set Shell bash
Set FontSize 15
Set Width 1200
Set Height 800
Set Padding 18
Set Theme "Dracula"
Set TypingSpeed 28ms

Hide
Type "export PATH=$PWD/bin:$PATH; export JWPUBKIT_HOME=/tmp/pubkit-demo; clear"
Enter
Show

Sleep 500ms
Type "pubkit pubs"
Sleep 250ms
Enter
Sleep 1800ms

Type "pubkit week 2026-01-05 --no-watchtower --offline"
Sleep 250ms
Enter
Sleep 2600ms

Type "clear"
Enter
Sleep 300ms

Type "pubkit verse 'Jer 38:6' --language S --no-citations"
Sleep 250ms
Enter
Sleep 3200ms

Type "clear"
Enter
Sleep 300ms

Type "pubkit search 'what does it say about courage?' --limit 3"
Sleep 250ms
Enter
Sleep 2600ms

Type "pubkit graph 'Ge 1:1' --per-relation 3"
Sleep 250ms
Enter
Sleep 2600ms

Type "clear"
Enter
Sleep 300ms

Type "pubkit reading-time --reference 'Jer 38:1-6' --language S"
Sleep 250ms
Enter
Sleep 2200ms

Type "pubkit config"
Sleep 250ms
Enter
Sleep 2400ms
