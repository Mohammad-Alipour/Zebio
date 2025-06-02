package downloader

import (
	"bytes"
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

func (d *Downloader) DownloadAudio(urlStr string, username string) (string, error) {
	log.Printf("[%s] Starting audio download for URL: %s\n", username, urlStr)
	start := time.Now()

	outputTemplate := filepath.Join(d.downloadDir, "%(title)s.%(ext)s")

	cmd := exec.Command(d.ytDLPPath,
		"-v",
		"--no-playlist",
		"-f", "bestaudio/best",
		"--extract-audio",
		"--audio-format", "mp3",
		"--restrict-filenames",
		"-o", outputTemplate,
		urlStr,
	)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Printf("[%s] Executing yt-dlp command: %s\n", username, strings.Join(cmd.Args, " "))

	err := cmd.Run()

	if stdoutBuf.Len() > 0 {
		log.Printf("[%s] yt-dlp STDOUT:\n%s\n", username, stdoutBuf.String())
	}
	if stderrBuf.Len() > 0 {
		log.Printf("[%s] yt-dlp STDERR:\n%s\n", username, stderrBuf.String())
	}

	if err != nil {
		return "", fmt.Errorf("[%s] yt-dlp execution failed: %w. STDERR: %s", username, err, stderrBuf.String())
	}

	downloadedFilePath, err := findLatestMP3(d.downloadDir, username)
	if err != nil {
		return "", fmt.Errorf("[%s] failed to find downloaded mp3 file: %w", username, err)
	}

	elapsed := time.Since(start)
	log.Printf("[%s] Audio download and processing for %s finished in %s. File: %s\n", username, urlStr, elapsed, downloadedFilePath)
	return downloadedFilePath, nil
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
