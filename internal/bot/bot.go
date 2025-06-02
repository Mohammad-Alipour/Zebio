package bot

import (
	"fmt"
	"github.com/Mohammad-Alipour/Zebio/internal/config"
	"github.com/Mohammad-Alipour/Zebio/internal/downloader"
	"log"
	"os"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api        *tgbotapi.BotAPI
	cfg        *config.Config
	downloader *downloader.Downloader
}

func New(cfg *config.Config, dl *downloader.Downloader) (*Bot, error) {
	if cfg.TelegramBotToken == "" {
		log.Fatal("Telegram Bot Token is not configured. Cannot start bot.")
	}
	api, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Bot API: %w", err)
	}
	log.Printf("Authorized on account %s (@%s)\n", api.Self.FirstName, api.Self.UserName)
	return &Bot{
		api:        api,
		cfg:        cfg,
		downloader: dl,
	}, nil
}

func (b *Bot) Start() {
	log.Println("Bot is starting to listen for updates...")
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		userID := update.Message.From.ID
		userName := update.Message.From.UserName
		if userName == "" {
			userName = update.Message.From.FirstName
		}
		log.Printf("[%s (%d)] Received message: %s\n", userName, userID, update.Message.Text)

		if len(b.cfg.AllowedUserIDs) > 0 {
			isAllowed := false
			for _, allowedID := range b.cfg.AllowedUserIDs {
				if userID == allowedID {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				log.Printf("User %s (%d) is not allowed. Ignoring message.", userName, userID)
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, "متاسفم، شما اجازه استفاده از این ربات را ندارید.")
				b.api.Send(reply)
				continue
			}
		}

		if update.Message.IsCommand() {
			b.handleCommand(update.Message)
		} else if update.Message.Text != "" {
			b.handleLink(update.Message, userName, userID)
		} else {
			log.Printf("[%s (%d)] Received non-text, non-command message. Ignoring.", userName, userID)
		}
	}
}

func (b *Bot) handleCommand(message *tgbotapi.Message) {
	userName := message.From.UserName
	if userName == "" {
		userName = message.From.FirstName
	}
	command := message.Command()
	log.Printf("[%s (%d)] Received command: /%s\n", userName, message.From.ID, command)
	var msgText string
	switch command {
	case "start":
		msgText = "سلام " + message.From.FirstName + "! 👋\n"
		msgText += "من ربات دانلودر شما هستم. لینک خود را برای دانلود ارسال کنید."
	default:
		msgText = "دستور شناخته نشد."
	}
	reply := tgbotapi.NewMessage(message.Chat.ID, msgText)
	reply.ReplyToMessageID = message.MessageID
	if _, err := b.api.Send(reply); err != nil {
		log.Printf("[%s (%d)] Error sending command reply: %v", userName, message.From.ID, err)
	}
}

