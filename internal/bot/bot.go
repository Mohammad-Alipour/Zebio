package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/Mohammad-Alipour/Zebio/internal/config"
	"github.com/Mohammad-Alipour/Zebio/internal/downloader"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/sync/semaphore"
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
	replyText := fmt.Sprintf("⚠️ کاربر گرامی، برای استفاده از امکانات ربات *%s*، ابتدا باید در کانال رسمی ما عضو شوید:\n\n%s\n\nپس از عضویت، دوباره دستور خود را ارسال کنید یا /start را بزنید\\.", escapedBotName, escapedChannelLink)
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
			if callback.Message != nil {
				chatID = callback.Message.Chat.ID
				if callback.Message.ReplyToMessage != nil {
					messageID = callback.Message.ReplyToMessage.MessageID
				} else {
					messageID = callback.Message.MessageID
				}
			}
			log.Printf("[%s (%d)] Received callback query data: %s from message %d\n", userName, userID, callback.Data, messageID)
		} else {
			continue
		}

		if b.cfg.ForceJoinChannel != "" {
			isMember, channelToJoin, err := b.isUserMemberOfRequiredChannel(userID)
			if err != nil {
				log.Printf("Error during channel membership check for user %d: %v. Sending error message.", userID, err)
				errMsgText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "خطا در بررسی عضویت کانال\\. لطفاً لحظاتی دیگر دوباره امتحان کنید\\.")
				reply := tgbotapi.NewMessage(chatID, errMsgText)
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
					b.api.Send(tgbotapi.NewCallback(update.CallbackQuery.ID, "لطفا ابتدا در کانال عضو شوید\\."))
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
				errMsgText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "متاسفم، شما اجازه استفاده از این ربات را ندارید\\.")
				reply := tgbotapi.NewMessage(chatID, errMsgText)
				reply.ParseMode = tgbotapi.ModeMarkdownV2
				if messageID != 0 && !isCallback {
					reply.ReplyToMessageID = messageID
				}
				b.api.Send(reply)
				if isCallback {
					b.api.Send(tgbotapi.NewCallback(update.CallbackQuery.ID, "شما مجاز نیستید\\."))
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
	escapedBotDisplayName := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, b.api.Self.FirstName)

	switch command {
	case "start":
		msgText = fmt.Sprintf("سلام *%s* عزیز\\! 👋\n\nبه ربات دانلودر *%s* خوش اومدی\\.\nمن می‌تونم از لینک‌هایی که می‌فرستی \\(مثل یوتیوب، ساندکلود، اینستاگرام و\\.\\.\\.\\) برات فایل صوتی یا ویدیویی دانلود کنم\\.\n\n🔗 کافیه لینک مورد نظرت رو برام ارسال کنی\\!\n\nراهنمایی بیشتر: /help", escapedFirstName, escapedBotDisplayName)
	case "help":
		msgText = fmt.Sprintf("راهنمای استفاده از ربات *%s* 🤖\n\n۱\\. لینک مستقیم از پلتفرم‌هایی مثل:\n   یوتیوب 🔴\n   ساندکلود 🟠\n   اینستاگرام 🟣\n   و \\.\\.\\. رو برای من ارسال کن\\.\n\n۲\\. اگر محتوای لینک هم صوتی و هم تصویری باشه، ازت می‌پرسم که کدوم رو می‌خوای برات دانلود کنم:\n   🎵 *صدا* \\(فایل MP3 با کاور\\)\n   🎬 *ویدیو* \\(فایل MP4\\)\n\n۳\\. بعد از انتخاب، فایل رو برات آماده و ارسال می‌کنم\\!", escapedBotDisplayName)
	default:
		msgText = tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "دستور شناخته نشد\\. برای راهنمایی /help رو بزنید\\.")
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

	log.Printf("[%s] Received link to process: %s", userIdentifier, urlToDownload)

	processingMsg := tgbotapi.NewMessage(chatID, tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "🔍 در حال بررسی و دریافت اطلاعات از لینک شما... لطفاً چند لحظه صبر کنید."))
	processingMsg.ReplyToMessageID = message.MessageID
	sentPInfoMsg, err := b.api.Send(processingMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'fetching link info' message: %v", userIdentifier, err)
	}

	linkInfo, err := b.downloader.GetLinkInfo(urlToDownload, userIdentifier)
	if sentPInfoMsg.MessageID != 0 {
		b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentPInfoMsg.MessageID))
	}

	if err != nil {
		log.Printf("[%s] Error fetching link info for URL %s: %v", userIdentifier, urlToDownload, err)
		escapedError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, err.Error())
		errorMsgText := fmt.Sprintf("⚠️ متاسفانه در پردازش اولیه لینک شما مشکلی پیش آمد\\.\n\nعلت خطا:\n`%s`\n\nلطفاً از صحت لینک مطمئن شوید یا لینک دیگری را امتحان کنید\\. اگر مشکل ادامه داشت، بعداً دوباره تلاش کنید\\.", escapedError)
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		errMsg.ReplyToMessageID = message.MessageID
		b.api.Send(errMsg)
		return
	}

	if linkInfo.Type == "album" && len(linkInfo.Tracks) > 0 {
		escapedTitle := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, linkInfo.Title)
		escapedUploader := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, linkInfo.Uploader)
		albumMsgText := fmt.Sprintf("آلبوم یا پلی‌لیست پیدا شد:\n*%s*\nتوسط: `%s`\nتعداد آهنگ‌ها: *%d*\n\nآیا می‌خواهید تمام آهنگ‌ها دانلود شوند؟", escapedTitle, escapedUploader, len(linkInfo.Tracks))
		yesButton := tgbotapi.NewInlineKeyboardButtonData("✅ بله، دانلود کن", "dlalbum:yes")
		noButton := tgbotapi.NewInlineKeyboardButtonData("❌ نه", "dlalbum:no")
		keyboard := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(yesButton, noButton))
		albumMsg := tgbotapi.NewMessage(chatID, albumMsgText)
		albumMsg.ParseMode = tgbotapi.ModeMarkdownV2
		albumMsg.ReplyToMessageID = message.MessageID
		albumMsg.ReplyMarkup = keyboard
		b.api.Send(albumMsg)
		return
	}

	if linkInfo.Type == "track" && len(linkInfo.Tracks) == 1 {
		trackInfo := linkInfo.Tracks[0]
		var buttons []tgbotapi.InlineKeyboardButton

		if trackInfo.HasVideo {
			videoButton := tgbotapi.NewInlineKeyboardButtonData("دانلود ویدیو 🎬", fmt.Sprintf("dltype:video:%d", message.MessageID))
			audioButton := tgbotapi.NewInlineKeyboardButtonData("دانلود صدا 🎵", fmt.Sprintf("dltype:audio:%d", message.MessageID))
			buttons = append(buttons, videoButton, audioButton)
		}
		if trackInfo.HasImage {
			photoButton := tgbotapi.NewInlineKeyboardButtonData("دانلود عکس 🖼️", fmt.Sprintf("dltype:photo:%d", message.MessageID))
			buttons = append(buttons, photoButton)
		}
		if trackInfo.IsAudioOnly {
			audioButton := tgbotapi.NewInlineKeyboardButtonData("دانلود صدا 🎵", fmt.Sprintf("dltype:audio:%d", message.MessageID))
			buttons = append(buttons, audioButton)
		}

		if len(buttons) == 0 {
			log.Printf("[%s] No downloadable content type found for URL %s. Informing user.", userIdentifier, urlToDownload)
			errMsgText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "محتوای قابل دانلودی \\(ویدیو، صدا یا عکس\\) در این لینک پیدا نشد\\.")
			errMsg := tgbotapi.NewMessage(chatID, errMsgText)
			errMsg.ParseMode = tgbotapi.ModeMarkdownV2
			errMsg.ReplyToMessageID = message.MessageID
			b.api.Send(errMsg)
			return
		}

		keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons)
		escapedArtist := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Artist)
		escapedTitle := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Title)

		choiceMsgText := ""
		if trackInfo.Title != "Unknown Title" && trackInfo.Artist != "Unknown Artist" {
			choiceMsgText = fmt.Sprintf("✅ اطلاعات با موفقیت دریافت شد:\n*پیج/خواننده:* `%s`\n*عنوان:* `%s`\n\nحالا انتخاب کنید که کدام مورد را برای شما آماده کنم؟ 👇", escapedArtist, escapedTitle)
		} else {
			choiceMsgText = "✅ اطلاعات اولیه لینک دریافت شد\\.\n\nلطفاً نوع دانلود مورد نظرتون رو انتخاب کنید: 👇"
		}

		choiceMsg := tgbotapi.NewMessage(chatID, choiceMsgText)
		choiceMsg.ParseMode = tgbotapi.ModeMarkdownV2
		choiceMsg.ReplyToMessageID = message.MessageID
		choiceMsg.ReplyMarkup = keyboard
		if _, err := b.api.Send(choiceMsg); err != nil {
			log.Printf("[%s] Error sending download type choice message: %v", userIdentifier, err)
		}
		return
	}

	log.Printf("[%s] Link type was not 'album' or 'track', or track list was empty. URL: %s", userIdentifier, urlToDownload)
	errMsgText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "نوع لینک ارسال شده پشتیبانی نمی‌شود یا محتوایی در آن یافت نشد\\.")
	errMsg := tgbotapi.NewMessage(chatID, errMsgText)
	errMsg.ParseMode = tgbotapi.ModeMarkdownV2
	errMsg.ReplyToMessageID = message.MessageID
	b.api.Send(errMsg)
}

