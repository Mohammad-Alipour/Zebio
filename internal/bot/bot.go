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

func (b *Bot) isUserMemberOfRequiredChannel(userID int64) (bool, string, error) {
	if b.cfg.ForceJoinChannel == "" {
		return true, "", nil
	}

	channelUsername := b.cfg.ForceJoinChannel

	chatMemberConfig := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			SuperGroupUsername: channelUsername,
			UserID:             userID,
		},
	}
	member, err := b.api.GetChatMember(chatMemberConfig)
	if err != nil {
		if apiErr, ok := err.(tgbotapi.Error); ok && (strings.Contains(strings.ToLower(apiErr.Message), "user not found") || strings.Contains(strings.ToLower(apiErr.Message), "member not found")) {
			log.Printf("User %d not found in channel %s (implies not a member)", userID, channelUsername)
			return false, channelUsername, nil
		}
		log.Printf("Error checking chat member status for user %d in channel %s: %v", userID, channelUsername, err)
		return false, channelUsername, err
	}

	switch member.Status {
	case "creator", "administrator", "member":
		return true, channelUsername, nil
	default:
		log.Printf("User %d has status '%s' in channel %s (not considered a member for usage).", userID, member.Status, channelUsername)
		return false, channelUsername, nil
	}
}

func (b *Bot) sendJoinChannelMessage(chatID int64, channelUsername string, replyToMessageID int) {
	channelLink := "https://t.me/" + strings.TrimPrefix(channelUsername, "@")
	replyText := fmt.Sprintf("⚠️ برای استفاده از امکانات ربات، لطفاً ابتدا در کانال ما عضو شوید:\n%s\n\nپس از عضویت، دوباره دستور خود را ارسال کنید یا /start را بزنید.", channelLink)

	reply := tgbotapi.NewMessage(chatID, replyText)
	if replyToMessageID != 0 {
		reply.ReplyToMessageID = replyToMessageID
	}

	joinButton := tgbotapi.NewInlineKeyboardButtonURL("عضویت در کانال 🚀", channelLink)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(joinButton))
	reply.ReplyMarkup = keyboard

	if _, err := b.api.Send(reply); err != nil {
		log.Printf("Error sending 'please join channel' message to chat %d: %v", chatID, err)
	}
}

