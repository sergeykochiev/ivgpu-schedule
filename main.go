package main

import (
	"log"
	"fmt"
	"os"
	"errors"
	"strings"
	"slices"
	"time"
	"strconv"
	"github.com/joho/godotenv"
	"github.com/sergeykochiev/ivgpu-schedule/tg"
	"github.com/sergeykochiev/ivgpu-schedule/api"
	"github.com/sergeykochiev/ivgpu-schedule/db"
	"github.com/sergeykochiev/ivgpu-schedule/common"
)

type IAppLogger interface {
	Info(a ...any)
	Warn(a ...any)
	Err(a ...any)
	Panic(a ...any)
}

type AppLogger struct{} 

func (l AppLogger) Info(a ...any) {
	log.Println(a...)
}

func (l AppLogger) Warn(a ...any) {
	fmt.Print("\033[33m")
	log.Print(a...)
	fmt.Println("\033[39m")
}

func (l AppLogger) Err(a ...any) {
	fmt.Print("\033[31m")
	log.Print(a...)
	fmt.Println("\033[39m")
}

func (l AppLogger) Panic(a ...any) {
	l.Err(a...)
	os.Exit(1)
}

type MainApp struct {
	bot tg.Bot
	db db.AppDb
	whitelist []string
	logger IAppLogger
	numWorkers int
	updChan chan tg.Update
	grouplist api.GrouplistResponse
	groupsSchedules map[int]api.GroupResponse
}

const AppDbName = "schedule.db"

func initMainApp(token string, numWorkers int, whitelist []string, logger IAppLogger) (app MainApp) {
	var err error
	app.whitelist = whitelist
	app.logger = logger
	app.numWorkers = numWorkers
	app.updChan = make(chan tg.Update, 1)
	app.groupsSchedules = make(map[int]api.GroupResponse)
	// TODO handle invalid token
	app.bot = tg.InitTgBot(token)
	app.db, err = db.InitAppDb("postgres", db.PostgresConnStr(
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_DB"),
		"sslmode=disable",
	))
	if err != nil {
		app.logger.Panic("Не удалось подключиться к БД: " + err.Error())
	}
	app.db.Conn.SetMaxOpenConns(numWorkers)
	app.db.Conn.SetMaxIdleConns(numWorkers) 
	app.grouplist, err = api.GetGrouplist()
	if err != nil {
		app.logger.Panic("Не удалось получить список групп: " + err.Error())
	}
	return
}

func defaultInlineKeyboard(group, week string) tg.ReplyKeyboardMarkup {
	return tg.ReplyKeyboardMarkup{
		Keyboard: [][]tg.KeyboardButton{
			{
				{ Text: common.ReplyKeyboardButtonToday },
				{ Text: common.ReplyKeyboardButtonTomorrow },
			},
			{
				{ Text: common.ReplyKeyboardButtonMonday },
				{ Text: common.ReplyKeyboardButtonTuesday },
				{ Text: common.ReplyKeyboardButtonWednesday },
				{ Text: common.ReplyKeyboardButtonThursday },
				{ Text: common.ReplyKeyboardButtonFriday },
				{ Text: common.ReplyKeyboardButtonSaturday },
			},
			{
				{ Text: common.ReplyKeyboardButtonExams },
			},
			{
				{ Text: common.StatefulButton(common.ReplyKeyboardButtonChangeGroup, group ) },
			},
			{

				{ Text: common.StatefulButton(common.ReplyKeyboardButtonChangeWeek, week ) },
			},
		},
		ResizeKeyboard: true,
	}
}

func (app *MainApp) handleError(err error, upd tg.Update) {
	if err == nil {
		return
	}
	app.logger.Err(err)
	switch err {
	case db.ErrNoUser:
		tg.SendMsg(&app.bot, tg.BaseSentMessage{
			Text: "1Используйте команду /start",
			ChatId: upd.ChatId(),
		})
		return
	case db.ErrNoGroupId:
		tg.SendMsg(&app.bot, tg.BaseSentMessage{
			Text: "Сначала выберите группу",
			ChatId: upd.ChatId(),
		})
		return
	}
	tg.SendMsg(&app.bot, tg.BaseSentMessage{
		Text: "Произошла неизвестная ошибка",
		ChatId: upd.ChatId(),
	})
}


func (app *MainApp) initInstituteChoiceQuery(upd tg.Update, query common.CallbackData) error {
	return tg.EditMsg(&app.bot, tg.EditedMessage{
		Text: "Пожалуйста, выберите свое направление (институт)",
		ChatId: upd.ChatId(),
		MessageId: query.MessageId,
		ReplyMarkup: app.grouplist.InlineButtons(),
	})
}

func (app *MainApp) initInstituteChoice(upd tg.Update) error {
	return tg.SendMsg(&app.bot, tg.SentMessage[tg.InlineKeyboardMarkup]{
		Text: "Пожалуйста, выберите свое направление (институт)",
		ChatId: upd.ChatId(),
		ReplyMarkup: app.grouplist.InlineButtons(),
	})
}

