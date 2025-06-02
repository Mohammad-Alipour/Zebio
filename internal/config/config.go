package config

import (
	"log"
	"os"
	"strconv" // برای تبدیل رشته به عدد (اگر شناسه کاربر عددی باشه)
	"strings" // برای کار با رشته‌ها
)

// Config ساختاری برای نگهداری تمام تنظیمات برنامه است.
type Config struct {
	TelegramBotToken string
	YTDLPPath        string
	DownloadDir      string
	AllowedUserIDs   []int64 // لیست شناسه‌های کاربری مجاز (اگر از این قابلیت استفاده می‌کنیم)
}

// Load تابع اصلی برای خواندن تنظیمات است.
// این تابع تنظیمات را از متغیرهای محیطی می‌خواند.
func Load() (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Println("WARNING: TELEGRAM_BOT_TOKEN environment variable not set.")
		// در حالت واقعی شاید بخواهیم اینجا خطا برگردانیم و برنامه را متوقف کنیم
		// return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN must be set")
	}

	ytDlpPath := os.Getenv("YTDLP_PATH")
	if ytDlpPath == "" {
		ytDlpPath = "yt-dlp" // مقدار پیش‌فرض اگر تنظیم نشده باشد
		log.Printf("YTDLP_PATH not set, using default: %s\n", ytDlpPath)
	}

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		downloadDir = "temp_downloads" // مقدار پیش‌فرض
		log.Printf("DOWNLOAD_DIR not set, using default: %s\n", downloadDir)
	}

	// خواندن شناسه‌های کاربری مجاز (اختیاری)
	// مثال: ALLOWED_USER_IDS="12345,67890"
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

	return &Config{
		TelegramBotToken: token,
		YTDLPPath:        ytDlpPath,
		DownloadDir:      downloadDir,
		AllowedUserIDs:   allowedUserIDs,
	}, nil
}
