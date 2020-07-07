package printer

import (
	"fmt"
	"io"
	"strings"
)

const defaultStyle string = "\x1b[0m"

type consoleLogger struct {
	color bool
	w     io.Writer
}

func NewConsoleLogger(color bool, writer io.Writer) *consoleLogger {
	return &consoleLogger{
		color: color,
		w:     writer,
	}
}

func (s *consoleLogger) Colorize(colorCode string, format string, args ...interface{}) string {
	var out string

	if len(args) > 0 {
		out = fmt.Sprintf(format, args...)
	} else {
		out = format
	}

	if s.color {
		return fmt.Sprintf("%s%s%s", colorCode, out, defaultStyle)
	} else {
		return out
	}
}

func (s *consoleLogger) PrintBanner(text string, bannerCharacter string) {
	fmt.Fprintln(s.w, text)
	fmt.Fprintln(s.w, strings.Repeat(bannerCharacter, len(text)))
}

func (s *consoleLogger) PrintNewLine() {
	fmt.Fprintln(s.w, "")
}

func (s *consoleLogger) Print(indentation int, format string, args ...interface{}) {
	fmt.Fprint(s.w, s.indent(indentation, format, args...))
}

func (s *consoleLogger) Println(indentation int, format string, args ...interface{}) {
	fmt.Fprintln(s.w, s.indent(indentation, format, args...))
}

func (s *consoleLogger) indent(indentation int, format string, args ...interface{}) string {
	var text string

	if len(args) > 0 {
		text = fmt.Sprintf(format, args...)
	} else {
		text = format
	}

	stringArray := strings.Split(text, "\n")
	padding := ""
	if indentation >= 0 {
		padding = strings.Repeat("  ", indentation)
	}
	for i, s := range stringArray {
		stringArray[i] = fmt.Sprintf("%s%s", padding, s)
	}

	return strings.Join(stringArray, "\n")
}
