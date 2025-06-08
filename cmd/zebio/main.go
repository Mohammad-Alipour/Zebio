package main

import (
	"context"
	"log"
	"os"

	"github.com/Mohammad-Alipour/Zebio/internal/bot"
	"github.com/Mohammad-Alipour/Zebio/internal/config"
	"github.com/Mohammad-Alipour/Zebio/internal/downloader"

	"github.com/joho/godotenv"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, relying on system environment variables.")
	}

	log.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Error loading configuration: %v", err)
		os.Exit(1)
	}

	log.Printf("Configuration loaded successfully.")
	log.Printf(" - YTDLP Path: %s", cfg.YTDLPPath)
	log.Printf(" - Download Dir: %s", cfg.DownloadDir)
	if cfg.TelegramBotToken == "" {
		log.Println("CRITICAL: Telegram Bot Token is not set in configuration! Exiting.")
		os.Exit(1)
	} else {
		log.Println("Telegram Bot Token is set.")
	}
	if len(cfg.AllowedUserIDs) > 0 {
		log.Printf(" - Allowed User IDs: %v", cfg.AllowedUserIDs)
	} else {
		log.Println(" - No specific User IDs are restricted by UserID list.")
	}
	if cfg.ForceJoinChannel != "" {
		log.Printf(" - Mandatory Join Channel: %s", cfg.ForceJoinChannel)
	} else {
		log.Printf(" - No mandatory channel join is configured.")
	}

	var spotifyClient *spotify.Client

	if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
		log.Println("Spotify ClientID and ClientSecret are configured. Initializing Spotify client...")
		spotifyAuthConfig := &clientcredentials.Config{
			ClientID:     cfg.SpotifyClientID,
			ClientSecret: cfg.SpotifyClientSecret,
			TokenURL:     spotifyauth.TokenURL,
		}
		token, tokenErr := spotifyAuthConfig.Token(context.Background())
		if tokenErr != nil {
			log.Printf("ERROR: Couldn't get spotify token: %v. Spotify features will be disabled.", tokenErr)
		} else {
			httpClient := spotifyauth.New().Client(context.Background(), token)
			spotifyClient = spotify.New(httpClient)
			log.Println("Successfully authenticated with Spotify API.")
		}
	} else {
		log.Println("WARNING: SPOTIFY_CLIENT_ID or SPOTIFY_CLIENT_SECRET is not set. Spotify features will be disabled.")
	}

	log.Println("Initializing Downloader...")
	downloaderService, err := downloader.New(cfg)
	if err != nil {
		log.Printf("Error initializing Downloader: %v", err)
		os.Exit(1)
	}
	log.Println("Downloader initialized successfully.")

	log.Println("Initializing Telegram bot...")
	telegramBot, err := bot.New(cfg, downloaderService, spotifyClient)
	if err != nil {
		log.Printf("Error initializing Telegram bot: %v", err)
		os.Exit(1)
	}

	log.Println("Application setup complete. Starting Telegram bot polling...")
	telegramBot.Start()

	log.Println("Bot has stopped.")
}
