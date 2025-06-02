package downloader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mohammad-Alipour/Zebio/internal/config"
)

type Downloader struct {
	ytDLPPath   string
	downloadDir string
}

type TrackInfo struct {
	Title        string
	Artist       string
	ThumbnailURL string
}

func New(cfg *config.Config) (*Downloader, error) {
	if _, err := os.Stat(cfg.DownloadDir); os.IsNotExist(err) {
		log.Printf("Download directory '%s' does not exist. Creating it...\n", cfg.DownloadDir)
		if err := os.MkdirAll(cfg.DownloadDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create download directory '%s': %w", cfg.DownloadDir, err)
		}
		log.Printf("Download directory '%s' created successfully.\n", cfg.DownloadDir)
	} else if err != nil {
		return nil, fmt.Errorf("error checking download directory '%s': %w", cfg.DownloadDir, err)
	}
	return &Downloader{
		ytDLPPath:   cfg.YTDLPPath,
		downloadDir: cfg.DownloadDir,
	}, nil
}

func (d *Downloader) GetTrackInfo(urlStr string, username string) (*TrackInfo, error) {
	log.Printf("[%s] Fetching track info for URL: %s\n", username, urlStr)
	cmd := exec.Command(d.ytDLPPath,
		"-J",
		"--no-playlist",
		urlStr,
	)
	var jsonData bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &jsonData
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if stderrBuf.Len() > 0 {
		log.Printf("[%s] yt-dlp (info) STDERR for %s:\n%s\n", username, urlStr, stderrBuf.String())
	}
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to get track info from yt-dlp for %s: %w", username, urlStr, err)
	}

	var data struct {
		Title     string `json:"title"`
		Artist    string `json:"artist"`
		Creator   string `json:"creator"`
		Uploader  string `json:"uploader"`
		Thumbnail string `json:"thumbnail"`
	}

	if err := json.Unmarshal(jsonData.Bytes(), &data); err != nil {
		return nil, fmt.Errorf("[%s] failed to unmarshal track info JSON for %s: %w", username, urlStr, err)
	}

	info := &TrackInfo{
		Title:        data.Title,
		Artist:       data.Artist,
		ThumbnailURL: data.Thumbnail,
	}

	if info.Artist == "" {
		if data.Creator != "" {
			info.Artist = data.Creator
		} else {
			info.Artist = data.Uploader
		}
	}
	if info.Artist == "" {
		info.Artist = "Unknown Artist"
	}
	if info.Title == "" {
		info.Title = "Unknown Title"
	}

	log.Printf("[%s] Track info fetched: Title: '%s', Artist: '%s', Thumbnail: '%s'\n", username, info.Title, info.Artist, info.ThumbnailURL)
	return info, nil
}

func (d *Downloader) DownloadAudio(urlStr string, username string, info *TrackInfo) (string, error) {
	log.Printf("[%s] Starting audio download for URL: %s (Title: %s)\n", username, urlStr, info.Title)
	start := time.Now()

	fileName := fmt.Sprintf("%s - %s.mp3", info.Artist, info.Title)
	outputTemplate := filepath.Join(d.downloadDir, fileName)

	cmdArgs := []string{
		"-v",
		"--no-playlist",
		"-f", "bestaudio/best",
		"--extract-audio",
		"--audio-format", "mp3",
		"--restrict-filenames",
		"--embed-thumbnail",
		"-o", outputTemplate,
		urlStr,
	}
	cmd := exec.Command(d.ytDLPPath, cmdArgs...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Printf("[%s] Executing yt-dlp download command: %s\n", username, strings.Join(cmd.Args, " "))

	err := cmd.Run()

	if stdoutBuf.Len() > 0 {
		log.Printf("[%s] yt-dlp (download) STDOUT:\n%s\n", username, stdoutBuf.String())
	}
	if stderrBuf.Len() > 0 {
		log.Printf("[%s] yt-dlp (download) STDERR:\n%s\n", username, stderrBuf.String())
	}

	if err != nil {
		return "", fmt.Errorf("[%s] yt-dlp download execution failed: %w. STDERR: %s", username, err, stderrBuf.String())
	}

	finalFilePath := outputTemplate
	if _, statErr := os.Stat(finalFilePath); os.IsNotExist(statErr) {
		log.Printf("[%s] File '%s' not found directly. Trying to find latest MP3.\n", username, finalFilePath)
		foundPath, findErr := findLatestMP3(d.downloadDir, username)
		if findErr != nil {
			return "", fmt.Errorf("[%s] yt-dlp ran but downloaded file could not be found: %w", username, findErr)
		}
		finalFilePath = foundPath
	}

	elapsed := time.Since(start)
	log.Printf("[%s] Audio download and processing for %s finished in %s. File: %s\n", username, urlStr, elapsed, finalFilePath)
	return finalFilePath, nil
}

func findLatestMP3(dir, username string) (string, error) {
	log.Printf("[%s] findLatestMP3: Scanning directory '%s' for .mp3 files\n", username, dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", dir, err)
	}
	var latestFile string
	var latestModTime time.Time
	foundOne := false
	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".mp3") {
			filePath := filepath.Join(dir, fileName)
			info, statErr := os.Stat(filePath)
			if statErr != nil {
				log.Printf("[%s] findLatestMP3: Stat failed for %s: %v. Skipping.\n", username, filePath, statErr)
				continue
			}
			log.Printf("[%s] findLatestMP3: Found MP3: %s, ModTime: %s\n", username, filePath, info.ModTime())
			if !foundOne || info.ModTime().After(latestModTime) {
				latestModTime = info.ModTime()
				latestFile = filePath
				foundOne = true
			}
		}
	}
	if !foundOne {
		return "", fmt.Errorf("no .mp3 file found in directory %s after download", dir)
	}
	log.Printf("[%s] findLatestMP3: Latest MP3 file selected: %s\n", username, latestFile)
	return latestFile, nil
}
