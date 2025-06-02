package bot

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/Mohammad-Alipour/Zebio/internal/config"
	"github.com/Mohammad-Alipour/Zebio/internal/downloader"

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
		userID := int64(0)
		userName := "UnknownUser"
		var chatID int64
		var originalMessageID int = 0

		if update.Message != nil {
			message := update.Message
			userID = message.From.ID
			userName = message.From.UserName
			if userName == "" {
				userName = message.From.FirstName
			}
			chatID = message.Chat.ID
			originalMessageID = message.MessageID
			log.Printf("[%s (%d)] Received message: %s\n", userName, userID, message.Text)

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
					reply := tgbotapi.NewMessage(chatID, "متاسفم، شما اجازه استفاده از این ربات را ندارید.")
					b.api.Send(reply)
					continue
				}
			}

			if message.IsCommand() {
				b.handleCommand(message)
			} else if message.Text != "" {
				b.handleLink(message, userName, userID)
			} else {
				log.Printf("[%s (%d)] Received non-text, non-command message. Ignoring.", userName, userID)
			}

		} else if update.CallbackQuery != nil {
			callback := update.CallbackQuery
			userID = callback.From.ID
			userName = callback.From.UserName
			if userName == "" {
				userName = callback.From.FirstName
			}
			chatID = callback.Message.Chat.ID
			originalMessageID = callback.Message.ReplyToMessage.MessageID // ID of the message with the link

			log.Printf("[%s (%d)] Received callback query data: %s\n", userName, userID, callback.Data)
			b.handleCallbackQuery(callback, userName, userID, originalMessageID)
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
	case "help":
		msgText = "برای دانلود، فقط کافیه لینک مورد نظرتون از سایت‌هایی مثل یوتیوب، ساندکلود، اینستاگرام و ... رو بفرستید.\n"
		msgText += "ربات از شما می‌پرسه که صدا می‌خواید یا ویدیو (اگر محتوا ویدیو باشه).\n"
		msgText += "بعد از انتخاب، فایل براتون ارسال میشه."
	default:
		msgText = "دستور شناخته نشد. برای راهنمایی /help رو بزنید."
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

	processingMsg := tgbotapi.NewMessage(chatID, "در حال دریافت اطلاعات لینک... ℹ️")
	processingMsg.ReplyToMessageID = message.MessageID
	sentPInfoMsg, err := b.api.Send(processingMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'fetching link info' message: %v", userIdentifier, err)
	}

	trackInfo, err := b.downloader.GetTrackInfo(urlToDownload, userIdentifier)
	if err != nil {
		log.Printf("[%s] Error fetching track info for URL %s: %v\n", userIdentifier, urlToDownload, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دریافت اطلاعات از لینک مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ReplyToMessageID = message.MessageID
		b.api.Send(errMsg)
		if sentPInfoMsg.MessageID != 0 {
			b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentPInfoMsg.MessageID))
		}
		return
	}

	if sentPInfoMsg.MessageID != 0 { // Delete "fetching link info" on success
		b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentPInfoMsg.MessageID))
	}

	var keyboard tgbotapi.InlineKeyboardMarkup
	if trackInfo.IsVideo {
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("دانلود صدا 🎵", fmt.Sprintf("dltype:audio:%d", message.MessageID)),
				tgbotapi.NewInlineKeyboardButtonData("دانلود ویدیو 🎬", fmt.Sprintf("dltype:video:%d", message.MessageID)),
			),
		)
	} else if trackInfo.IsAudioOnly {
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("دانلود صدا 🎵", fmt.Sprintf("dltype:audio:%d", message.MessageID)),
			),
		)
	} else {
		// If it's neither video nor audio only (e.g. an image page, or unknown)
		// We might need specific handling for images later. For now, offer audio.
		// Or, if trackInfo.Extension suggests an image, we could have an "Image" button.
		// For now, if yt-dlp -J doesn't give IsVideo or IsAudioOnly, we can assume it might be downloadable as 'best'
		// This part can be refined. For now, if not clearly video/audio, we won't offer buttons and proceed to default download.
		// Let's default to trying audio for now if not clear video.
		log.Printf("[%s] Content type unclear for %s (IsVideo: %t, IsAudioOnly: %t). Defaulting to audio download directly.", userIdentifier, urlToDownload, trackInfo.IsVideo, trackInfo.IsAudioOnly)
		b.processDownloadRequest(message.Chat.ID, message.MessageID, urlToDownload, downloader.AudioOnly, trackInfo, userName, userID)
		return
	}

	choiceMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("یافت شد: %s - %s\nچه نوع دانلودی می‌خواهید؟", trackInfo.Artist, trackInfo.Title))
	choiceMsg.ReplyToMessageID = message.MessageID
	choiceMsg.ReplyMarkup = keyboard
	if _, err := b.api.Send(choiceMsg); err != nil {
		log.Printf("[%s] Error sending download type choice message: %v", userIdentifier, err)
	}
}

