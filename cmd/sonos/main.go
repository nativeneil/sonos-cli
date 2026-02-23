package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/pflag"
	"golang.org/x/term"

	"sonos-playlist/internal/ai"
	"sonos-playlist/internal/config"
	nativecli "sonos-playlist/internal/native/cli"
	"sonos-playlist/internal/output"
	"sonos-playlist/internal/playlist"
	"sonos-playlist/internal/setup"
	"sonos-playlist/internal/sonos"
)

const (
	exitSuccess     = 0
	exitFailure     = 1
	exitUsage       = 2
	exitInterrupted = 130
	version         = "1.0.0"
)

type cliOptions struct {
	Provider          string
	ProviderSpecified bool
	Room              string
	Count             int
	SonosAPI          string
	DryRun            bool
	Monitor           bool
	ListRooms         bool
	Setup             bool
	Prompt            string
	JSON              bool
	Plain             bool
	Quiet             bool
	Verbose           bool
	NoColor           bool
	NoInput           bool
	Help              bool
	Version           bool
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func main() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigc
		fmt.Fprintln(os.Stderr, "Interrupted (Ctrl-C)")
		os.Exit(exitInterrupted)
	}()

	if nativecli.ShouldHandle(os.Args[1:]) {
		if err := nativecli.ExecuteArgs(os.Args[1:]); err != nil {
			var ue usageError
			if errors.As(err, &ue) {
				fmt.Fprintln(os.Stderr, ue.msg)
				os.Exit(exitUsage)
			}
			fmt.Fprintln(os.Stderr, "Error:", err.Error())
			os.Exit(exitFailure)
		}
		os.Exit(exitSuccess)
	}

	if err := run(context.Background()); err != nil {
		var ue usageError
		if errors.As(err, &ue) {
			fmt.Fprintln(os.Stderr, ue.msg)
			os.Exit(exitUsage)
		}
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
		os.Exit(exitFailure)
	}
	os.Exit(exitSuccess)
}

