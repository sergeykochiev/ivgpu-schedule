package tg 

import (
	"encoding/json"
	"net/http"
	"github.com/sergeykochiev/ivgpu-schedule/common"
)

const ApiBase = "https://api.telegram.org/bot"

const (
	endpointSendMessage = "sendMessage"
	endpointEditMessage = "editMessageText"
	endpointGetUpdates = "getUpdates"
)

type User struct {
	Id int `json:"id"`
}

type CallbackQuery struct {
	Id string `json:"id"`
	Message ReceivedMessage `json:"message"`
	Data string `json:"data"`
	From User `json:"from"`
}

type Update struct {
	UpdateId int     `json:"update_id"`
	Message  ReceivedMessage `json:"message"`
  CallbackQuery	CallbackQuery `json:"callback_query"`
}

func (u Update) ChatId() int {
	if u.IsCallbackQuery() {
		return u.CallbackQuery.From.Id
	}
	return u.Message.Chat.Id
}

func (u Update) IsCallbackQuery() bool {
	return u.CallbackQuery.Id != ""
}

type Chat struct {
	Id int `json:"id"`
}

type KeyboardButton struct {
	Text string `json:"text"`
}

type InlineKeyboardButton struct {
	Text string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type ReplyKeyboardMarkup struct {
	Keyboard [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard bool `json:"resize_keyboard"`
}

type ReceivedMessage struct {
	MessageId int `json:"message_id"`
	Text   string `json:"text"`
	Chat   Chat   `json:"chat"`
	From User `json:"from"`
}

type BaseSentMessage struct {
	ChatId int    `json:"chat_id"`
	Text   string `json:"text"`
}

type SentMessage[T any] struct {
	ChatId int    `json:"chat_id"`
	Text   string `json:"text"`
	ReplyMarkup T `json:"reply_markup"`
}

type BaseEditedMessage struct {
	ChatId int    `json:"chat_id"`
	MessageId int `json:"message_id"`
	Text   string `json:"text"`
}
type EditedMessage struct {
	ChatId int    `json:"chat_id"`
	MessageId int `json:"message_id"`
	Text   string `json:"text"`
	ReplyMarkup InlineKeyboardMarkup `json:"reply_markup"`
}

type Response[T any] struct {
	Ok     bool `json:"ok"`
	Result T    `json:"result"`
}

type Bot struct {
	token          string
	lastUpdateId   int
	allowedUpdates []string
}

type UpdatesRequest struct {
	Offset         int      `json:"offset"`
	Timeout        int      `json:"timeout"`
	AllowedUpdates []string `json:"allowed_updates"`
}

func InitTgBot(token string) Bot {
	return Bot{
		token: token,
		allowedUpdates: []string{"message", "callback_query"},
	}
}

func baseTgReq[T any](t *Bot, body T, endpoint string) (err error) {
	_, err = common.JsonReq[Response[any]](t.url(endpoint), body)
	return
}

func SendMsg[T any](t *Bot, m T) error {
	return baseTgReq(t, m, endpointSendMessage)
}

func EditMsg[T any](t *Bot, m T) error {
	return baseTgReq(t, m, endpointEditMessage)
}

func (t *Bot) SetLastUpdate(id int) {
	t.lastUpdateId = id
}

func (t *Bot) url(endpoint string) string {
	return ApiBase + t.token + "/" + endpoint
}

func (t *Bot) GetUpdates() (output []Update, err error) {
	var tgRes Response[[]Update]
	bodyBytes, err := json.Marshal(UpdatesRequest{
		Offset: t.lastUpdateId,
		AllowedUpdates: t.allowedUpdates,
		Timeout: 60,
	})
	if err != nil {
		return
	}
	res, err := common.Req(http.MethodPost, t.url(endpointGetUpdates), bodyBytes)
	if err != nil {
		return
	}
	err = json.NewDecoder(res.Body).Decode(&tgRes)
	if !tgRes.Ok {
		err = common.ErrNotOk
		return
	}
	output = tgRes.Result
	return
}
