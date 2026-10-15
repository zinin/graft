package config

import (
	"regexp"
	"slices"
	"strconv"
	"unicode/utf8"
)

// yamlLine matches the text of a yaml.v3 error that names a line, such as
// "yaml: line 8: did not find expected ',' or ']'". yaml.v3 leaves the line
// out when it is 0, so a line it names starts at 1.
var yamlLine = regexp.MustCompile(`^yaml: line ([1-9][0-9]*): (.*)$`)

// yamlParserMessages are the messages of the yaml.v3 parser (parserc.go of
// v3.0.5), word for word. The parser counts lines from 0, and (*parser).fail
// in decode.go adds 1 to the line of a scanner error only, so the line in an
// error with one of these messages is one less than the line of the file.
// The messages of the scanner differ from these.
var yamlParserMessages = []string{
	"did not find expected <stream-start>",
	"did not find expected <document start>",
	"did not find expected node content",
	"did not find expected '-' indicator",
	"did not find expected key",
	"did not find expected ',' or ']'",
	"did not find expected ',' or '}'",
	"found duplicate %YAML directive",
	"found duplicate %TAG directive",
	"found incompatible YAML document",
	"found undefined tag handle",
}

// YAMLError splits an error of the yaml.v3 decoder, which was reading data,
// into the line of data it is about, counted from 1, and the message without
// the "yaml: line N: " in front of it. line is 0 when the error names no
// line; msg is then the text of err as it is. The line is never past the
// last line of data: yaml.v3 puts the end of the stream on the line after
// the last one, which is where it finds an unclosed [ or { at the end, and
// the reader has no such line to look at.
func YAMLError(err error, data []byte) (line int, msg string) {
	text := err.Error()
	m := yamlLine.FindStringSubmatch(text)
	if m == nil {
		return 0, text
	}
	n, convErr := strconv.Atoi(m[1])
	if convErr != nil {
		// Too big for an int: no file has such a line.
		return 0, text
	}
	if slices.Contains(yamlParserMessages, m[2]) {
		n++
	}
	last := lastLine(data)
	if last == 0 {
		// Empty data has no line to name.
		return 0, text
	}
	return min(n, last), m[2]
}

// lastLine returns the number of the last line of data, 0 for empty data.
// Lines are counted as the yaml.v3 scanner counts them (is_break in
// yamlprivateh.go; skip_line and read_line in scannerc.go): "\r\n" is one
// line break, and so are a lone "\r", "\n", NEL (U+0085), LS (U+2028) and PS
// (U+2029). Text after the last break is a line of its own. Fewer breaks than
// yaml.v3 counts would lower a line that yaml.v3 named right.
func lastLine(data []byte) int {
	n := 0
	afterBreak := true // no text since the last break; true for empty data
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		i += size
		isBreak := r == '\r' || r == '\n' || r == '\u0085' || r == '\u2028' || r == '\u2029'
		if r == '\r' && i < len(data) && data[i] == '\n' {
			i++ // "\r\n" is one break
		}
		if isBreak {
			n++
		}
		afterBreak = isBreak
	}
	if !afterBreak {
		n++
	}
	return n
}