func run(ctx context.Context) error {
	cfg := config.Load()
	opts, err := parseArgs(cfg)
	if err != nil {
		return err
	}
	if opts.Help {
		printUsage(cfg)
		return nil
	}
	if opts.Version {
		fmt.Fprintln(os.Stdout, version)
		return nil
	}

	out := output.New(output.Options{
		JSON:    opts.JSON,
		Plain:   opts.Plain,
		Quiet:   opts.Quiet,
		Verbose: opts.Verbose,
		NoColor: opts.NoColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb",
	})

	if opts.Setup {
		if err := setup.Run(out); err != nil {
			return err
		}
		if opts.JSON {
			_ = out.EmitJSON(map[string]any{"status": "ok", "action": "setup"})
		}
		return nil
	}

	client := sonos.NewClient(opts.SonosAPI)
	ensureConnected := func() error {
		if !client.CheckConnection(ctx) {
			setup.PrintInstructions(out)
			return fmt.Errorf("sonos api not reachable")
		}
		return nil
	}

	if opts.ListRooms {
		if err := ensureConnected(); err != nil {
			return err
		}
		rooms, err := sonos.DiscoverRooms(ctx, client)
		if err != nil {
			return err
		}
		if opts.JSON {
			return out.EmitJSON(map[string]any{"rooms": rooms})
		}
		out.Print(out.Bold("Available Sonos speakers:"))
		for _, room := range rooms {
			out.Print("  - " + room)
		}
		return nil
	}

	prompt := opts.Prompt
	if prompt == "" && !opts.NoInput && !term.IsTerminal(int(os.Stdin.Fd())) {
		prompt = readPromptFromStdin()
	}
	if prompt == "" {
		return usageError{msg: strings.Join([]string{
			"Missing prompt or control command.",
			"Examples:",
			"  sonos \"relaxing jazz for Sunday morning\"",
			"  sonos play",
			"  echo \"night driving synthwave\" | sonos",
			"Run with --help for usage.",
		}, "\n")}
	}

	room := opts.Room
	if room == "" {
		if opts.DryRun {
			room = "Living Room"
		} else {
			if err := ensureConnected(); err != nil {
				return err
			}
			defaultRoom, err := sonos.DefaultRoom(ctx, client)
			if err != nil {
				return err
			}
			room = defaultRoom
		}
	}

	promptLower := strings.ToLower(strings.TrimSpace(prompt))
	if v, ok := parseVolumeCommand(promptLower); ok {
		if err := ensureConnected(); err != nil {
			return err
		}
		level, err := sonos.VolumeSet(ctx, client, room, v)
		if err != nil {
			return err
		}
		if opts.JSON {
			return out.EmitJSON(map[string]any{"action": "volume", "room": room, "value": level})
		}
		out.Success(fmt.Sprintf("Volume: %d", level))
		return nil
	}

	if handler, ok := controlCommands[promptLower]; ok {
		if err := ensureConnected(); err != nil {
			return err
		}
		result, err := handler(ctx, client, room)
		if err != nil {
			return err
		}
		if opts.JSON {
			payload := map[string]any{"room": room, "action": result.Action}
			if result.Value != nil {
				payload["value"] = result.Value
			}
			return out.EmitJSON(payload)
		}
		out.Success(renderControlText(result))
		return nil
	}

	keys := ai.APIKeys{
		Anthropic: cfg.AnthropicAPIKey,
		OpenAI:    cfg.OpenAIAPIKey,
		Google:    cfg.GoogleAPIKey,
		XAI:       cfg.XAIAPIKey,
	}
	if opts.ProviderSpecified {
		switch opts.Provider {
		case "claude":
			keys = ai.APIKeys{Anthropic: cfg.AnthropicAPIKey}
		case "openai":
			keys = ai.APIKeys{OpenAI: cfg.OpenAIAPIKey}
		case "gemini":
			keys = ai.APIKeys{Google: cfg.GoogleAPIKey}
		case "grok", "xai":
			keys = ai.APIKeys{XAI: cfg.XAIAPIKey}
		}
	}

	providers := selectedProviders(keys)
	if len(providers) == 0 {
		if opts.ProviderSpecified {
			out.Error("No API key configured for provider: " + opts.Provider)
		} else {
			out.Error("No API keys configured.")
		}
		out.Error("Set at least one of: ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY, XAI_API_KEY")
		return fmt.Errorf("missing api keys")
	}

	out.Info(out.Gray("Using providers: " + strings.Join(providers, ", ")))
	if !opts.DryRun {
		if err := ensureConnected(); err != nil {
			return err
		}
		out.Info(out.Gray("Using speaker: " + room))
	}

	result, err := playlist.GenerateAndPlay(ctx, playlist.GeneratorOptions{
		Keys:             keys,
		Prompt:           prompt,
		Room:             room,
		DryRun:           opts.DryRun,
		Client:           client,
		Monitor:          opts.Monitor,
		CountPerProvider: opts.Count,
		Output:           out,
	})
	if err != nil {
		return err
	}

	if opts.JSON {
		return out.EmitJSON(map[string]any{
			"providers":        providers,
			"countPerProvider": opts.Count,
			"room":             result.Room,
			"dryRun":           result.DryRun,
			"queuedSongs":      result.QueuedSongs,
			"failedSongs":      result.FailedSongs,
			"playbackStarted":  result.PlaybackStarted,
			"monitored":        result.Monitored,
		})
	}
	return nil
}