func (b *Bot) handleCallbackQuery(callback *tgbotapi.CallbackQuery, userName string, userID int64, fromFirstName string) {
	b.api.Send(tgbotapi.NewCallback(callback.ID, ""))

	chatID := callback.Message.Chat.ID
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)
	parts := strings.Split(callback.Data, ":")

	if len(parts) > 0 && parts[0] == "dlalbum" {
		action := parts[1]
		if action == "no" {
			log.Printf("[%s] User cancelled album download.", userIdentifier)
			b.api.Send(tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID))
			return
		}

		if action == "yes" {
			log.Printf("[%s] User confirmed album download.", userIdentifier)

			var originalLinkURL string
			if callback.Message != nil && callback.Message.ReplyToMessage != nil {
				originalLinkURL = callback.Message.ReplyToMessage.Text
			}

			if originalLinkURL == "" {
				log.Printf("[%s] Could not find original album link from callback reply.", userIdentifier)
				b.api.Send(tgbotapi.NewEditMessageText(chatID, callback.Message.MessageID, "خطا: لینک اصلی آلبوم پیدا نشد."))
				return
			}

			editMsgText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "✅ بسیار خب! فرآیند دانلود آلبوم آغاز شد. این فرآیند ممکن است زمان‌بر باشد...")
			editMsg := tgbotapi.NewEditMessageText(chatID, callback.Message.MessageID, editMsgText)
			editMsg.ParseMode = tgbotapi.ModeMarkdownV2
			editMsg.ReplyMarkup = nil
			b.api.Send(editMsg)

			go b.processAlbumDownload(chatID, originalLinkURL, userIdentifier, userName, userID, fromFirstName, callback.Message.MessageID)
		}
		return
	}

	if len(parts) < 3 || parts[0] != "dltype" {
		log.Printf("[%s] Invalid callback data format for single download: %s", userIdentifier, callback.Data)
		return
	}

	originalLinkMessageID := 0
	var originalLinkURL string

	if callback.Message.ReplyToMessage != nil {
		originalLinkMessageID = callback.Message.ReplyToMessage.MessageID
		originalLinkURL = callback.Message.ReplyToMessage.Text
	} else {
		log.Printf("[%s] Callback query message does not have ReplyToMessage. Cannot determine original link.", userIdentifier)
		return
	}

	if originalLinkURL == "" {
		log.Printf("[%s] Original link URL is empty from ReplyToMessage.", userIdentifier)
		return
	}

	b.api.Send(tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID))

	chosenTypeStr := parts[1]
	var dlType downloader.DownloadType
	switch chosenTypeStr {
	case "audio":
		dlType = downloader.AudioOnly
	case "video":
		dlType = downloader.VideoBest
	case "photo":
		dlType = downloader.ImageBest
	default:
		log.Printf("[%s] Unknown download type in callback: %s", userIdentifier, chosenTypeStr)
		return
	}

	log.Printf("[%s] User chose %s for URL: %s (Original MsgID: %d)", userIdentifier, chosenTypeStr, originalLinkURL, originalLinkMessageID)

	linkInfo, err := b.downloader.GetLinkInfo(originalLinkURL, userIdentifier)
	if err != nil || len(linkInfo.Tracks) == 0 {
		log.Printf("[%s] Error re-fetching link info for URL %s: %v", userIdentifier, originalLinkURL, err)
		return
	}

	downloadURL := linkInfo.Tracks[0].URL
	if linkInfo.Tracks[0].OriginalURL != "" {
		downloadURL = linkInfo.Tracks[0].OriginalURL
	}
	if downloadURL == "" {
		downloadURL = originalLinkURL
	}

	b.processDownloadRequest(chatID, originalLinkMessageID, downloadURL, dlType, linkInfo.Tracks[0], userName, userID, fromFirstName)
}

