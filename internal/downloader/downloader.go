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

type DownloadType int

const (
	AudioOnly DownloadType = iota
	VideoBest
	ImageBest
)

type Downloader struct {
	ytDLPPath   string
	downloadDir string
}

type TrackInfo struct {
	Title          string
	Artist         string
	ThumbnailURL   string
	Extension      string
	Filename       string
	OriginalURL    string
	URL            string
	HasVideo       bool
	HasImage       bool
	IsAudioOnly    bool
	DirectImageURL string
}

type LinkInfo struct {
	Type        string
	Title       string
	Uploader    string
	Tracks      []*TrackInfo
	OriginalURL string
}

type ytdlpJSONEntry struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Creator      string `json:"creator"`
	Uploader     string `json:"uploader"`
	Thumbnail    string `json:"thumbnail"`
	DisplayURL   string `json:"display_url"`
	URL          string `json:"url"`
	Ext          string `json:"ext"`
	Vcodec       string `json:"vcodec"`
	Acodec       string `json:"acodec"`
	ExtractorKey string `json:"extractor_key"`
	Filename     string `json:"_filename"`
	WebpageURL   string `json:"webpage_url"`
	Thumbnails   []struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"thumbnails"`
}

type ytdlpPlaylistJSON struct {
	Type       string           `json:"_type"`
	Title      string           `json:"title"`
	Uploader   string           `json:"uploader"`
	WebpageURL string           `json:"webpage_url"`
	Entries    []ytdlpJSONEntry `json:"entries"`
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

func (d *Downloader) GetLinkInfo(urlStr string, username string) (*LinkInfo, error) {
	log.Printf("[%s] Fetching link info for URL: %s\n", username, urlStr)

	var cmd *exec.Cmd
	if strings.Contains(urlStr, "soundcloud.com") && (strings.Contains(urlStr, "/sets/") || strings.Contains(urlStr, "/playlists/")) {
		cmd = exec.Command(d.ytDLPPath, "-J", "-i", "--flat-playlist", urlStr)
	} else {
		cmd = exec.Command(d.ytDLPPath, "-J", "-i", "--no-playlist", urlStr)
	}

	var jsonData bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &jsonData
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if stderrBuf.Len() > 0 {
		log.Printf("[%s] yt-dlp (info) STDERR for %s:\n%s\n", username, urlStr, stderrBuf.String())
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, fmt.Errorf("[%s] failed to run yt-dlp for %s: %w", username, urlStr, err)
		}
	}

	if jsonData.Len() == 0 {
		return nil, fmt.Errorf("[%s] yt-dlp returned no JSON data for %s", username, urlStr)
	}

	var output ytdlpPlaylistJSON
	if err := json.Unmarshal(jsonData.Bytes(), &output); err == nil && output.Type == "playlist" {
		linkInfo := &LinkInfo{
			Type:        "album",
			Title:       output.Title,
			Uploader:    output.Uploader,
			OriginalURL: output.WebpageURL,
		}
		for _, entryData := range output.Entries {
			track := parseTrackInfoFromData(entryData)
			linkInfo.Tracks = append(linkInfo.Tracks, track)
		}
		log.Printf("[%s] Album/Playlist info fetched: Title: '%s', Track Count: %d\n", username, linkInfo.Title, len(linkInfo.Tracks))
		return linkInfo, nil
	}

	var singleEntry ytdlpJSONEntry
	if err := json.Unmarshal(jsonData.Bytes(), &singleEntry); err != nil {
		return nil, fmt.Errorf("[%s] failed to unmarshal yt-dlp JSON for %s: %w", username, urlStr, err)
	}

	track := parseTrackInfoFromData(singleEntry)
	linkInfo := &LinkInfo{
		Type:        "track",
		Title:       track.Title,
		Uploader:    track.Artist,
		OriginalURL: track.OriginalURL,
		Tracks:      []*TrackInfo{track},
	}
	log.Printf("[%s] Single track info fetched: Title: '%s', Artist: '%s'\n", username, track.Title, track.Artist)
	return linkInfo, nil
}

func parseTrackInfoFromData(data ytdlpJSONEntry) *TrackInfo {
	info := &TrackInfo{
		Title:        data.Title,
		Artist:       data.Artist,
		ThumbnailURL: data.Thumbnail,
		Extension:    data.Ext,
		Filename:     data.Filename,
		OriginalURL:  data.WebpageURL,
		URL:          data.URL,
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
		if data.ID != "" {
			info.Title = data.ID
		} else {
			info.Title = "Unknown Title"
		}
	}

	if data.Vcodec != "none" && data.Vcodec != "" {
		info.HasVideo = true
	}

	if data.ExtractorKey == "Instagram" && !info.HasVideo {
		info.HasImage = true
		if data.DisplayURL != "" {
			info.DirectImageURL = data.DisplayURL
		} else if data.URL != "" && (strings.HasSuffix(data.URL, ".jpg") || strings.HasSuffix(data.URL, ".jpeg")) {
			info.DirectImageURL = data.URL
		} else if len(data.Thumbnails) > 0 {
			var bestThumbnailURL string
			var maxWidth int = 0
			for _, t := range data.Thumbnails {
				if t.Width > maxWidth {
					maxWidth = t.Width
					bestThumbnailURL = t.URL
				}
			}
			info.DirectImageURL = bestThumbnailURL
		}
	}

	if !info.HasVideo && !info.HasImage && data.Acodec != "none" && data.Acodec != "" {
		info.IsAudioOnly = true
	}
	return info
}

func (d *Downloader) FindYouTubeURL(query string, username string) (string, error) {
	log.Printf("[%s] Searching on YouTube for: %s", username, query)
	cmd := exec.Command(d.ytDLPPath, "--get-url", fmt.Sprintf("ytsearch1:%s", query))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[%s] yt-dlp Youtube failed. STDERR: %s", username, stderr.String())
		return "", fmt.Errorf("could not find a youtube video for query '%s': %w", query, err)
	}

	youtubeURL := strings.TrimSpace(stdout.String())
	if youtubeURL == "" {
		return "", fmt.Errorf("yt-dlp Youtube returned an empty URL for query '%s'", query)
	}

	log.Printf("[%s] Youtube found URL: %s", username, youtubeURL)
	return youtubeURL, nil
}

func (d *Downloader) FindSoundCloudURL(query string, username string) (string, error) {
	log.Printf("[%s] Searching on SoundCloud for: %s", username, query)
	cmd := exec.Command(d.ytDLPPath, "-J", "--no-playlist", fmt.Sprintf("scsearch1:%s", query))

	var jsonData, stderr bytes.Buffer
	cmd.Stdout = &jsonData
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[%s] yt-dlp SoundCloud search failed. STDERR: %s", username, stderr.String())
		return "", fmt.Errorf("could not find a soundcloud track for query '%s': %w", query, err)
	}

	var searchResult struct {
		Entries []struct {
			WebpageURL string `json:"webpage_url"`
		} `json:"entries"`
	}

	if err := json.Unmarshal(jsonData.Bytes(), &searchResult); err != nil {
		return "", fmt.Errorf("could not parse soundcloud search result: %w", err)
	}

	if len(searchResult.Entries) == 0 || searchResult.Entries[0].WebpageURL == "" {
		return "", fmt.Errorf("yt-dlp SoundCloud search returned no results for query '%s'", query)
	}

	soundcloudURL := strings.TrimSpace(searchResult.Entries[0].WebpageURL)
	log.Printf("[%s] SoundCloud search found URL: %s", username, soundcloudURL)
	return soundcloudURL, nil
}

func (d *Downloader) DownloadMedia(urlStr string, username string, prefType DownloadType, info *TrackInfo) (string, string, error) {
	log.Printf("[%s] Starting download for URL: %s (Title: %s, Preferred Type: %v)\n", username, urlStr, info.Title, prefType)
	start := time.Now()

	var cmdArgs []string
	outputFilename := fmt.Sprintf("%s - %s", info.Artist, info.Title)
	outputTemplateBase := filepath.Join(d.downloadDir, outputFilename)

	downloadURL := urlStr
	if prefType != ImageBest && info.URL != "" {
		downloadURL = info.URL
	}

	switch prefType {
	case AudioOnly:
		cmdArgs = []string{
			"-v", "--no-playlist", "-f", "bestaudio/best", "--extract-audio",
			"--audio-format", "mp3", "--restrict-filenames", "--embed-thumbnail",
			"-o", outputTemplateBase + ".%(ext)s", downloadURL,
		}
	case VideoBest:
		cmdArgs = []string{
			"-v", "--no-playlist", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
			"--merge-output-format", "mp4", "--restrict-filenames", "--embed-thumbnail",
			"-o", outputTemplateBase + ".%(ext)s", downloadURL,
		}
	case ImageBest:
		if info.DirectImageURL != "" {
			downloadURL = info.DirectImageURL
		}
		cmdArgs = []string{
			"-v", "--no-playlist", "--restrict-filenames",
			"-o", outputTemplateBase + ".%(ext)s", downloadURL,
		}
	default:
		return "", "", fmt.Errorf("[%s] unknown download type requested", username)
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
		return "", "", fmt.Errorf("[%s] yt-dlp download execution failed: %w. STDERR: %s", username, err, stderrBuf.String())
	}

	actualFilename, findErr := findDownloadedFile(d.downloadDir, outputFilename, username)
	if findErr != nil {
		return "", "", fmt.Errorf("[%s] yt-dlp ran but downloaded file could not be reliably found (basename: %s): %w", username, outputFilename, findErr)
	}

	detectedExt := strings.TrimPrefix(filepath.Ext(actualFilename), ".")
	elapsed := time.Since(start)
	log.Printf("[%s] Download and processing for %s finished in %s. File: %s, Actual Ext: %s\n", username, urlStr, elapsed, actualFilename, detectedExt)
	return actualFilename, detectedExt, nil
}

func findDownloadedFile(dir, baseName, username string) (string, error) {
	log.Printf("[%s] findDownloadedFile: Scanning directory '%s' for files starting with '%s'\n", username, dir, baseName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var latestFile string
	var latestModTime time.Time
	var fileMatched bool = false
	var foundFiles []string

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), baseName) {
			filePath := filepath.Join(dir, entry.Name())
			info, statErr := os.Stat(filePath)
			if statErr != nil {
				log.Printf("[%s] findDownloadedFile: Stat failed for %s: %v. Skipping.\n", username, filePath, statErr)
				continue
			}
			if !fileMatched || info.ModTime().After(latestModTime) {
				latestModTime = info.ModTime()
				latestFile = filePath
				fileMatched = true
			}
			foundFiles = append(foundFiles, filePath)
		}
	}

	if !fileMatched {
		return "", fmt.Errorf("no file found starting with basename '%s' in directory '%s'", baseName, dir)
	}

	if len(foundFiles) > 1 {
		log.Printf("[%s] Warning: Multiple files found starting with basename '%s': %v. Selected latest: %s\n", username, baseName, foundFiles, latestFile)
	}

	log.Printf("[%s] findDownloadedFile: File selected: %s\n", username, latestFile)
	return latestFile, nil
}
