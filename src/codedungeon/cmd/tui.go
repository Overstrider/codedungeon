package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
)

var (
	tuiOut io.Writer = os.Stderr
	tuiIn  io.Reader = os.Stdin
)

func isTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

func useColor() bool {
	if runtime.GOOS == "windows" {
		return os.Getenv("WT_SESSION") != "" || os.Getenv("TERM") != ""
	}
	return true
}

func ansi(code, text string) string {
	if !useColor() {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

func printBanner(version string) {
	fmt.Fprintf(tuiOut, "\n  %s %s\n", ansi("1", "codedungeon"), version+" setup")
	fmt.Fprintf(tuiOut, "  %s\n\n", ansi("2", "────────────────────────"))
}

func printStep(n, total int, msg string) {
	fmt.Fprintf(tuiOut, "  %s %s\n", ansi("1", fmt.Sprintf("[%d/%d]", n, total)), msg)
}

func printDetail(msg string) {
	fmt.Fprintf(tuiOut, "        %s\n", msg)
}

func printOK(label, status string) {
	fmt.Fprintf(tuiOut, "        %-11s %s\n", label, ansi("32", status))
}

func printWarn(msg string) {
	fmt.Fprintf(tuiOut, "  %s %s\n", ansi("33", "WARN"), msg)
}

func printErr(msg string) {
	fmt.Fprintf(tuiOut, "  %s %s\n", ansi("31", "ERROR"), msg)
}

func promptChoice(prompt string, opts []string, defaultIdx int) int {
	reader := bufio.NewReader(tuiIn)
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Fprintln(tuiOut)
		for i, o := range opts {
			marker := "  "
			if i == defaultIdx {
				marker = ansi("32", "* ")
			}
			fmt.Fprintf(tuiOut, "        %s%d) %s\n", marker, i+1, o)
		}
		fmt.Fprintf(tuiOut, "\n        Choice [%d]: ", defaultIdx+1)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultIdx
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= len(opts) {
			return n - 1
		}
		fmt.Fprintf(tuiOut, "        %s\n", ansi("33", "Invalid choice, try again."))
	}
	return defaultIdx
}

func promptYesNo(prompt string, defaultYes bool) bool {
	reader := bufio.NewReader(tuiIn)
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	fmt.Fprintf(tuiOut, "        %s %s: ", prompt, hint)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}
