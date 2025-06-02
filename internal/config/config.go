package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	// "fmt" // اگر برای خطا در توکن لازم شد
)

type Config struct {
	TelegramBotToken string
	YTDLPPath        string
	DownloadDir      string
	AllowedUserIDs   []int64
	ForceJoinChannel string // <--- فیلد جدید برای کانال جوین اجباری
}

func Load() (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Println("WARNING: TELEGRAM_BOT_TOKEN environment variable not set.")
		// return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN must be set")
	}

	ytDlpPath := os.Getenv("YTDLP_PATH")
	if ytDlpPath == "" {
		ytDlpPath = "yt-dlp"
		log.Printf("YTDLP_PATH not set, using default: %s\n", ytDlpPath)
	}

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		downloadDir = "temp_downloads"
		log.Printf("DOWNLOAD_DIR not set, using default: %s\n", downloadDir)
	}

	var allowedUserIDs []int64
	allowedUserIDsStr := os.Getenv("ALLOWED_USER_IDS")
	if allowedUserIDsStr != "" {
		ids := strings.Split(allowedUserIDsStr, ",")
		for _, idStr := range ids {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				log.Printf("Warning: Could not parse user ID '%s': %v. Skipping.\n", idStr, err)
				continue
			}
			allowedUserIDs = append(allowedUserIDs, id)
		}
		log.Printf("Allowed user IDs loaded: %v\n", allowedUserIDs)
	} else {
		log.Println("ALLOWED_USER_IDS not set. Bot will be open to all (if no other checks are in place).")
	}

	forceJoinChannel := os.Getenv("FORCE_JOIN_CHANNEL") // <--- خواندن از متغیر محیطی
	if forceJoinChannel != "" {
		// برای اطمینان، اگر @ در ابتدا نبود، اضافه می‌کنیم (هرچند بهتره با @ تنظیم بشه)
		if !strings.HasPrefix(forceJoinChannel, "@") {
			forceJoinChannel = "@" + forceJoinChannel
		}
		log.Printf("Force join channel configured: %s\n", forceJoinChannel)
	} else {
		log.Println("FORCE_JOIN_CHANNEL not set. No mandatory channel join required.")
	}

	return &Config{
		TelegramBotToken: token,
		YTDLPPath:        ytDlpPath,
		DownloadDir:      downloadDir,
		AllowedUserIDs:   allowedUserIDs,
		ForceJoinChannel: forceJoinChannel, // <--- اضافه کردن به ساختار خروجی
	}, nil
}
