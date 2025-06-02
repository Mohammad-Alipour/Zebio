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
	escapedBotName := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, b.api.Self.FirstName)
	escapedChannelLink := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, channelLink)

	replyText := fmt.Sprintf("⚠️ کاربر گرامی، برای استفاده از امکانات ربات *%s*، ابتدا باید در کانال رسمی ما عضو شوید:\n\n%s\n\nپس از عضویت، دوباره دستور خود را ارسال کنید یا /start را بزنید.", escapedBotName, escapedChannelLink)

	reply := tgbotapi.NewMessage(chatID, replyText)
	reply.ParseMode = tgbotapi.ModeMarkdownV2
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
		var messageID int = 0
		var isCallback bool = false
		var fromFirstName string = ""

		if update.Message != nil {
			message := update.Message
			if message.From == nil {
				continue
			}
			userID = message.From.ID
			userName = message.From.UserName
			fromFirstName = message.From.FirstName
			if userName == "" {
				userName = fromFirstName
			}
			chatID = message.Chat.ID
			messageID = message.MessageID
			log.Printf("[%s (%d)] Received message: %s\n", userName, userID, message.Text)
		} else if update.CallbackQuery != nil {
			isCallback = true
			callback := update.CallbackQuery
			if callback.From == nil {
				continue
			}
			userID = callback.From.ID
			userName = callback.From.UserName
			fromFirstName = callback.From.FirstName
			if userName == "" {
				userName = fromFirstName
			}
			chatID = callback.Message.Chat.ID
			if callback.Message.ReplyToMessage != nil {
				messageID = callback.Message.ReplyToMessage.MessageID
			} else {
				messageID = callback.Message.MessageID
			}
			log.Printf("[%s (%d)] Received callback query data: %s from message %d\n", userName, userID, callback.Data, callback.Message.MessageID)
		} else {
			continue
		}

		if b.cfg.ForceJoinChannel != "" {
			isMember, channelToJoin, err := b.isUserMemberOfRequiredChannel(userID)
			if err != nil {
				log.Printf("Error during channel membership check for user %d: %v. Sending error message.", userID, err)
				reply := tgbotapi.NewMessage(chatID, "خطا در بررسی عضویت کانال\\. لطفاً لحظاتی دیگر دوباره امتحان کنید\\.")
				reply.ParseMode = tgbotapi.ModeMarkdownV2
				if messageID != 0 && !isCallback {
					reply.ReplyToMessageID = messageID
				}
				b.api.Send(reply)
				continue
			}
			if !isMember {
				log.Printf("User %d (%s) is not a member of %s. Requesting join.", userID, userName, channelToJoin)
				replyToID := messageID
				if isCallback {
					if update.CallbackQuery.Message.ReplyToMessage != nil {
						replyToID = update.CallbackQuery.Message.ReplyToMessage.MessageID
					} else {
						replyToID = 0
					}
				}
				b.sendJoinChannelMessage(chatID, channelToJoin, replyToID)
				if isCallback {
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
				reply.ParseMode = tgbotapi.ModeMarkdownV2
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
			b.handleCallbackQuery(update.CallbackQuery, userName, userID, fromFirstName)
		} else if update.Message.IsCommand() {
			b.handleCommand(update.Message, fromFirstName)
		} else if update.Message.Text != "" {
			b.handleLink(update.Message, userName, userID, fromFirstName)
		} else {
			log.Printf("[%s (%d)] Received non-text, non-command message. Ignoring.", userName, userID)
		}
	}
}

