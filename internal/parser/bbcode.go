package parser

import (
	"html"
	"regexp"
	"strings"
)

var (
	reBr           = regexp.MustCompile(`(?i)<br\s*/?>`)
	reReplyBtn     = regexp.MustCompile(`(?is)\[pid=[^\]]*\]Reply\[/pid\]\s*`)
	reQuote        = regexp.MustCompile(`(?is)\[quote\](.*?)\[/quote\]`)
	reBold         = regexp.MustCompile(`(?is)\[b\](.*?)\[/b\]`)
	reItalic       = regexp.MustCompile(`(?is)\[i\](.*?)\[/i\]`)
	reDel          = regexp.MustCompile(`(?is)\[del\](.*?)\[/del\]`)
	reImg          = regexp.MustCompile(`(?is)\[img\](.*?)\[/img\]`)
	reUrlWith      = regexp.MustCompile(`(?is)\[url=([^\]]+)\](.*?)\[/url\]`)
	reUrlSimple    = regexp.MustCompile(`(?is)\[url\](.*?)\[/url\]`)
	reColor        = regexp.MustCompile(`(?is)\[color=[^\]]+\](.*?)\[/color\]`)
	reSize         = regexp.MustCompile(`(?is)\[size=[^\]]+\](.*?)\[/size\]`)
	reGenericTag   = regexp.MustCompile(`\[/?[a-zA-Z0-9_]+(?:=[^\]]+)?\]`)
)

// CleanContent converts HTML line breaks, decodes HTML entities,
// and transforms common NGA BBCode tags into clean readable terminal text.
func CleanContent(raw string) string {
	if raw == "" {
		return ""
	}

	// 1. Convert <br/> to newline
	text := reBr.ReplaceAllString(raw, "\n")

	// 2. Decode HTML entities
	text = html.UnescapeString(text)

	// 3. Strip quote noise like [pid=...]Reply[/pid]
	text = reReplyBtn.ReplaceAllString(text, "")

	// 4. Format [quote]
	text = reQuote.ReplaceAllStringFunc(text, func(m string) string {
		sub := reQuote.FindStringSubmatch(m)
		if len(sub) > 1 {
			inner := CleanContent(sub[1])
			lines := strings.Split(inner, "\n")
			var quoted []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					quoted = append(quoted, "> "+line)
				}
			}
			return "\n" + strings.Join(quoted, "\n") + "\n\n"
		}
		return m
	})

	// 5. Format images
	text = reImg.ReplaceAllString(text, "[图片: $1]")

	// 6. Format URLs
	text = reUrlWith.ReplaceAllString(text, "$2 ($1)")
	text = reUrlSimple.ReplaceAllString(text, "$1")

	// 7. Format styles
	text = reBold.ReplaceAllString(text, "$1")
	text = reItalic.ReplaceAllString(text, "$1")
	text = reDel.ReplaceAllString(text, "~~$1~~")
	text = reColor.ReplaceAllString(text, "$1")
	text = reSize.ReplaceAllString(text, "$1")

	// 8. Strip leftover BBCode tags (like [pid=...], [uid=...], etc.)
	text = reGenericTag.ReplaceAllString(text, "")

	// 9. Collapse excessive blank lines
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	consecutiveBlank := 0
	for _, l := range lines {
		trimmed := strings.TrimRight(l, " \t\r")
		if trimmed == "" {
			consecutiveBlank++
			if consecutiveBlank <= 1 {
				cleanedLines = append(cleanedLines, "")
			}
		} else {
			consecutiveBlank = 0
			cleanedLines = append(cleanedLines, trimmed)
		}
	}

	return strings.TrimSpace(strings.Join(cleanedLines, "\n"))
}