func (app *MainApp) initGroupChoice(upd tg.Update, query common.CallbackData) error {
	var groups api.GrouplistGroupList
	for _, inst := range(app.grouplist) {
		if inst.Abbreviate == query.Data {
			groups = inst.Groups
			break
		}
	}
	return tg.EditMsg(&app.bot, tg.EditedMessage{
		Text: "Пожалуйста, выберите свою группу",
		MessageId: query.MessageId,
		ChatId: upd.ChatId(),
		ReplyMarkup: groups.InlineButtons(query.MessageId),
	})
}

func (app *MainApp) handleCommand(upd tg.Update) error {
	switch (upd.Message.Text[1:]) {
		case "start":
			_, err := app.db.GetUserById(upd.ChatId())
			if errors.Is(err, db.ErrNoUser) {
				err = app.db.CreateUser(upd.ChatId())
			}
			if err != nil {
				return err
			}
			return app.initInstituteChoice(upd)
		default:
			app.logger.Warn("Получена неизвестная команда: " + upd.Message.Text[1:])
	}
	return nil
}

func (app *MainApp) acceptGroupChoice(upd tg.Update, user db.User, query common.CallbackData) error {
	groupId, _ := strconv.Atoi(query.Data)
	var inst api.GrouplistInstitute
	for _, i := range(app.grouplist) {
		if i.Abbreviate == user.InstituteAbr {
			inst = i
			break
		}
	}
	var groupName string
	for _, g := range(inst.Groups) {
		if groupId == g.Id {
			groupName = g.Title
			break
		}
	}
	if err := app.db.SetUserGroup(user.Id, groupId, groupName); err != nil {
		return err
	}
	err := tg.EditMsg(&app.bot, tg.BaseEditedMessage{
		Text: "Группа изменена успешно",
		MessageId: query.MessageId,
		ChatId: upd.ChatId(),
	})
	if err != nil {
		return err
	}
	return tg.SendMsg(&app.bot, tg.SentMessage[tg.ReplyKeyboardMarkup]{
		ChatId: upd.ChatId(),
		Text: groupName,
		ReplyMarkup: defaultInlineKeyboard(groupName, common.Weeknames[user.Week]),
	})
}

func (app *MainApp) acceptInstituteChoice(upd tg.Update, user db.User, query common.CallbackData) error {
	if err := app.db.SetUserInstitute(user.Id, query.Data); err != nil {
		return err
	}
	return app.initGroupChoice(upd, query)
}
 
func (app *MainApp) handleCallbackQuery(upd tg.Update) error {
	query := common.ParseCallbackData(upd.CallbackQuery.Data)
	user, err := app.db.GetUserById(upd.ChatId())
	if err != nil {
		return err
	}
	if (user == db.User{}) {
		return db.ErrNoUser
	}
	if query.MessageId == 0 {
		query.MessageId = upd.CallbackQuery.Message.MessageId
	}
	switch (query.Typ) {
	case common.CallbackQueryTypeInstitute:
		return app.acceptInstituteChoice(upd, user, query)
	case common.CallbackQueryTypeGroups:
		return app.acceptGroupChoice(upd, user, query)
	case common.CallbackQueryTypeWeek:
		return app.acceptWeekChoice(upd, user, query)
	case common.CallbackQueryTypeChangeInstitute:
		return app.initInstituteChoiceQuery(upd, query)
	default:
		app.logger.Err("НЕДОСТИЖИМО: " + query.Typ)
	}
	return nil
}

func (app *MainApp) _getSchedule(user db.User) (schedule api.GroupResponse, err error) {
	if user.GroupId == 0 {
		err = db.ErrNoGroupId
		return
	}
	var ok bool
	if schedule, ok = app.groupsSchedules[user.GroupId]; !ok {
		schedule, err = api.GetGroup(user.GroupId)
		if err != nil {
			err = errors.Join(errors.New("Не удалось получить расписание: "), err)
			return
		}
		app.groupsSchedules[user.GroupId] = schedule
	}
	return
}

func (app *MainApp) getSchedule(upd tg.Update, user db.User, t time.Time) error {
	s, err := app._getSchedule(user)
	if err != nil {
	  return err
	}
	return tg.SendMsg(&app.bot, tg.BaseSentMessage{
		ChatId: upd.ChatId(),
		Text: s.ByDate(t, user.Week),
	})
}

func (app *MainApp) getExams(upd tg.Update, user db.User) error {
	s, err := app._getSchedule(user)
	if err != nil {
	  return err
	}
	return tg.SendMsg(&app.bot, tg.BaseSentMessage{
		ChatId: upd.ChatId(),
		Text: s.Exams(),
	})
}