func (b *Bot) handleCommand(message *tgbotapi.Message, fromFirstName string) {
	userName := message.From.UserName
	if userName == "" {
		userName = fromFirstName
	}
	command := message.Command()
	log.Printf("[%s (%d)] Received command: /%s\n", userName, message.From.ID, command)

	var msgText string
	escapedFirstName := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, fromFirstName)
	escapedBotName := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, b.api.Self.FirstName)

	switch command {
	case "start":
		msgText = fmt.Sprintf("سلام *%s* عزیز! 👋\n\nبه ربات دانلودر *%s* خوش اومدی.\nمن می‌تونم از لینک‌هایی که می‌فرستی (مثل یوتیوب، ساندکلود، اینستاگرام و...) برات فایل صوتی یا ویدیویی دانلود کنم.\n\n🔗 کافیه لینک مورد نظرت رو برام ارسال کنی!\n\nراهنمایی بیشتر: /help", escapedFirstName, escapedBotName)
	case "help":
		msgText = fmt.Sprintf("راهنمای استفاده از ربات *%s* 🤖\n\n۱. لینک مستقیم از پلتفرم‌هایی مثل:\n   یوتیوب 🔴\n   ساندکلود 🟠\n   اینستاگرام 🟣\n   و ... رو برای من ارسال کن.\n\n۲. اگر محتوای لینک هم صوتی و هم تصویری باشه، ازت می‌پرسم که کدوم رو می‌خوای برات دانلود کنم:\n   🎵 *صدا* (فایل MP3 با کاور)\n   🎬 *ویدیو* (فایل MP4)\n\n۳. بعد از انتخاب، فایل رو برات آماده و ارسال می‌کنم!", escapedBotName)
	default:
		msgText = "دستور شناخته نشد. برای راهنمایی /help رو بزنید."
	}
	reply := tgbotapi.NewMessage(message.Chat.ID, msgText)
	reply.ParseMode = tgbotapi.ModeMarkdownV2
	reply.ReplyToMessageID = message.MessageID
	if _, err := b.api.Send(reply); err != nil {
		log.Printf("[%s (%d)] Error sending command reply: %v", userName, message.From.ID, err)
	}
}

func (b *Bot) handleLink(message *tgbotapi.Message, userName string, userID int64, fromFirstName string) {
	chatID := message.Chat.ID
	urlToDownload := message.Text
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)

	log.Printf("[%s] Received link to process: %s\n", userIdentifier, urlToDownload)

	processingMsgText := "🔍 در حال بررسی و دریافت اطلاعات از لینک شما\\.\\.\\. لطفاً چند لحظه صبر کنید\\."
	processingMsg := tgbotapi.NewMessage(chatID, processingMsgText)
	processingMsg.ParseMode = tgbotapi.ModeMarkdownV2
	processingMsg.ReplyToMessageID = message.MessageID
	sentPInfoMsg, err := b.api.Send(processingMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'fetching link info' message: %v", userIdentifier, err)
	}

	trackInfo, err := b.downloader.GetTrackInfo(urlToDownload, userIdentifier)
	if err != nil {
		log.Printf("[%s] Error fetching track info for URL %s: %v\n", userIdentifier, urlToDownload, err)
		escapedError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, err.Error())
		errorMsgText := fmt.Sprintf("⚠️ متاسفانه در پردازش اولیه لینک شما مشکلی پیش آمد.\n\nعلت خطا:\n`%s`\n\nلطفاً از صحت لینک مطمئن شوید یا لینک دیگری را امتحان کنید. اگر مشکل ادامه داشت، بعداً دوباره تلاش کنید.", escapedError)
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		errMsg.ReplyToMessageID = message.MessageID
		b.api.Send(errMsg)
		if sentPInfoMsg.MessageID != 0 {
			_, delErr := b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentPInfoMsg.MessageID))
			if delErr != nil {
				log.Printf("[%s] Failed to delete 'PInfoMsg' after error: %v", userIdentifier, delErr)
			}
		}
		return
	}

	if sentPInfoMsg.MessageID != 0 {
		_, delErr := b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentPInfoMsg.MessageID))
		if delErr != nil {
			log.Printf("[%s] Failed to delete 'PInfoMsg': %v", userIdentifier, delErr)
		}
	}

	var keyboard tgbotapi.InlineKeyboardMarkup
	escapedArtist := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Artist)
	escapedTitle := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Title)

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
		log.Printf("[%s] Content type unclear for %s. Defaulting to audio download directly.", userIdentifier, urlToDownload)
		b.processDownloadRequest(message.Chat.ID, message.MessageID, urlToDownload, downloader.AudioOnly, trackInfo, userName, userID, fromFirstName)
		return
	}

	choiceMsgText := ""
	if trackInfo.Title != "Unknown Title" && trackInfo.Artist != "Unknown Artist" {
		choiceMsgText = fmt.Sprintf("✅ اطلاعات با موفقیت دریافت شد:\n*خواننده:* `%s`\n*عنوان:* `%s`\n\nحالا انتخاب کنید که کدام مورد را برای شما آماده کنم؟ 👇", escapedArtist, escapedTitle)
	} else {
		choiceMsgText = "✅ اطلاعات اولیه لینک دریافت شد.\n\nلطفاً نوع دانلود مورد نظرتون رو انتخاب کنید: 👇"
	}
	choiceMsg := tgbotapi.NewMessage(chatID, choiceMsgText)
	choiceMsg.ParseMode = tgbotapi.ModeMarkdownV2
	choiceMsg.ReplyToMessageID = message.MessageID
	choiceMsg.ReplyMarkup = keyboard
	if _, err := b.api.Send(choiceMsg); err != nil {
		log.Printf("[%s] Error sending download type choice message: %v", userIdentifier, err)
	}
}

