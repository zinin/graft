package config

import (
	"regexp"
	"slices"
	"strconv"
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

// YAMLError splits an error of the yaml.v3 decoder into the line of the
// file it is about, counted from 1, and the message without the
// "yaml: line N: " in front of it. line is 0 when the error names no line;
// msg is then the text of err as it is.
func YAMLError(err error) (line int, msg string) {
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
	return n, m[2]
}