func (app *MainApp) initWeekChoice(upd tg.Update) error {
	return tg.SendMsg(&app.bot, tg.SentMessage[tg.InlineKeyboardMarkup]{
		ChatId: upd.ChatId(),
		Text: "Пожалуйста, выберите неделю",
		ReplyMarkup: tg.InlineKeyboardMarkup{
			InlineKeyboard: [][]tg.InlineKeyboardButton{
				{
					{ Text: "1", CallbackData: common.CallbackData{
						Typ: common.CallbackQueryTypeWeek,
						Data: "1",
					}.ToJson() },
					{ Text: "2", CallbackData: common.CallbackData{
						Typ: common.CallbackQueryTypeWeek,
						Data: "2",
					}.ToJson() },
				},
				{
					{ Text: "Текущая", CallbackData: common.CallbackData{
						Typ: common.CallbackQueryTypeWeek,
						Data: "0",
					}.ToJson() },
				},
			},
		},
	})
}

func (app *MainApp) acceptWeekChoice(upd tg.Update, user db.User, query common.CallbackData) error {
	week, err := strconv.Atoi(query.Data)
	if err != nil {
		return err
	}
	err = app.db.SetUserWeek(user.Id, week)
	if err != nil {
		return err
	}
	err = tg.EditMsg(&app.bot, tg.BaseEditedMessage{
		ChatId: upd.ChatId(),
		MessageId: query.MessageId,
		Text: "Неделя сменена успешно",
	})
	if err != nil {
		return err
	}
	weekName := common.Weeknames[week]
	return tg.SendMsg(&app.bot, tg.SentMessage[tg.ReplyKeyboardMarkup]{
		ChatId: upd.ChatId(),
		Text: weekName,
		ReplyMarkup: defaultInlineKeyboard(user.GroupName, weekName),
	})
}

func (app *MainApp) handleMessage(upd tg.Update) error {
	if upd.Message.Text[0] == '/' {
		return app.handleCommand(upd)
	}
	user, err := app.db.GetUserById(upd.ChatId())
	if err != nil {
		return err
	}
	if (user == db.User{}) {
		return db.ErrNoUser
	}
	t := time.Now().Add(time.Hour * 3);
	if common.StartsWith(upd.Message.Text, common.ReplyKeyboardButtonChangeGroup) {
		return app.initInstituteChoice(upd)
	}
	if common.StartsWith(upd.Message.Text, common.ReplyKeyboardButtonChangeWeek) {
		return app.initWeekChoice(upd)
	}
	switch (upd.Message.Text) {
	case common.ReplyKeyboardButtonExams:
		return app.getExams(upd, user)
	case common.ReplyKeyboardButtonToday:
		user.Week = 0
		return app.getSchedule(upd, user, t)
	case common.ReplyKeyboardButtonTomorrow:
		user.Week = 0
		return app.getSchedule(upd, user, t.AddDate(0, 0, 1))
	case common.ReplyKeyboardButtonMonday:
		return app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 1))
	case common.ReplyKeyboardButtonTuesday:
		return app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 2))
	case common.ReplyKeyboardButtonWednesday:
		return app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 3))
	case common.ReplyKeyboardButtonThursday:
		return app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 4))
	case common.ReplyKeyboardButtonFriday:
		return app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 5))
	case common.ReplyKeyboardButtonSaturday:
		return app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 6))
	}
	return nil
}

func (app *MainApp) handleUpdate(upd tg.Update) error {
	if upd.IsCallbackQuery() {
		return app.handleCallbackQuery(upd)
	}
	return app.handleMessage(upd)
}

func (app *MainApp) NewWorker() {
	for upd := range app.updChan {
		if !slices.Contains(app.whitelist, strconv.Itoa(upd.ChatId())) {
			continue	
		}
		app.handleError(app.handleUpdate(upd), upd)
	}
}

func (app *MainApp) GetUpdates() {
	for range app.numWorkers {
		app.logger.Info("Создан воркер")
		go app.NewWorker()
	}
	for {
		upds, err := app.bot.GetUpdates()
		if err != nil {
			app.handleError(err, tg.Update{})
			continue
		}
		for _, upd := range upds {
			app.bot.SetLastUpdate(upd.UpdateId + 1)
			app.updChan <- upd
		}
	}
}

// TODO:
// - timer to reset schedule cache daily?
func main() {
	logger := AppLogger{}
	godotenv.Load()

	token := os.Getenv("TOKEN")
	if token == "" {
		logger.Panic("Предоставьте токен через ENV")
	}

	numWorkers, err := strconv.Atoi(os.Getenv("NUM_WORKERS"))
	if err != nil {
		logger.Warn("Количество воркеров не предоставлено, используется 1 по умолчанию")
		numWorkers = 1
	}

	whitelist := []string{}
	if whitelistEnv := os.Getenv("WHITELIST"); whitelistEnv != "" {
		whitelist = strings.Split(whitelistEnv, ",")
	} else {
		logger.Warn("Вайтлист пустой!")
	}

	mainApp := initMainApp(token, numWorkers, whitelist, logger)
	log.Println("Приложение запущено")
	mainApp.GetUpdates()
}