func (b *Bot) processAlbumDownload(chatID int64, urlToDownload string, userIdentifier string, userName string, userID int64, fromFirstName string, statusMessageID int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[%s] RECOVERED from panic in processAlbumDownload: %v\n%s", userIdentifier, r, string(debug.Stack()))
			errorText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "❌ یک خطای داخلی بسیار جدی در حین دانلود آلبوم رخ داد و فرآیند متوقف شد.")
			b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMessageID, errorText))
		}
	}()

	log.Printf("[%s] Starting album download process for URL: %s", userIdentifier, urlToDownload)
	initialLinkInfo, err := b.downloader.GetLinkInfo(urlToDownload, userIdentifier)
	if err != nil || initialLinkInfo.Type != "album" || len(initialLinkInfo.Tracks) == 0 {
		log.Printf("[%s] Failed to get album info for batch download: %v", userIdentifier, err)
		errorText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "خطایی در دریافت مجدد اطلاعات آلبوم رخ داد\\. لطفاً دوباره تلاش کنید\\.")
		b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMessageID, errorText))
		return
	}

	totalTracks := len(initialLinkInfo.Tracks)
	escapedAlbumTitle := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, initialLinkInfo.Title)

	type downloadedTrack struct {
		FilePath  string
		TrackInfo *downloader.TrackInfo
	}
	var downloadedFiles []downloadedTrack
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := semaphore.NewWeighted(3)

	for _, shallowTrack := range initialLinkInfo.Tracks {
		wg.Add(1)
		sem.Acquire(context.Background(), 1)

		go func(trackToProcess *downloader.TrackInfo) {
			defer wg.Done()
			defer sem.Release(1)

			trackURL := trackToProcess.URL
			if trackToProcess.OriginalURL != "" {
				trackURL = trackToProcess.OriginalURL
			}
			if trackURL == "" {
				log.Printf("[%s] Skipping track (%s) because its URL is empty in the album list.", userIdentifier, trackToProcess.Title)
				return
			}

			detailedLinkInfo, err := b.downloader.GetLinkInfo(trackURL, userIdentifier)
			if err != nil || len(detailedLinkInfo.Tracks) == 0 {
				log.Printf("[%s] Failed to fetch detailed info for track (%s): %v. Skipping.", userIdentifier, trackURL, err)
				return
			}

			track := detailedLinkInfo.Tracks[0]

			downloadedFilePath, _, err := b.downloader.DownloadMedia(trackURL, userIdentifier, downloader.AudioOnly, track)
			if err != nil {
				log.Printf("[%s] Failed to download track %s: %v", userIdentifier, track.Title, err)
				return
			}

			mu.Lock()
			downloadedFiles = append(downloadedFiles, downloadedTrack{FilePath: downloadedFilePath, TrackInfo: track})
			progressText := fmt.Sprintf("تعداد %d از %d آهنگ با موفقیت دانلود شد...", len(downloadedFiles), totalTracks)
			editMsg := tgbotapi.NewEditMessageText(chatID, statusMessageID, progressText)
			editMsg.ParseMode = tgbotapi.ModeMarkdownV2
			b.api.Send(editMsg)
			mu.Unlock()

		}(shallowTrack)
	}

	wg.Wait()

	if len(downloadedFiles) == 0 {
		log.Printf("[%s] All tracks failed to download for album: %s", userIdentifier, urlToDownload)
		errorText := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "متاسفانه دانلود هیچ یک از آهنگ‌های آلبوم موفقیت‌آمیز نبود.")
		b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMessageID, errorText))
		return
	}

	b.api.Send(tgbotapi.NewDeleteMessage(chatID, statusMessageID))
	log.Printf("[%s] All %d tracks downloaded. Now sending as media group(s).", userIdentifier, len(downloadedFiles))

	chunkSize := 10
	for i := 0; i < len(downloadedFiles); i += chunkSize {
		end := i + chunkSize
		if end > len(downloadedFiles) {
			end = len(downloadedFiles)
		}
		chunk := downloadedFiles[i:end]

		mediaGroup := []interface{}{}
		for j, file := range chunk {
			audioFile := tgbotapi.NewInputMediaAudio(tgbotapi.FilePath(file.FilePath))
			audioFile.Title = file.TrackInfo.Title
			audioFile.Performer = file.TrackInfo.Artist
			if j == 0 {
				caption := fmt.Sprintf("✅ آلبوم: *%s*\nتوسط: `%s`\n\n@%s", escapedAlbumTitle, tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, initialLinkInfo.Uploader), tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, b.api.Self.UserName))
				audioFile.Caption = caption
				audioFile.ParseMode = tgbotapi.ModeMarkdownV2
			}
			mediaGroup = append(mediaGroup, audioFile)
		}

		if _, err := b.api.SendMediaGroup(tgbotapi.NewMediaGroup(chatID, mediaGroup)); err != nil {
			log.Printf("[%s] Error sending media group chunk %d: %v", userIdentifier, i/chunkSize+1, err)
		}
	}

	for _, file := range downloadedFiles {
		os.Remove(file.FilePath)
	}
	log.Printf("[%s] Album download and send process finished for: %s", userIdentifier, urlToDownload)
}