func (b *Bot) handleCallbackQuery(callback *tgbotapi.CallbackQuery, userName string, userID int64, fromFirstName string) {
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
		errMsgText := "🚫 یک خطای داخلی در یافتن لینک اصلی شما رخ داد (کد خطا: CB\\_NO\\_LINK)\\. لطفاً دوباره لینک را ارسال کرده و سپس انتخاب کنید."
		errMsg := tgbotapi.NewMessage(chatID, errMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		b.api.Send(errMsg)
		return
	}

	if originalLinkURL == "" {
		log.Printf("[%s] Original link URL is empty from ReplyToMessage.", userIdentifier)
		errMsgText := "🚫 یک خطای داخلی در یافتن لینک اصلی شما رخ داد (کد خطا: CB\\_EMPTY\\_LINK)\\. لطفاً دوباره لینک را ارسال کرده و سپس انتخاب کنید."
		errMsg := tgbotapi.NewMessage(chatID, errMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		b.api.Send(errMsg)
		return
	}

	deleteChoiceMsg := tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID)
	if _, err := b.api.Send(deleteChoiceMsg); err != nil {
		log.Printf("[%s] Failed to delete choice message %d: %v", userIdentifier, callback.Message.MessageID, err)
	}

	parts := strings.Split(callback.Data, ":")
	// Expecting dltype:type (original_link_msg_id is implicit via ReplyToMessage)
	if len(parts) < 2 || parts[0] != "dltype" {
		log.Printf("[%s] Invalid callback data format: %s\n", userIdentifier, callback.Data)
		errMsgText := "🚫 یک خطای داخلی در پردازش درخواست شما رخ داد (کد خطا: CB\\_INV\\_FORMAT)\\. لطفاً دوباره تلاش کنید."
		errMsg := tgbotapi.NewMessage(chatID, errMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		b.api.Send(errMsg)
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
		errMsgText := "🚫 نوع دانلود درخواستی شما نامعتبر است (کد خطا: CB\\_INV\\_TYPE)\\. لطفاً دوباره تلاش کنید."
		errMsg := tgbotapi.NewMessage(chatID, errMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		b.api.Send(errMsg)
		return
	}

	log.Printf("[%s] User chose %s for URL: %s (Original MsgID: %d)\n", userIdentifier, chosenTypeStr, originalLinkURL, originalLinkMessageID)

	trackInfo, err := b.downloader.GetTrackInfo(originalLinkURL, userIdentifier)
	if err != nil {
		log.Printf("[%s] Error re-fetching track info for URL %s: %v\n", userIdentifier, originalLinkURL, err)
		escapedError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, err.Error())
		errorMsgText := fmt.Sprintf("⚠️ متاسفانه در پردازش اولیه لینک شما مشکلی پیش آمد.\n\nعلت خطا:\n`%s`\n\nلطفاً از صحت لینک مطمئن شوید یا لینک دیگری را امتحان کنید. اگر مشکل ادامه داشت، بعداً دوباره تلاش کنید.", escapedError)
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		if originalLinkMessageID != 0 {
			errMsg.ReplyToMessageID = originalLinkMessageID
		}
		b.api.Send(errMsg)
		return
	}

	b.processDownloadRequest(chatID, originalLinkMessageID, originalLinkURL, dlType, trackInfo, userName, userID, fromFirstName)
}

func (b *Bot) processDownloadRequest(chatID int64, originalLinkMessageID int, urlToDownload string, dlType downloader.DownloadType, trackInfo *downloader.TrackInfo, userName string, userID int64, fromFirstName string) {
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)
	escapedArtist := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Artist)
	escapedTitle := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Title)
	escapedFileType := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, typeToString(dlType))

	downloadingMsgText := ""
	if trackInfo.Title != "Unknown Title" && trackInfo.Artist != "Unknown Artist" {
		downloadingMsgText = fmt.Sprintf("در حال آماده‌سازی و دانلود *%s* برای:\n`%s \\- %s`\n\nاین فرآیند ممکن است کمی طول بکشد، لطفاً صبور باشید... ⏳", escapedFileType, escapedArtist, escapedTitle)
	} else {
		downloadingMsgText = fmt.Sprintf("در حال آماده‌سازی و دانلود *%s* شما... ⏳", escapedFileType)
	}

	dlNoticeMsg := tgbotapi.NewMessage(chatID, downloadingMsgText)
	dlNoticeMsg.ParseMode = tgbotapi.ModeMarkdownV2
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
		escapedError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, err.Error())
		errorMsgText := fmt.Sprintf("❌ متاسفانه در فرآیند دانلود مشکلی پیش آمد.\n\nجزئیات خطا:\n`%s`\n\nلطفاً لینک دیگری را امتحان کنید یا اگر فکر می‌کنید لینک سالم است، چند لحظه دیگر دوباره تلاش کنید.", escapedError)
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		if originalLinkMessageID != 0 {
			errMsg.ReplyToMessageID = originalLinkMessageID
		}
		b.api.Send(errMsg)
		if sentDlNoticeMsg.MessageID != 0 {
			_, delErr := b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentDlNoticeMsg.MessageID))
			if delErr != nil {
				log.Printf("[%s] Failed to delete 'DlNoticeMsg' after error: %v", userIdentifier, delErr)
			}
		}
		return
	}

	if sentDlNoticeMsg.MessageID != 0 {
		_, delErr := b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentDlNoticeMsg.MessageID))
		if delErr != nil {
			log.Printf("[%s] Failed to delete 'DlNoticeMsg': %v", userIdentifier, delErr)
		}
	}

	log.Printf("[%s] Media downloaded: %s (ext: %s). Sending to user.\n", userIdentifier, downloadedFilePath, actualExt)

	if trackInfo.ThumbnailURL != "" && (dlType == downloader.AudioOnly || dlType == downloader.VideoBest) {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(trackInfo.ThumbnailURL))
		// photoMsg.Caption = fmt.Sprintf("*«%s»*\n_%s_", escapedTitle, escapedArtist) // کپشن عکس کاور (اختیاری) - فعلا بدون کپشن
		// photoMsg.ParseMode = tgbotapi.ModeMarkdownV2
		if _, err := b.api.Send(photoMsg); err != nil {
			log.Printf("[%s] Error sending cover photo for %s: %v", userIdentifier, trackInfo.Title, err)
		} else {
			log.Printf("[%s] Cover art for %s sent successfully.\n", userIdentifier, trackInfo.Title)
		}
	}

	var caption string
	escapedBotUsernameMention := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "@"+b.api.Self.UserName)

	if dlType == downloader.AudioOnly || actualExt == "mp3" {
		caption = fmt.Sprintf("🎵 *%s*\n👤 _%s_\n\n%s", escapedTitle, escapedArtist, escapedBotUsernameMention)
		audioFile := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			audioFile.ReplyToMessageID = originalLinkMessageID
		}
		audioFile.Title = trackInfo.Title
		audioFile.Performer = trackInfo.Artist
		audioFile.Caption = caption
		audioFile.ParseMode = tgbotapi.ModeMarkdownV2
		sentMediaMessage, sendErr := b.api.Send(audioFile)
		if sendErr != nil {
			log.Printf("[%s] Error sending audio file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
			escapedSendError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, sendErr.Error())
			errorMsgText := fmt.Sprintf("⚠️ فایل شما با موفقیت دانلود شد، اما متاسفانه در مرحله ارسال به تلگرام مشکلی رخ داد.\n\nجزئیات خطا:\n`%s`", escapedSendError)
			errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
			errMsg.ParseMode = tgbotapi.ModeMarkdownV2
			b.api.Send(errMsg)
		} else {
			log.Printf("[%s] Audio file %s sent successfully. MessageID: %d\n", userIdentifier, downloadedFilePath, sentMediaMessage.MessageID)
		}
	} else if dlType == downloader.VideoBest || actualExt == "mp4" || actualExt == "mkv" || actualExt == "webm" {
		caption = fmt.Sprintf("🎬 *%s*\n👤 _%s_\n\n%s", escapedTitle, escapedArtist, escapedBotUsernameMention)
		videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			videoFile.ReplyToMessageID = originalLinkMessageID
		}
		videoFile.Caption = caption
		videoFile.ParseMode = tgbotapi.ModeMarkdownV2
		sentMediaMessage, sendErr := b.api.Send(videoFile)
		if sendErr != nil {
			log.Printf("[%s] Error sending video file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
			escapedSendError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, sendErr.Error())
			errorMsgText := fmt.Sprintf("⚠️ فایل شما با موفقیت دانلود شد، اما متاسفانه در مرحله ارسال به تلگرام مشکلی رخ داد.\n\nجزئیات خطا:\n`%s`", escapedSendError)
			errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
			errMsg.ParseMode = tgbotapi.ModeMarkdownV2
			b.api.Send(errMsg)
		} else {
			log.Printf("[%s] Video file %s sent successfully. MessageID: %d\n", userIdentifier, downloadedFilePath, sentMediaMessage.MessageID)
		}
	} else {
		log.Printf("[%s] Unknown/unhandled extension '%s', sending as document.\n", userIdentifier, actualExt)
		caption = fmt.Sprintf("📄 *%s*\n👤 _%s_\n\n%s", escapedTitle, escapedArtist, escapedBotUsernameMention)
		docFile := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			docFile.ReplyToMessageID = originalLinkMessageID
		}
		docFile.Caption = caption
		docFile.ParseMode = tgbotapi.ModeMarkdownV2
		sentMediaMessage, sendErr := b.api.Send(docFile) // sentMediaMessage was not declared here, fixed.
		if sendErr != nil {
			log.Printf("[%s] Error sending document file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
			escapedSendError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, sendErr.Error())
			errorMsgText := fmt.Sprintf("⚠️ فایل شما با موفقیت دانلود شد، اما متاسفانه در مرحله ارسال به تلگرام مشکلی رخ داد.\n\nجزئیات خطا:\n`%s`", escapedSendError)
			errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
			errMsg.ParseMode = tgbotapi.ModeMarkdownV2
			b.api.Send(errMsg)
		} else {
			log.Printf("[%s] Document file %s sent successfully. MessageID: %d\n", userIdentifier, downloadedFilePath, sentMediaMessage.MessageID)
		}
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
		return "فایل صوتی"
	}
	if dlType == downloader.VideoBest {
		return "فایل ویدیویی"
	}
	return "فایل"
}
