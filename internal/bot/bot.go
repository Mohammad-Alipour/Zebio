package bot

import (
	"fmt"
	"log"
	"os"
	"strconv"

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
			b.handleLink(update.Message)
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

func (b *Bot) handleLink(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID
	userName := message.From.UserName
	if userName == "" {
		userName = message.From.FirstName
	}
	urlToDownload := message.Text

	log.Printf("[%s (%d)] Received link to process: %s\n", userName, userID, urlToDownload)

	processingMsg := tgbotapi.NewMessage(chatID, "در حال پردازش لینک شما... لطفاً کمی صبر کنید. ⏳")
	processingMsg.ReplyToMessageID = message.MessageID
	if _, err := b.api.Send(processingMsg); err != nil {
		log.Printf("[%s (%d)] Error sending 'processing' message: %v", userName, userID, err)
	}

	downloadedFilePath, err := b.downloader.DownloadAudio(urlToDownload, userName+"_"+strconv.FormatInt(userID, 10))
	if err != nil {
		log.Printf("[%s (%d)] Error downloading audio for URL %s: %v\n", userName, userID, urlToDownload, err)
		errorMsgText := fmt.Sprintf("متاسفانه در دانلود از لینک مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		errMsg.ReplyToMessageID = message.MessageID
		b.api.Send(errMsg)
		return
	}

	log.Printf("[%s (%d)] File downloaded successfully: %s. Attempting to send.\n", userName, userID, downloadedFilePath)

	sendingMsg := tgbotapi.NewMessage(chatID, "فایل صوتی با موفقیت دانلود شد. در حال ارسال... 📤")
	sendingMsg.ReplyToMessageID = message.MessageID
	b.api.Send(sendingMsg)

	audioFile := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(downloadedFilePath))
	audioFile.ReplyToMessageID = message.MessageID
	// audioFile.Title = "Downloaded Audio" // می‌توانید عنوان هم برای فایل صوتی بگذارید
	// audioFile.Performer = "Zebio Bot"   // یا نام اجراکننده

	if _, err := b.api.Send(audioFile); err != nil {
		log.Printf("[%s (%d)] Error sending audio file %s: %v\n", userName, userID, downloadedFilePath, err)
		errorMsgText := fmt.Sprintf("فایل دانلود شد اما در ارسال آن مشکلی پیش آمد.\nخطا: %s", err.Error())
		errMsg := tgbotapi.NewMessage(chatID, errorMsgText)
		b.api.Send(errMsg)
	} else {
		log.Printf("[%s (%d)] Audio file %s sent successfully.\n", userName, userID, downloadedFilePath)
	}

	log.Printf("[%s (%d)] Attempting to remove temporary file: %s\n", userName, userID, downloadedFilePath)
	errRemove := os.Remove(downloadedFilePath)
	if errRemove != nil {
		log.Printf("[%s (%d)] Error removing temporary file %s: %v\n", userName, userID, downloadedFilePath, errRemove)
	} else {
		log.Printf("[%s (%d)] Temporary file %s removed successfully.\n", userName, userID, downloadedFilePath)
	}
}
