package main

import (
	"log"
	"os"

	// مسیر صحیح به پکیج‌های داخلی شما
	"github.com/Mohammad-Alipour/Zebio/internal/bot" // پکیج ربات رو اضافه می‌کنیم
	"github.com/Mohammad-Alipour/Zebio/internal/config"
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
		os.Exit(1) // اگر توکن نباشد، برنامه باید خارج شود
	} else {
		log.Println("Telegram Bot Token is set.") // طول توکن را دیگر اینجا چاپ نمی‌کنیم
	}
	if len(cfg.AllowedUserIDs) > 0 {
		log.Printf(" - Allowed User IDs: %v", cfg.AllowedUserIDs)
	} else {
		log.Println(" - No specific User IDs are restricted.")
	}

	// 2. راه‌اندازی و اجرای ربات تلگرام
	log.Println("Initializing Telegram bot...")
	telegramBot, err := bot.New(cfg) // نمونه ربات را می‌سازیم
	if err != nil {
		log.Printf("Error initializing Telegram bot: %v", err)
		os.Exit(1)
	}

	// 3. (قدم‌های بعدی) آماده‌سازی دانلودر با استفاده از cfg.YTDLPPath و cfg.DownloadDir
	log.Println("Downloader initialization is a placeholder for now.")
	// downloaderService := downloader.New(cfg) // چیزی شبیه این

	log.Println("Application setup complete. Starting Telegram bot polling...")
	// این تابع برنامه را در حال اجرا نگه می‌دارد و به پیام‌ها گوش می‌دهد
	telegramBot.Start()

	log.Println("Bot has stopped.") // این خط معمولاً اجرا نمی‌شود مگر اینکه Start() خاتمه یابد
}