func (b *Bot) processDownloadRequest(chatID int64, originalLinkMessageID int, urlToDownload string, dlType downloader.DownloadType, trackInfo *downloader.TrackInfo, userName string, userID int64, fromFirstName string) {
	userIdentifier := userName + "_" + strconv.FormatInt(userID, 10)
	escapedArtist := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Artist)
	escapedTitle := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, trackInfo.Title)
	escapedFileType := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, typeToString(dlType))

	dlNoticeMsgText := ""
	if trackInfo.Title != "Unknown Title" && trackInfo.Artist != "Unknown Artist" {
		dlNoticeMsgText = fmt.Sprintf("در حال آماده‌سازی و دانلود *%s* برای:\n`%s \\- %s`\n\nاین فرآیند ممکن است کمی طول بکشد، لطفاً صبور باشید\\.\\.\\. ⏳", escapedFileType, escapedArtist, escapedTitle)
	} else {
		dlNoticeMsgText = fmt.Sprintf("در حال آماده‌سازی و دانلود *%s* شما\\.\\.\\. ⏳", escapedFileType)
	}
	dlNoticeMsg := tgbotapi.NewMessage(chatID, dlNoticeMsgText)
	dlNoticeMsg.ParseMode = tgbotapi.ModeMarkdownV2
	if originalLinkMessageID != 0 {
		dlNoticeMsg.ReplyToMessageID = originalLinkMessageID
	}
	sentMsg, err := b.api.Send(dlNoticeMsg)
	if err != nil {
		log.Printf("[%s] Error sending 'downloading media' message: %v", userIdentifier, err)
	}

	downloadedFilePath, actualExt, err := b.downloader.DownloadMedia(urlToDownload, userIdentifier, dlType, trackInfo)
	if sentMsg.MessageID != 0 {
		b.api.Send(tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID))
	}

	if err != nil {
		log.Printf("[%s] Error downloading media for URL %s: %v\n", userIdentifier, urlToDownload, err)
		escapedError := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, err.Error())
		errorMsgText := fmt.Sprintf("❌ متاسفانه در فرآیند دانلود برای آهنگ `%s` مشکلی پیش آمد\\.\n\nجزئیات خطا:\n`%s`", escapedTitle, escapedError)
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ParseMode = tgbotapi.ModeMarkdownV2
		if originalLinkMessageID != 0 {
			errMsg.ReplyToMessageID = originalLinkMessageID
		}
		b.api.Send(errMsg)
		return
	}

	log.Printf("[%s] Media downloaded: %s (ext: %s). Sending to user.\n", userIdentifier, downloadedFilePath, actualExt)

	escapedBotUsernameMention := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "@"+b.api.Self.UserName)

	if dlType == downloader.AudioOnly || actualExt == "mp3" {
		caption := fmt.Sprintf("🎵 *%s*\n👤 _%s_\n\n%s", escapedTitle, escapedArtist, escapedBotUsernameMention)
		audioFile := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(downloadedFilePath))
		audioFile.Title = trackInfo.Title
		audioFile.Performer = trackInfo.Artist
		audioFile.Caption = caption
		audioFile.ParseMode = tgbotapi.ModeMarkdownV2
		if originalLinkMessageID != 0 {
			audioFile.ReplyToMessageID = originalLinkMessageID
		}
		_, sendErr := b.api.Send(audioFile)
		if sendErr != nil {
			log.Printf("[%s] Error sending audio file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
		} else {
			log.Printf("[%s] Audio file %s sent successfully.\n", userIdentifier, downloadedFilePath)
		}
	} else if dlType == downloader.VideoBest || actualExt == "mp4" || actualExt == "mkv" || actualExt == "webm" {
		caption := fmt.Sprintf("🎬 *%s*\n👤 _%s_\n\n%s", escapedTitle, escapedArtist, escapedBotUsernameMention)
		videoFile := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			videoFile.ReplyToMessageID = originalLinkMessageID
		}
		videoFile.Caption = caption
		videoFile.ParseMode = tgbotapi.ModeMarkdownV2
		_, sendErr := b.api.Send(videoFile)
		if sendErr != nil {
			log.Printf("[%s] Error sending video file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
		} else {
			log.Printf("[%s] Video file %s sent successfully.\n", userIdentifier, downloadedFilePath)
		}
	} else if dlType == downloader.ImageBest || actualExt == "jpg" || actualExt == "jpeg" || actualExt == "webp" || actualExt == "png" {
		caption := fmt.Sprintf("🖼️ *%s*\n👤 _%s_\n\n%s", escapedTitle, escapedArtist, escapedBotUsernameMention)
		photoFile := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			photoFile.ReplyToMessageID = originalLinkMessageID
		}
		photoFile.Caption = caption
		photoFile.ParseMode = tgbotapi.ModeMarkdownV2
		_, sendErr := b.api.Send(photoFile)
		if sendErr != nil {
			log.Printf("[%s] Error sending photo file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
		} else {
			log.Printf("[%s] Photo file %s sent successfully.\n", userIdentifier, downloadedFilePath)
		}
	} else {
		log.Printf("[%s] Unknown/unhandled extension '%s', sending as document.\n", userIdentifier, actualExt)
		caption := fmt.Sprintf("📄 *%s*\n👤 _%s_\n\n%s", escapedTitle, escapedArtist, escapedBotUsernameMention)
		docFile := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(downloadedFilePath))
		if originalLinkMessageID != 0 {
			docFile.ReplyToMessageID = originalLinkMessageID
		}
		docFile.Caption = caption
		docFile.ParseMode = tgbotapi.ModeMarkdownV2
		_, sendErr := b.api.Send(docFile)
		if sendErr != nil {
			log.Printf("[%s] Error sending document file %s: %v\n", userIdentifier, downloadedFilePath, sendErr)
		} else {
			log.Printf("[%s] Document file %s sent successfully.\n", userIdentifier, downloadedFilePath)
		}
	}

	os.Remove(downloadedFilePath)
}

func typeToString(dlType downloader.DownloadType) string {
	if dlType == downloader.AudioOnly {
		return "فایل صوتی"
	}
	if dlType == downloader.VideoBest {
		return "فایل ویدیویی"
	}
	if dlType == downloader.ImageBest {
		return "فایل عکس"
	}
	return "فایل"
}