func (b *Bot) handleLink(message *tgbotapi.Message, userName string, userID int64) {
	chatID := message.Chat.ID
	urlToDownload := message.Text
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)

	log.Printf("[%s] Received link to process: %s\n", userIdentifier, urlToDownload)

	processingMsg := tgbotapi.NewMessage(chatID, "در حال دریافت اطلاعات آهنگ... لطفاً کمی صبر کنید. ℹ️")
	processingMsg.ReplyToMessageID = message.MessageID
	sentProcessingMsg, err := b.api.Send(processingMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'fetching info' message: %v", userIdentifier, err)
	}

	trackInfo, err := b.downloader.GetTrackInfo(urlToDownload, userIdentifier)
	if err != nil {
		log.Printf("[%s] Error fetching track info for URL %s: %v\n", userIdentifier, urlToDownload, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دریافت اطلاعات آهنگ از لینک مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ReplyToMessageID = message.MessageID
		b.api.Send(errMsg)
		if sentProcessingMsg.MessageID != 0 { // Delete "fetching info" on error too
			deleteMsgConfig := tgbotapi.NewDeleteMessage(chatID, sentProcessingMsg.MessageID)
			b.api.Send(deleteMsgConfig)
		}
		return
	}

	if sentProcessingMsg.MessageID != 0 {
		deleteMsgConfig := tgbotapi.NewDeleteMessage(chatID, sentProcessingMsg.MessageID)
		if _, delErr := b.api.Send(deleteMsgConfig); delErr != nil {
			log.Printf("[%s] Failed to delete 'fetching info' message %d: %v", userIdentifier, sentProcessingMsg.MessageID, delErr)
		}
	}

	downloadingMsgText := fmt.Sprintf("اطلاعات دریافت شد: %s - %s\nدر حال دانلود... ⏳", trackInfo.Artist, trackInfo.Title)
	if trackInfo.Title == "Unknown Title" && trackInfo.Artist == "Unknown Artist" { // if GetTrackInfo returned defaults
		downloadingMsgText = "در حال دانلود لینک شما... ⏳"
	}
	downloadingMsg := tgbotapi.NewMessage(chatID, downloadingMsgText)
	downloadingMsg.ReplyToMessageID = message.MessageID
	sentDownloadingMsg, err := b.api.Send(downloadingMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'downloading' message: %v", userIdentifier, err)
	}

	downloadedFilePath, err := b.downloader.DownloadAudio(urlToDownload, userIdentifier, trackInfo)
	if err != nil {
		log.Printf("[%s] Error downloading audio for URL %s: %v\n", userIdentifier, urlToDownload, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دانلود آهنگ مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ReplyToMessageID = message.MessageID
		b.api.Send(errMsg)
		if sentDownloadingMsg.MessageID != 0 {
			deleteMsgConfig := tgbotapi.NewDeleteMessage(chatID, sentDownloadingMsg.MessageID)
			b.api.Send(deleteMsgConfig)
		}
		return
	}

	log.Printf("[%s] File downloaded successfully: %s. Attempting to send.\n", userIdentifier, downloadedFilePath)

	if sentDownloadingMsg.MessageID != 0 {
		deleteMsgConfig := tgbotapi.NewDeleteMessage(chatID, sentDownloadingMsg.MessageID)
		if _, delErr := b.api.Send(deleteMsgConfig); delErr != nil {
			log.Printf("[%s] Failed to delete 'downloading' message %d: %v", userIdentifier, sentDownloadingMsg.MessageID, delErr)
		}
	}

	if trackInfo.ThumbnailURL != "" {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(trackInfo.ThumbnailURL))
		photoMsg.Caption = fmt.Sprintf("%s - %s\nCover Art", trackInfo.Title, trackInfo.Artist)
		if _, err := b.api.Send(photoMsg); err != nil {
			log.Printf("[%s] Error sending cover photo for %s: %v\n", userIdentifier, trackInfo.Title, err)
		} else {
			log.Printf("[%s] Cover art for %s sent successfully.\n", userIdentifier, trackInfo.Title)
		}
	}

	audioFile := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(downloadedFilePath))
	audioFile.ReplyToMessageID = message.MessageID
	audioFile.Title = trackInfo.Title
	audioFile.Performer = trackInfo.Artist
	audioFile.Caption = fmt.Sprintf("🎵 %s\n👤 %s\n\n@Zebio_bot", trackInfo.Title, trackInfo.Artist)

	if _, err := b.api.Send(audioFile); err != nil {
		log.Printf("[%s] Error sending audio file %s: %v\n", userIdentifier, downloadedFilePath, err)
		errorMsgText := fmt.Sprintf("فایل دانلود شد اما در ارسال آن مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		b.api.Send(errMsg)
	} else {
		log.Printf("[%s] Audio file %s sent successfully with metadata and caption.\n", userIdentifier, downloadedFilePath)
	}

	log.Printf("[%s] Attempting to remove temporary file: %s\n", userIdentifier, downloadedFilePath)
	errRemove := os.Remove(downloadedFilePath)
	if errRemove != nil {
		log.Printf("[%s] Error removing temporary file %s: %v\n", userIdentifier, downloadedFilePath, errRemove)
	} else {
		log.Printf("[%s] Temporary file %s removed successfully.\n", userIdentifier, downloadedFilePath)
	}
}