func (b *Bot) handleCallbackQuery(callback *tgbotapi.CallbackQuery, userName string, userID int64, originalLinkMessageID int) {
	b.api.Send(tgbotapi.NewCallback(callback.ID, "")) // Acknowledge callback

	chatID := callback.Message.Chat.ID
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)

	// Delete the message with the inline keyboard
	deleteChoiceMsg := tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID)
	if _, err := b.api.Send(deleteChoiceMsg); err != nil {
		log.Printf("[%s] Failed to delete choice message %d: %v", userIdentifier, callback.Message.MessageID, err)
	}

	// Data format: "dltype:<type>:<original_link_msg_id_already_extracted>"
	parts := strings.Split(callback.Data, ":")
	if len(parts) < 2 || parts[0] != "dltype" {
		log.Printf("[%s] Invalid callback data format: %s\n", userIdentifier, callback.Data)
		b.api.Send(tgbotapi.NewMessage(chatID, "خطای داخلی: درخواست نامعتبر."))
		return
	}
	chosenTypeStr := parts[1]

	// Get the original message that contained the link
	// We use callback.Message.ReplyToMessage which should be the user's original link message
	if callback.Message.ReplyToMessage == nil || callback.Message.ReplyToMessage.Text == "" {
		log.Printf("[%s] Could not retrieve original link from ReplyToMessage for callback.\n", userIdentifier)
		b.api.Send(tgbotapi.NewMessage(chatID, "خطای داخلی: لینک اصلی پیدا نشد."))
		return
	}
	urlToDownload := callback.Message.ReplyToMessage.Text

	var dlType downloader.DownloadType
	switch chosenTypeStr {
	case "audio":
		dlType = downloader.AudioOnly
	case "video":
		dlType = downloader.VideoBest
	default:
		log.Printf("[%s] Unknown download type in callback: %s\n", userIdentifier, chosenTypeStr)
		b.api.Send(tgbotapi.NewMessage(chatID, "خطای داخلی: نوع دانلود نامشخص."))
		return
	}

	log.Printf("[%s] User chose %s for URL: %s\n", userIdentifier, chosenTypeStr, urlToDownload)

	// Re-fetch trackInfo as it's simpler than passing it all through callback data
	trackInfo, err := b.downloader.GetTrackInfo(urlToDownload, userIdentifier)
	if err != nil {
		log.Printf("[%s] Error re-fetching track info for URL %s: %v\n", userIdentifier, urlToDownload, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دریافت مجدد اطلاعات از لینک مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ReplyToMessageID = originalLinkMessageID
		b.api.Send(errMsg)
		return
	}

	// Call the actual download and send logic
	b.processDownloadRequest(chatID, originalLinkMessageID, urlToDownload, dlType, trackInfo, userName, userID)
}

