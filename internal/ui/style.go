package ui

import (
	"fmt"
	"io"
	"os"
)

type Color string

const (
	Green   Color = "\x1b[32m"
	Cyan    Color = "\x1b[36m"
	Blue    Color = "\x1b[34m"
	Yellow  Color = "\x1b[33m"
	Red     Color = "\x1b[31m"
	Magenta Color = "\x1b[35m"

	reset = "\x1b[0m"
)

func Tag(out io.Writer, label string) string {
	return Colorize(out, labelColor(label), fmt.Sprintf("%-6s", label))
}

func Status(out io.Writer, label string) string {
	return Colorize(out, labelColor(label), fmt.Sprintf("%-4s", label))
}

func Colorize(out io.Writer, color Color, text string) string {
	if color == "" || !SupportsColor(out) {
		return text
	}
	return string(color) + text + reset
}

func SupportsColor(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	forceColor := os.Getenv("FORCE_COLOR")
	if forceColor != "" {
		return forceColor != "0"
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func labelColor(label string) Color {
	switch label {
	case "OK":
		return Green
	case "READY":
		return Cyan
	case "SERVE":
		return Magenta
	case "WARN":
		return Yellow
	case "ERR":
		return Red
	case "REDR":
		return Blue
	default:
		return ""
	}
}
