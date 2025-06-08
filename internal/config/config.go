package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TelegramBotToken    string
	YTDLPPath           string
	DownloadDir         string
	AllowedUserIDs      []int64
	ForceJoinChannel    string
	SpotifyClientID     string
	SpotifyClientSecret string
	YouTubeCookiesPath  string
}

func Load() (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Println("WARNING: TELEGRAM_BOT_TOKEN environment variable not set.")
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
		if len(allowedUserIDs) > 0 {
			log.Printf("Allowed user IDs loaded: %v\n", allowedUserIDs)
		}
	} else {
		log.Println("ALLOWED_USER_IDS not set. Bot will be open to all (if no other checks are in place).")
	}

	forceJoinChannel := os.Getenv("FORCE_JOIN_CHANNEL")
	if forceJoinChannel != "" {
		if !strings.HasPrefix(forceJoinChannel, "@") {
			forceJoinChannel = "@" + forceJoinChannel
		}
		log.Printf("Force join channel configured: %s\n", forceJoinChannel)
	} else {
		log.Println("FORCE_JOIN_CHANNEL not set. No mandatory channel join required.")
	}

	spotifyClientID := os.Getenv("SPOTIFY_CLIENT_ID")
	spotifyClientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")

	if spotifyClientID == "" || spotifyClientSecret == "" {
		log.Println("WARNING: SPOTIFY_CLIENT_ID or SPOTIFY_CLIENT_SECRET is not set. Spotify features will be disabled.")
	} else {
		log.Println("Spotify ClientID and ClientSecret loaded successfully.")
	}

	youTubeCookiesPath := os.Getenv("YOUTUBE_COOKIES_PATH")
	if youTubeCookiesPath != "" {
		log.Printf("YouTube cookies file path loaded: %s\n", youTubeCookiesPath)
	} else {
		log.Println("Warning: YOUTUBE_COOKIES_PATH not set. Youtubees may fail due to bot detection.")
	}

	return &Config{
		TelegramBotToken:    token,
		YTDLPPath:           ytDlpPath,
		DownloadDir:         downloadDir,
		AllowedUserIDs:      allowedUserIDs,
		ForceJoinChannel:    forceJoinChannel,
		SpotifyClientID:     spotifyClientID,
		SpotifyClientSecret: spotifyClientSecret,
		YouTubeCookiesPath:  youTubeCookiesPath,
	}, nil
}