func (b *Bot) Start() {
	log.Println("Bot is starting to listen for updates...")
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		var userID int64
		var userName string
		var chatID int64
		var messageID int = 0 // ID of the user's message, if applicable
		var isCallback bool = false

		if update.Message != nil {
			message := update.Message
			userID = message.From.ID
			userName = message.From.UserName
			if userName == "" {
				userName = message.From.FirstName
			}
			chatID = message.Chat.ID
			messageID = message.MessageID
			log.Printf("[%s (%d)] Received message: %s\n", userName, userID, message.Text)
		} else if update.CallbackQuery != nil {
			isCallback = true
			callback := update.CallbackQuery
			userID = callback.From.ID
			userName = callback.From.UserName
			if userName == "" {
				userName = callback.From.FirstName
			}
			chatID = callback.Message.Chat.ID
			// For callbacks, the relevant message to reply to (if needed for context) might be callback.Message.ReplyToMessage
			if callback.Message.ReplyToMessage != nil {
				messageID = callback.Message.ReplyToMessage.MessageID
			} else {
				messageID = callback.Message.MessageID // Fallback to the message with buttons
			}
			log.Printf("[%s (%d)] Received callback query data: %s from message %d\n", userName, userID, callback.Data, callback.Message.MessageID)
		} else {
			continue
		}

		if b.cfg.ForceJoinChannel != "" {
			isMember, channelToJoin, err := b.isUserMemberOfRequiredChannel(userID)
			if err != nil {
				log.Printf("Error during channel membership check for user %d: %v. Sending error message.", userID, err)
				reply := tgbotapi.NewMessage(chatID, "خطا در بررسی عضویت کانال. لطفاً لحظاتی دیگر دوباره امتحان کنید.")
				if messageID != 0 && !isCallback { // Only reply to original message if it's not a callback's inline message
					reply.ReplyToMessageID = messageID
				}
				b.api.Send(reply)
				continue
			}
			if !isMember {
				log.Printf("User %d (%s) is not a member of %s. Requesting join.", userID, userName, channelToJoin)
				// If it's a callback, we might not want to reply to the original link message,
				// but rather send a new message or edit the inline message.
				// For simplicity now, just send a new message.
				// The messageID passed to sendJoinChannelMessage should be the original user message if available
				b.sendJoinChannelMessage(chatID, channelToJoin, messageID)
				if isCallback { // Answer callback even if denying access
					b.api.Send(tgbotapi.NewCallback(update.CallbackQuery.ID, "لطفا ابتدا در کانال عضو شوید."))
				}
				continue
			}
		}

		if len(b.cfg.AllowedUserIDs) > 0 {
			isAllowed := false
			for _, allowedID := range b.cfg.AllowedUserIDs {
				if userID == allowedID {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				log.Printf("User %s (%d) is not in AllowedUserIDs list. Ignoring.", userName, userID)
				reply := tgbotapi.NewMessage(chatID, "متاسفم، شما اجازه استفاده از این ربات را ندارید.")
				if messageID != 0 && !isCallback {
					reply.ReplyToMessageID = messageID
				}
				b.api.Send(reply)
				if isCallback {
					b.api.Send(tgbotapi.NewCallback(update.CallbackQuery.ID, "شما مجاز نیستید."))
				}
				continue
			}
		}

		if isCallback {
			b.handleCallbackQuery(update.CallbackQuery, userName, userID)
		} else if update.Message.IsCommand() {
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

	if sentPInfoMsg.MessageID != 0 {
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
		log.Printf("[%s] Content type unclear for %s (IsVideo: %t, IsAudioOnly: %t). Defaulting to audio download directly.", userIdentifier, urlToDownload, trackInfo.IsVideo, trackInfo.IsAudioOnly)
		b.processDownloadRequest(message.Chat.ID, message.MessageID, urlToDownload, downloader.AudioOnly, trackInfo, userName, userID)
		return
	}

	choiceMsgText := fmt.Sprintf("یافت شد: %s - %s\nچه نوع دانلودی می‌خواهید؟", trackInfo.Artist, trackInfo.Title)
	if trackInfo.Title == "Unknown Title" && trackInfo.Artist == "Unknown Artist" {
		choiceMsgText = "چه نوع دانلودی می‌خواهید؟"
	}
	choiceMsg := tgbotapi.NewMessage(chatID, choiceMsgText)
	choiceMsg.ReplyToMessageID = message.MessageID
	choiceMsg.ReplyMarkup = keyboard
	if _, err := b.api.Send(choiceMsg); err != nil {
		log.Printf("[%s] Error sending download type choice message: %v", userIdentifier, err)
	}
}

func (b *Bot) handleCallbackQuery(callback *tgbotapi.CallbackQuery, userName string, userID int64) {
	b.api.Send(tgbotapi.NewCallback(callback.ID, ""))

	chatID := callback.Message.Chat.ID
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)

	originalLinkMessageID := 0
	var originalLinkURL string

	if callback.Message.ReplyToMessage != nil {
		originalLinkMessageID = callback.Message.ReplyToMessage.MessageID
		originalLinkURL = callback.Message.ReplyToMessage.Text
	} else {
		log.Printf("[%s] Callback query message does not have ReplyToMessage. Cannot determine original link.", userIdentifier)
		b.api.Send(tgbotapi.NewMessage(chatID, "خطای داخلی: اطلاعات لینک اصلی برای پردازش درخواست شما یافت نشد."))
		return
	}

	if originalLinkURL == "" {
		log.Printf("[%s] Original link URL is empty from ReplyToMessage.", userIdentifier)
		b.api.Send(tgbotapi.NewMessage(chatID, "خطای داخلی: لینک اصلی برای دانلود خالی است."))
		return
	}

	deleteChoiceMsg := tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID)
	if _, err := b.api.Send(deleteChoiceMsg); err != nil {
		log.Printf("[%s] Failed to delete choice message %d: %v", userIdentifier, callback.Message.MessageID, err)
	}

	parts := strings.Split(callback.Data, ":")
	if len(parts) < 2 || parts[0] != "dltype" {
		log.Printf("[%s] Invalid callback data format: %s\n", userIdentifier, callback.Data)
		b.api.Send(tgbotapi.NewMessage(chatID, "خطای داخلی: درخواست نامعتبر."))
		return
	}
	chosenTypeStr := parts[1]

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

	log.Printf("[%s] User chose %s for URL: %s (Original MsgID: %d)\n", userIdentifier, chosenTypeStr, originalLinkURL, originalLinkMessageID)

	trackInfo, err := b.downloader.GetTrackInfo(originalLinkURL, userIdentifier)
	if err != nil {
		log.Printf("[%s] Error re-fetching track info for URL %s: %v\n", userIdentifier, originalLinkURL, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دریافت مجدد اطلاعات از لینک مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		if originalLinkMessageID != 0 {
			errMsg.ReplyToMessageID = originalLinkMessageID
		}
		b.api.Send(errMsg)
		return
	}

	b.processDownloadRequest(chatID, originalLinkMessageID, originalLinkURL, dlType, trackInfo, userName, userID)
}

func (b *Bot) processDownloadRequest(chatID int64, originalLinkMessageID int, urlToDownload string, dlType downloader.DownloadType, trackInfo *downloader.TrackInfo, userName string, userID int64) {
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)

	downloadingMsgText := fmt.Sprintf("در حال دانلود %s - %s به صورت %s... ⏳", trackInfo.Artist, trackInfo.Title, typeToString(dlType))
	if trackInfo.Title == "Unknown Title" && trackInfo.Artist == "Unknown Artist" {
		downloadingMsgText = fmt.Sprintf("در حال دانلود لینک شما به صورت %s... ⏳", typeToString(dlType))
	}

	dlNoticeMsg := tgbotapi.NewMessage(chatID, downloadingMsgText)
	if originalLinkMessageID != 0 {
		dlNoticeMsg.ReplyToMessageID = originalLinkMessageID
	}
	sentDlNoticeMsg, err := b.api.Send(dlNoticeMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'downloading media' message: %v", userIdentifier, err)
	}

	downloadedFilePath, actualExt, err := b.downloader.DownloadMedia(urlToDownload, userIdentifier, dlType, trackInfo)
	if err != nil {
		log.Printf("[%s] Error downloading media for URL %s: %v\n", userIdentifier, urlToDownload, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دانلود مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		if originalLinkMessageID != 0 {
			errMsg.ReplyToMessageID = originalLinkMessageID
		}
		b.api.Send(errMsg)
		if sentDlNoticeMsg.MessageID != 0 {
			b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentDlNoticeMsg.MessageID))
		}
		return
	}

	if sentDlNoticeMsg.MessageID != 0 {
		b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentDlNoticeMsg.MessageID))
	}

	log.Printf("[%s] Media downloaded: %s (ext: %s). Sending to user.\n", userIdentifier, downloadedFilePath, actualExt)

	if trackInfo.ThumbnailURL != "" && (dlType == downloader.AudioOnly || dlType == downloader.VideoBest) {
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
		if originalLinkMessageID != 0 {
			audioFile.ReplyToMessageID = originalLinkMessageID
		}
		audioFile.Title = trackInfo.Title
		audioFile.Performer = trackInfo.Artist
		audioFile.Caption = caption
		sentMediaMessage, sendErr = b.api.Send(audioFile)
	} else if dlType == downloader.VideoBest || actualExt == "mp4" || actualExt == "mkv" || actualExt == "webm" {
		videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			videoFile.ReplyToMessageID = originalLinkMessageID
		}
		videoFile.Caption = caption
		sentMediaMessage, sendErr = b.api.Send(videoFile)
	} else {
		log.Printf("[%s] Unknown/unhandled extension '%s', sending as document.\n", userIdentifier, actualExt)
		docFile := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			docFile.ReplyToMessageID = originalLinkMessageID
		}
		docFile.Caption = caption
		sentMediaMessage, sendErr = b.api.Send(docFile)
	}

	if sendErr != nil {
		log.Printf("[%s] Error sending media file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
		errorMsgText := fmt.Sprintf("فایل دانلود شد اما در ارسال آن مشکلی پیش آمد.\nخطا: %s", sendErr.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		b.api.Send(errMsg)
	} else {
		log.Printf("[%s] Media file %s sent successfully. MessageID: %d\n", userIdentifier, downloadedFilePath, sentMediaMessage.MessageID)
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
