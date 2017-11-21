package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	bot "github.com/meinside/telegram-bot-go"
)

const (
	// for monitoring
	defaultMonitorIntervalSeconds = 3

	// commands
	commandStart        = "/start"
	commandPreset       = "/preset"
	commandChangePreset = "/presetchange"
	commandHelp         = "/help"
	commandCancel       = "/cancel"

	// messages
	messageDefault          = "Record your voice to start."
	messageSelectPreset     = "Select a preset."
	messageNoPreset         = "No preset available."
	messageNoMatchingPreset = "No such preset"
	messagePresetChanged    = "Applied preset"
	messagePresetNotSet     = "Preset not set"
	messageUnknownCommand   = "Unknown command"
	messageCancel           = "Cancel"
	messageCanceled         = "Canceled."
)

type session struct {
	UserId         string
	SelectedPreset string
}

type sessionPool struct {
	Sessions map[string]session
	sync.Mutex
}

const (
	configFilename = "config.json"
)

// variables
var soxPath string
var soxPresets map[string][]string
var soxDefaultPreset = []string{"speed", "1.0"}
var apiToken string
var monitorInterval int
var isVerbose bool
var availableIds []string
var pool sessionPool

// keyboards
var allKeyboards = [][]bot.KeyboardButton{
	bot.NewKeyboardButtons(commandPreset, commandHelp),
}

// struct for config file
type config struct {
	SoxBinPath       string              `json:"sox_bin"`
	SoxPresetOptions map[string][]string `json:"sox_presets"`
	ApiToken         string              `json:"api_token"`
	AvailableIds     []string            `json:"available_ids"`
	MonitorInterval  int                 `json:"monitor_interval"`
	IsVerbose        bool                `json:"is_verbose"`
}

// Read config
func getConfig() (cfg config, err error) {
	_, filename, _, _ := runtime.Caller(0) // = __FILE__

	if file, err := ioutil.ReadFile(filepath.Join(path.Dir(filename), configFilename)); err == nil {
		var conf config
		if err := json.Unmarshal(file, &conf); err == nil {
			return conf, nil
		} else {
			return config{}, err
		}
	} else {
		return config{}, err
	}
}

// initialization
func init() {
	// read variables from config file
	if cfg, err := getConfig(); err == nil {
		soxPath = cfg.SoxBinPath
		soxPresets = cfg.SoxPresetOptions
		apiToken = cfg.ApiToken
		availableIds = cfg.AvailableIds
		monitorInterval = cfg.MonitorInterval
		if monitorInterval <= 0 {
			monitorInterval = defaultMonitorIntervalSeconds
		}
		isVerbose = cfg.IsVerbose

		// initialize variables
		sessions := make(map[string]session)
		for _, v := range availableIds {
			sessions[v] = session{
				UserId: v,
			}
		}
		pool = sessionPool{
			Sessions: sessions,
		}
	} else {
		panic(err.Error())
	}
}

// check if given Telegram id is available
func isAvailableId(id string) bool {
	for _, v := range availableIds {
		if v == id {
			return true
		}
	}
	return false
}

// for showing help message
func getHelp() string {
	return `
Following commands are supported:

/preset: change preset
/help : show this help message
`
}

// process incoming update from Telegram
func processUpdate(b *bot.Bot, update bot.Update) bool {
	// check username
	var userId string
	if update.Message.From.Username == nil {
		log.Printf("*** Not allowed (no user name): %s", update.Message.From.FirstName)
		return false
	}
	userId = *update.Message.From.Username
	if !isAvailableId(userId) {
		log.Printf("*** Id not allowed: %s", userId)
		return false
	}

	// process result
	result := false

	pool.Lock()
	if session, exists := pool.Sessions[userId]; exists {
		// text from message
		var txt string
		if update.Message.HasText() {
			txt = *update.Message.Text
		} else {
			txt = ""
		}

		var message string
		var options = map[string]interface{}{
			"reply_markup": bot.ReplyKeyboardMarkup{
				Keyboard:       allKeyboards,
				ResizeKeyboard: true,
			},
			//"parse_mode": bot.ParseModeMarkdown,
		}

		if update.Message.HasVoice() {
			// recording voice...
			b.SendChatAction(update.Message.Chat.Id, bot.ChatActionRecordAudio)

			// send synthesized voice
			if data, err := synthesizeVoiceWithFileId(b, update.Message.Voice.FileId, session.SelectedPreset); err == nil {
				// uploading voice...
				b.SendChatAction(update.Message.Chat.Id, bot.ChatActionUploadAudio)

				// voice caption
				if len(session.SelectedPreset) > 0 {
					options["caption"] = fmt.Sprintf("%s (%s)", session.SelectedPreset, strings.Join(soxPresets[session.SelectedPreset], " "))
				} else {
					options["caption"] = messagePresetNotSet
				}

				// upload voice
				if sent := b.SendVoice(update.Message.Chat.Id, bot.InputFileFromBytes(data), options); sent.Ok {
					result = true
				} else {
					log.Printf("*** Failed to send photo: %s", *sent.Description)
				}
			} else {
				log.Printf("*** Voice synthesis failed: %s", err)

				message = fmt.Sprintf("Failed to synthesize voice: %s", err.Error())
				b.SendMessage(update.Message.Chat.Id, message, options)
			}
		} else {
			switch {
			// start
			case strings.HasPrefix(txt, commandStart):
				message = messageDefault
			case strings.HasPrefix(txt, commandPreset):
				if len(soxPresets) > 0 {
					message = messageSelectPreset

					keys := map[string]string{}
					for k := range soxPresets {
						keys[k] = fmt.Sprintf("%s %s", commandChangePreset, k)
					}
					keys[messageCancel] = commandCancel

					options["reply_markup"] = bot.InlineKeyboardMarkup{
						InlineKeyboard: bot.NewInlineKeyboardButtonsAsRowsWithCallbackData(keys),
					}
				} else {
					message = messageNoPreset
				}
			// help
			case strings.HasPrefix(txt, commandHelp):
				message = getHelp()
			// fallback
			default:
				message = fmt.Sprintf("%s: %s", messageUnknownCommand, txt)
			}

			// send message
			if sent := b.SendMessage(update.Message.Chat.Id, message, options); sent.Ok {
				result = true
			} else {
				log.Printf("*** Failed to send message: %s", *sent.Description)
			}
		}
	} else {
		log.Printf("*** Session does not exist for id: %s", userId)
	}
	pool.Unlock()

	return result
}

