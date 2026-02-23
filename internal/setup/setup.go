package setup

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"sonos-playlist/internal/output"
)

func installDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".sonos-http-api"
	}
	return filepath.Join(home, ".sonos-http-api")
}

const repoURL = "https://github.com/jishi/node-sonos-http-api.git"

func Run(out *output.Output) error {
	dir := installDir()
	out.Info("Setting up node-sonos-http-api...")
	if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
		out.Warn("Found existing installation at " + dir)
		out.Info("Pulling latest changes...")
		cmd := exec.Command("git", "pull")
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			out.Warn("Could not pull updates, continuing...")
		}
	} else {
		out.Info("Cloning to " + dir + "...")
		cmd := exec.Command("git", "clone", repoURL, dir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	out.Info("Installing dependencies...")
	npmInstall := exec.Command("npm", "install")
	npmInstall.Dir = dir
	npmInstall.Stdout = os.Stdout
	npmInstall.Stderr = os.Stderr
	if err := npmInstall.Run(); err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}

	out.Info("Starting node-sonos-http-api...")
	out.Info("(Press Ctrl+C to stop)")
	server := exec.Command("node", "server.js")
	server.Dir = dir
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	server.Stdin = os.Stdin
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start node-sonos-http-api: %w", err)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigc
		out.Warn("Stopping...")
		_ = server.Process.Signal(os.Interrupt)
	}()

	return server.Wait()
}

func PrintInstructions(out *output.Output) {
	dir := installDir()
	out.Error("Could not connect to node-sonos-http-api")
	out.Print("The Sonos HTTP API service needs to be running.")
	out.Print("Run the following command to set it up:")
	out.Print("  sonos --setup")
	out.Print("Or start it manually if already installed:")
	out.Print("  cd " + dir + " && node server.js")
}