func parseArgs(cfg config.Config) (cliOptions, error) {
	opts := cliOptions{
		Provider: string(cfg.DefaultProvider),
		Room:     cfg.DefaultRoom,
		Count:    cfg.DefaultCount,
		SonosAPI: cfg.SonosAPIURL,
	}

	fs := pflag.NewFlagSet("sonos", pflag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.SortFlags = false

	fs.BoolVarP(&opts.Help, "help", "h", false, "display help")
	fs.BoolVar(&opts.Version, "version", false, "output the version number")
	fs.StringVarP(&opts.Provider, "provider", "p", opts.Provider, "AI provider: claude, openai, gemini, grok")
	fs.StringVarP(&opts.Room, "room", "r", opts.Room, "Sonos speaker name")
	fs.IntVarP(&opts.Count, "count", "c", opts.Count, "Number of songs generated per provider (1-50)")
	fs.StringVarP(&opts.SonosAPI, "sonos-api", "s", opts.SonosAPI, "Sonos HTTP API URL")
	fs.BoolVarP(&opts.DryRun, "dry-run", "d", false, "Preview playlist without playing")
	fs.BoolVar(&opts.JSON, "json", false, "Output machine-readable JSON for supported commands")
	fs.BoolVar(&opts.Plain, "plain", false, "Disable decorative formatting")
	fs.BoolVarP(&opts.Quiet, "quiet", "q", false, "Suppress non-essential output")
	fs.BoolVarP(&opts.Verbose, "verbose", "v", false, "Enable verbose diagnostics")
	fs.BoolVar(&opts.NoColor, "no-color", false, "Disable colored output")
	fs.BoolVar(&opts.NoInput, "no-input", false, "Disable stdin reads/prompts")
	fs.BoolVar(&opts.Monitor, "monitor", false, "Keep running after queueing and auto-skip unavailable tracks (opt-in)")
	fs.BoolVarP(&opts.ListRooms, "list-rooms", "l", false, "Show available Sonos speakers")
	fs.BoolVar(&opts.Setup, "setup", false, "Install and start node-sonos-http-api")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return cliOptions{}, usageError{msg: err.Error() + "\n(run with --help for usage)"}
	}
	if fs.Lookup("provider") != nil {
		opts.ProviderSpecified = fs.Lookup("provider").Changed
	}

	switch strings.ToLower(opts.Provider) {
	case "claude", "openai", "gemini", "grok", "xai":
		opts.Provider = strings.ToLower(opts.Provider)
	default:
		return cliOptions{}, usageError{msg: "provider must be one of: claude, openai, gemini, grok"}
	}
	if opts.Count < 1 || opts.Count > 50 {
		return cliOptions{}, usageError{msg: "count must be between 1 and 50"}
	}

	args := fs.Args()
	opts.Prompt = strings.TrimSpace(strings.Join(args, " "))
	return opts, nil
}

func printUsage(cfg config.Config) {
	fmt.Fprintln(os.Stdout, "Usage: sonos [options] [prompt]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "AI-powered Sonos playlist generator (queues and exits by default)")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Arguments:")
	fmt.Fprintln(os.Stdout, "  prompt                     Natural language playlist description")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Options:")
	fmt.Fprintln(os.Stdout, "  -h, --help                 display help")
	fmt.Fprintln(os.Stdout, "      --version              output the version number")
	fmt.Fprintf(os.Stdout, "  -p, --provider <provider>  AI provider: claude, openai, gemini, grok (default: %q)\n", cfg.DefaultProvider)
	fmt.Fprintln(os.Stdout, "  -r, --room <room>          Sonos speaker name")
	fmt.Fprintf(os.Stdout, "  -c, --count <number>       Number of songs generated per provider (1-50) (default: %d)\n", cfg.DefaultCount)
	fmt.Fprintf(os.Stdout, "  -s, --sonos-api <url>      Sonos HTTP API URL (default: %q)\n", cfg.SonosAPIURL)
	fmt.Fprintln(os.Stdout, "  -d, --dry-run              Preview playlist without playing")
	fmt.Fprintln(os.Stdout, "      --json                 Output machine-readable JSON for supported commands")
	fmt.Fprintln(os.Stdout, "      --plain                Disable decorative formatting")
	fmt.Fprintln(os.Stdout, "  -q, --quiet                Suppress non-essential output")
	fmt.Fprintln(os.Stdout, "  -v, --verbose              Enable verbose diagnostics")
	fmt.Fprintln(os.Stdout, "      --no-color             Disable colored output")
	fmt.Fprintln(os.Stdout, "      --no-input             Disable stdin reads/prompts")
	fmt.Fprintln(os.Stdout, "      --monitor              Keep running after queueing and auto-skip unavailable tracks (opt-in)")
	fmt.Fprintln(os.Stdout, "  -l, --list-rooms           Show available Sonos speakers")
	fmt.Fprintln(os.Stdout, "      --setup                Install and start node-sonos-http-api")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Native Sonos Commands:")
	fmt.Fprintln(os.Stdout, "  discover, status (now), queue, favorites, group, config, volume, mute, watch, scene, play, pause, stop, next, prev")
	fmt.Fprintln(os.Stdout, "  Run `sonos <command> --help` for command-specific usage.")
	fmt.Fprintln(os.Stdout, "  Run `sonos help` for the native command tree.")
}

func readPromptFromStdin() string {
	scanner := bufio.NewScanner(os.Stdin)
	lines := []string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, " "))
}

