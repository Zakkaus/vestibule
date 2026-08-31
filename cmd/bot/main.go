package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Zakkaus/vestibule/internal/app"
	"github.com/Zakkaus/vestibule/internal/edition"
	"github.com/Zakkaus/vestibule/internal/status"
)

// version is set with -ldflags; plain builds use "dev".
var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/"+edition.Name+"/config.json", "path to config.json")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	log.SetOutput(status.RedactingWriter(os.Stderr))
	if *showVersion {
		fmt.Println(version)
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := app.Run(ctx, app.Options{
		ConfigPath:     *configPath,
		Token:          os.Getenv("BOT_TOKEN"),
		StateDirectory: os.Getenv("STATE_DIRECTORY"),
		TelegramAPIURL: os.Getenv("TELEGRAM_API_URL"),
		GitHubToken:    os.Getenv("GITHUB_TOKEN"),
		NotifySocket:   os.Getenv("NOTIFY_SOCKET"),
		Version:        version,
	}); err != nil {
		log.Fatal(err)
	}
}
