package main

import (
	"log"
	"os"

	"github.com/Mohammad-Alipour/Zebio/internal/bot"
	"github.com/Mohammad-Alipour/Zebio/internal/config"
	"github.com/Mohammad-Alipour/Zebio/internal/downloader"
)

func main() {
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
	if cfg.ForceJoinChannel != "" { // <--- لاگ کردن تنظیمات کانال
		log.Printf(" - Mandatory Join Channel: %s", cfg.ForceJoinChannel)
	} else {
		log.Printf(" - No mandatory channel join is configured.")
	}

	log.Println("Initializing Downloader...")
	downloaderService, err := downloader.New(cfg)
	if err != nil {
		log.Printf("Error initializing Downloader: %v", err)
		os.Exit(1)
	}
	log.Println("Downloader initialized successfully.")

	log.Println("Initializing Telegram bot...")
	telegramBot, err := bot.New(cfg, downloaderService)
	if err != nil {
		log.Printf("Error initializing Telegram bot: %v", err)
		os.Exit(1)
	}

	log.Println("Application setup complete. Starting Telegram bot polling...")
	telegramBot.Start()

	log.Println("Bot has stopped.")
}