func (b *Bot) processDownloadRequest(chatID int64, originalLinkMessageID int, urlToDownload string, dlType downloader.DownloadType, trackInfo *downloader.TrackInfo, userName string, userID int64) {
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)

	downloadingMsgText := fmt.Sprintf("در حال دانلود %s - %s به صورت %s... ⏳", trackInfo.Artist, trackInfo.Title, typeToString(dlType))
	if trackInfo.Title == "Unknown Title" && trackInfo.Artist == "Unknown Artist" {
		downloadingMsgText = fmt.Sprintf("در حال دانلود لینک شما به صورت %s... ⏳", typeToString(dlType))
	}

	dlNoticeMsg := tgbotapi.NewMessage(chatID, downloadingMsgText)
	dlNoticeMsg.ReplyToMessageID = originalLinkMessageID
	sentDlNoticeMsg, err := b.api.Send(dlNoticeMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'downloading media' message: %v", userIdentifier, err)
	}

	downloadedFilePath, actualExt, err := b.downloader.DownloadMedia(urlToDownload, userIdentifier, dlType, trackInfo)
	if err != nil {
		log.Printf("[%s] Error downloading media for URL %s: %v\n", userIdentifier, urlToDownload, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دانلود مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ReplyToMessageID = originalLinkMessageID
		b.api.Send(errMsg)
		if sentDlNoticeMsg.MessageID != 0 {
			b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentDlNoticeMsg.MessageID))
		}
		return
	}

	if sentDlNoticeMsg.MessageID != 0 { // Delete "downloading media" on success
		b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentDlNoticeMsg.MessageID))
	}

	log.Printf("[%s] Media downloaded: %s (ext: %s). Sending to user.\n", userIdentifier, downloadedFilePath, actualExt)

	if trackInfo.ThumbnailURL != "" && (dlType == downloader.AudioOnly || dlType == downloader.VideoBest) { // Send cover for audio/video
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(trackInfo.ThumbnailURL))
		photoMsg.Caption = fmt.Sprintf("%s - %s\nCover Art", trackInfo.Title, trackInfo.Artist)
		if _, err := b.api.Send(photoMsg); err != nil {
			log.Printf("[%s] Error sending cover photo for %s: %v\n", userIdentifier, trackInfo.Title, err)
		} else {
			log.Printf("[%s] Cover art for %s sent successfully.\n", userIdentifier, trackInfo.Title)
		}
	}

	caption := fmt.Sprintf("🎵 %s\n👤 %s\n\n@Zebio_bot", trackInfo.Title, trackInfo.Artist)
	var sentMediaMessage tgbotapi.Message
	var sendErr error

	if dlType == downloader.AudioOnly || actualExt == "mp3" {
		audioFile := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(downloadedFilePath))
		audioFile.ReplyToMessageID = originalLinkMessageID
		audioFile.Title = trackInfo.Title
		audioFile.Performer = trackInfo.Artist
		audioFile.Caption = caption
		sentMediaMessage, sendErr = b.api.Send(audioFile)
	} else if dlType == downloader.VideoBest || actualExt == "mp4" || actualExt == "mkv" || actualExt == "webm" { // Add other video exts if needed
		videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(downloadedFilePath))
		videoFile.ReplyToMessageID = originalLinkMessageID
		videoFile.Caption = caption
		// For video, Title & Performer are not standard fields like in NewAudio. Caption is primary.
		// videoFile.SupportsStreaming = true // Optional
		sentMediaMessage, sendErr = b.api.Send(videoFile)
	} else { // Fallback for other types, try sending as document
		log.Printf("[%s] Unknown/unhandled extension '%s', sending as document.\n", userIdentifier, actualExt)
		docFile := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(downloadedFilePath))
		docFile.ReplyToMessageID = originalLinkMessageID
		docFile.Caption = caption
		sentMediaMessage, sendErr = b.api.Send(docFile)
	}

	if sendErr != nil {
		log.Printf("[%s] Error sending media file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
		errorMsgText := fmt.Sprintf("فایل دانلود شد اما در ارسال آن مشکلی پیش آمد.\nخطا: %s", sendErr.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		b.api.Send(errMsg)
	} else {
		log.Printf("[%s] Media file %s sent successfully.\n", userIdentifier, downloadedFilePath)
	}

	log.Printf("[%s] Attempting to remove temporary file: %s\n", userIdentifier, downloadedFilePath)
	errRemove := os.Remove(downloadedFilePath)
	if errRemove != nil {
		log.Printf("[%s] Error removing temporary file %s: %v\n", userIdentifier, downloadedFilePath, errRemove)
	} else {
		log.Printf("[%s] Temporary file %s removed successfully.\n", userIdentifier, downloadedFilePath)
	}
}

func typeToString(dlType downloader.DownloadType) string {
	if dlType == downloader.AudioOnly {
		return "صدا"
	}
	if dlType == downloader.VideoBest {
		return "ویدیو"
	}
	return "فایل"
}
