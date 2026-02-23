package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
)

type Options struct {
	JSON    bool
	Plain   bool
	Quiet   bool
	Verbose bool
	NoColor bool
}

type Output struct {
	JSON    bool
	Plain   bool
	Quiet   bool
	Verbose bool

	green  *color.Color
	yellow *color.Color
	red    *color.Color
	gray   *color.Color
	bold   *color.Color
}

func New(opts Options) *Output {
	if opts.NoColor || opts.Plain {
		color.NoColor = true
	}
	return &Output{
		JSON:    opts.JSON,
		Plain:   opts.Plain,
		Quiet:   opts.Quiet,
		Verbose: opts.Verbose,
		green:   color.New(color.FgGreen),
		yellow:  color.New(color.FgYellow),
		red:     color.New(color.FgRed),
		gray:    color.New(color.FgHiBlack),
		bold:    color.New(color.Bold),
	}
}

func (o *Output) Green(s string) string {
	return o.green.Sprint(s)
}

func (o *Output) Yellow(s string) string {
	return o.yellow.Sprint(s)
}

func (o *Output) Red(s string) string {
	return o.red.Sprint(s)
}

func (o *Output) Gray(s string) string {
	return o.gray.Sprint(s)
}

func (o *Output) Bold(s string) string {
	return o.bold.Sprint(s)
}

func (o *Output) Info(msg string) {
	if o.JSON || o.Quiet {
		return
	}
	fmt.Fprintln(os.Stdout, msg)
}

func (o *Output) Success(msg string) {
	if o.JSON || o.Quiet {
		return
	}
	fmt.Fprintln(os.Stdout, o.Green(msg))
}

func (o *Output) Warn(msg string) {
	if o.JSON || o.Quiet {
		return
	}
	fmt.Fprintln(os.Stdout, o.Yellow(msg))
}

func (o *Output) Debug(msg string) {
	if o.JSON || !o.Verbose {
		return
	}
	fmt.Fprintln(os.Stderr, o.Gray(msg))
}

func (o *Output) Error(msg string) {
	fmt.Fprintln(os.Stderr, o.Red(msg))
}

func (o *Output) Print(msg string) {
	if o.JSON || o.Quiet {
		return
	}
	fmt.Fprintln(os.Stdout, msg)
}

func (o *Output) Write(msg string) {
	if o.JSON || o.Quiet {
		return
	}
	fmt.Fprint(os.Stdout, msg)
}

func (o *Output) EmitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