var volumeRE = regexp.MustCompile(`^volume\s+(\d+)$`)

func parseVolumeCommand(s string) (int, bool) {
	m := volumeRE.FindStringSubmatch(s)
	if len(m) != 2 {
		return 0, false
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return v, true
}

type controlResult struct {
	Action string
	Value  any
}

type controlHandler func(ctx context.Context, client *sonos.Client, room string) (controlResult, error)

var controlCommands = map[string]controlHandler{
	"play": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "play"}, sonos.Play(ctx, client, room)
	},
	"pause": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "pause"}, sonos.Pause(ctx, client, room)
	},
	"stop": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "stop"}, sonos.Pause(ctx, client, room)
	},
	"skip": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "skip"}, sonos.Skip(ctx, client, room)
	},
	"next": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "skip"}, sonos.Skip(ctx, client, room)
	},
	"previous": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "previous"}, sonos.Previous(ctx, client, room)
	},
	"prev": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "previous"}, sonos.Previous(ctx, client, room)
	},
	"back": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "previous"}, sonos.Previous(ctx, client, room)
	},
	"again": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "restart"}, sonos.Restart(ctx, client, room)
	},
	"replay": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "restart"}, sonos.Restart(ctx, client, room)
	},
	"restart": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		return controlResult{Action: "restart"}, sonos.Restart(ctx, client, room)
	},
	"volume up": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.VolumeUp(ctx, client, room)
		return controlResult{Action: "volume", Value: v}, err
	},
	"volume down": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.VolumeDown(ctx, client, room)
		return controlResult{Action: "volume", Value: v}, err
	},
	"volume high": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.VolumeHigh(ctx, client, room)
		return controlResult{Action: "volume", Value: v}, err
	},
	"volume low": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.VolumeLow(ctx, client, room)
		return controlResult{Action: "volume", Value: v}, err
	},
	"volume": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.GetVolume(ctx, client, room)
		return controlResult{Action: "volume", Value: v}, err
	},
	"repeat": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.Repeat(ctx, client, room, "")
		return controlResult{Action: "repeat", Value: v}, err
	},
	"repeat all": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.Repeat(ctx, client, room, "all")
		return controlResult{Action: "repeat", Value: v}, err
	},
	"repeat one": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.Repeat(ctx, client, room, "one")
		return controlResult{Action: "repeat", Value: v}, err
	},
	"repeat off": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.Repeat(ctx, client, room, "none")
		return controlResult{Action: "repeat", Value: v}, err
	},
	"shuffle": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		v, err := sonos.Shuffle(ctx, client, room, nil)
		return controlResult{Action: "shuffle", Value: v}, err
	},
	"shuffle on": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		t := true
		v, err := sonos.Shuffle(ctx, client, room, &t)
		return controlResult{Action: "shuffle", Value: v}, err
	},
	"shuffle off": func(ctx context.Context, client *sonos.Client, room string) (controlResult, error) {
		f := false
		v, err := sonos.Shuffle(ctx, client, room, &f)
		return controlResult{Action: "shuffle", Value: v}, err
	},
}

func renderControlText(result controlResult) string {
	if result.Action == "volume" {
		if v, ok := result.Value.(int); ok {
			return fmt.Sprintf("Volume: %d", v)
		}
	}
	if result.Action == "repeat" {
		if v, ok := result.Value.(string); ok {
			return "Repeat: " + v
		}
	}
	if result.Action == "shuffle" {
		if v, ok := result.Value.(bool); ok {
			if v {
				return "Shuffle: on"
			}
			return "Shuffle: off"
		}
	}
	switch result.Action {
	case "play":
		return "Playing"
	case "pause":
		return "Paused"
	case "stop":
		return "Stopped"
	case "skip":
		return "Skipped to next track"
	case "previous":
		return "Back to previous track"
	case "restart":
		return "Restarted track"
	default:
		return "Done"
	}
}

func selectedProviders(keys ai.APIKeys) []string {
	providers := []string{}
	if keys.Anthropic != "" {
		providers = append(providers, "Claude")
	}
	if keys.OpenAI != "" {
		providers = append(providers, "OpenAI")
	}
	if keys.Google != "" {
		providers = append(providers, "Gemini")
	}
	if keys.XAI != "" {
		providers = append(providers, "Grok")
	}
	return providers
}
