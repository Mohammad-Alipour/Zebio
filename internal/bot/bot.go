package bot

import (
	"log"
	// مسیر پکیج config شما
	"github.com/Mohammad-Alipour/Zebio/internal/config" // اگر نام ماژول متفاوت است، تغییر دهید

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot ساختاری برای نگهداری نمونه API ربات و تنظیمات است.
type Bot struct {
	api *tgbotapi.BotAPI
	cfg *config.Config
}

// New یک نمونه جدید از Bot ایجاد و برمی‌گرداند.
// این تابع سعی می‌کند به API تلگرام متصل شود.
func New(cfg *config.Config) (*Bot, error) {
	if cfg.TelegramBotToken == "" {
		log.Fatal("Telegram Bot Token is not configured. Cannot start bot.")
		// در واقعیت، چون config.Load باید این را چک می‌کرد، اینجا نباید برسیم
		// اما بررسی مجدد ضرری ندارد.
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, err // برگرداندن خطا اگر اتصال ناموفق بود
	}

	// (اختیاری) فعال کردن حالت دیباگ برای دیدن درخواست‌ها و پاسخ‌های API
	// api.Debug = true

	log.Printf("Authorized on account %s (@%s)\n", api.Self.FirstName, api.Self.UserName)

	return &Bot{
		api: api,
		cfg: cfg,
	}, nil
}

// Start شروع به گوش دادن به پیام‌ها و دستورات می‌کند.
// این تابع به صورت یک حلقه بی‌نهایت اجرا می‌شود تا زمانی که برنامه متوقف شود.
func (b *Bot) Start() {
	log.Println("Bot is starting to listen for updates...")
	u := tgbotapi.NewUpdate(0) // 0 یعنی از آخرین آپدیت شروع کن
	u.Timeout = 60             // Timeout ۶۰ ثانیه‌ای برای دریافت آپدیت‌ها

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil { // اگر آپدیت پیام نبود، نادیده بگیر
			continue
		}

		userID := update.Message.From.ID
		log.Printf("[%s (%d)] %s\n", update.Message.From.UserName, userID, update.Message.Text)

		// بررسی اینکه آیا کاربر مجاز است (اگر لیست کاربران مجاز تعریف شده باشد)
		if len(b.cfg.AllowedUserIDs) > 0 {
			isAllowed := false
			for _, allowedID := range b.cfg.AllowedUserIDs {
				if userID == allowedID {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				log.Printf("User %s (%d) is not allowed. Ignoring message.", update.Message.From.UserName, userID)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "متاسفم، شما اجازه استفاده از این ربات را ندارید.")
				b.api.Send(msg)
				continue
			}
		}

		// پاسخ ساده به دستور /start
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				msgText := "سلام " + update.Message.From.FirstName + "! 👋\n"
				msgText += "من ربات دانلودر شما هستم. لینک خود را برای دانلود ارسال کنید."
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
				msg.ReplyToMessageID = update.Message.MessageID
				if _, err := b.api.Send(msg); err != nil {
					log.Printf("Error sending message: %v", err)
				}
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "دستور شناخته نشد.")
				if _, err := b.api.Send(msg); err != nil {
					log.Printf("Error sending message: %v", err)
				}
			}
		} else {
			// فعلاً برای پیام‌های غیر دستوری (که انتظار داریم لینک باشن)
			// فقط یک پاسخ موقت میدیم. در مراحل بعد این قسمت تکمیل میشه.
			responseText := "لینک دریافت شد: " + update.Message.Text + "\n"
			responseText += "در حال حاضر قابلیت دانلود پیاده‌سازی نشده است."
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, responseText)
			if _, err := b.api.Send(msg); err != nil {
				log.Printf("Error sending message: %v", err)
			}
		}
	}
}