// process incoming callback query
func processCallbackQuery(b *bot.Bot, update bot.Update) bool {
	query := *update.CallbackQuery
	txt := *query.Data

	// process result
	result := false

	var message string
	if strings.HasPrefix(txt, commandChangePreset) {
		preset := strings.TrimSpace(strings.TrimPrefix(txt, commandChangePreset))

		if _, exists := soxPresets[preset]; exists {
			userId := *query.From.Username
			if !isAvailableId(userId) {
				log.Printf("*** Id not allowed: %s", userId)
			} else {
				// change preset
				pool.Sessions[userId] = session{
					UserId:         userId,
					SelectedPreset: preset,
				}

				message = fmt.Sprintf("%s: %s", messagePresetChanged, preset)
			}
		} else {
			message = fmt.Sprintf("%s: %s", messageNoMatchingPreset, preset)
		}
	} else if strings.HasPrefix(txt, commandCancel) {
		message = messageCanceled
	} else {
		log.Printf("*** Unprocessable callback query: %s", txt)
	}

	if len(message) > 0 {
		// answer callback query
		if apiResult := b.AnswerCallbackQuery(query.Id, map[string]interface{}{"text": message}); apiResult.Ok {
			// edit message and remove inline keyboards
			options := map[string]interface{}{
				"chat_id":    query.Message.Chat.Id,
				"message_id": query.Message.MessageId,
			}
			if apiResult := b.EditMessageText(message, options); apiResult.Ok {
				result = true
			} else {
				log.Printf("*** Failed to edit message text: %s", *apiResult.Description)
			}
		} else {
			log.Printf("*** Failed to answer callback query: %+v", query)
		}
	}

	return result
}

// synthesize voice from given file_id
func synthesizeVoiceWithFileId(b *bot.Bot, fileId string, preset string) ([]byte, error) {
	if f := b.GetFile(fileId); f.Ok {
		if res, err := http.Get(b.GetFileUrl(*f.Result)); err == nil {
			defer res.Body.Close()
			// get bytes of given voice file
			if data, err := ioutil.ReadAll(res.Body); err == nil {
				return soxConvert(data, preset)
			} else {
				return []byte{}, err
			}
		} else {
			return []byte{}, err
		}
	} else {
		return []byte{}, fmt.Errorf("Failed to get file: %s", *f.Description)
	}
}

// convert given bytes using sox and preset
//
// eg) $ cat "original.oga" | sox -t opus - -t ogg - speed 2.0 > "converted.ogg"
func soxConvert(original []byte, preset string) ([]byte, error) {
	if isVerbose {
		log.Printf("Received: %s (%d bytes)", http.DetectContentType(original), len(original))
	}

	// command line arguments
	args := []string{
		// default arguments
		"-t", "opus", "-", // input from stdin
		"-t", "ogg", "-", // output to stdout
	}
	// presets as additional arguments
	if p, exists := soxPresets[preset]; exists {
		args = append(args, p...)
	} else {
		args = append(args, soxDefaultPreset...)
	}

	// execute command
	out, errs := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := exec.Command(soxPath, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(original), out, errs
	if err := cmd.Run(); err == nil {
		return out.Bytes(), nil
	} else {
		return []byte{}, fmt.Errorf("%s %s", string(out.Bytes()), string(errs.Bytes()))
	}
}

func main() {
	client := bot.NewClient(apiToken)
	client.Verbose = isVerbose

	// get info about this bot
	if me := client.GetMe(); me.Ok {
		log.Printf("Launching bot: @%s (%s)", *me.Result.Username, me.Result.FirstName)

		// delete webhook (getting updates will not work when wehbook is set up)
		if unhooked := client.DeleteWebhook(); unhooked.Ok {
			// wait for new updates
			client.StartMonitoringUpdates(0, monitorInterval, func(b *bot.Bot, update bot.Update, err error) {
				if err == nil {
					if update.HasMessage() {
						processUpdate(b, update)
					} else if update.HasCallbackQuery() {
						processCallbackQuery(b, update)
					}
				} else {
					log.Printf("*** Error while receiving update (%s)", err.Error())
				}
			})
		} else {
			panic("Failed to delete webhook")
		}
	} else {
		panic("Failed to get info of the bot")
	}
}
