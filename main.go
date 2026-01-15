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

const (
	LogInfo string = " INFO "
	LogWarn string = " WARN "
	LogErr string = " ERR "
)

type IAppLogger interface {
	Log(string, ...any)
	Fatal(...any)
}

type FileLogger struct {
	logfile *os.File
}

func NewFileLogger(logfile *os.File) FileLogger {
	return FileLogger{
		logfile: logfile,
	}
}

func (l FileLogger) Log(lvlPrefix string, msg ...any) {
	fmt.Fprint(l.logfile, time.Now())
	fmt.Fprint(l.logfile, lvlPrefix)
	fmt.Fprintln(l.logfile, msg...)
}

func (l FileLogger) Fatal(msg ...any) {
	l.Log(LogErr, msg...)
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

func initMainApp(token string, numWorkers int, whitelist []string, logger IAppLogger) (app MainApp, err error) {
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
		err = errors.Join(common.ErrConnectDb, err)
		return
	}
	app.db.Conn.SetMaxOpenConns(numWorkers)
	app.db.Conn.SetMaxIdleConns(numWorkers) 
	app.grouplist, err = api.GetGrouplist()
	if err != nil {
		err = errors.Join(common.ErrGetGroupList, err)
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
	app.logger.Log(LogErr, err)
	switch err {
	case common.ErrNoUser:
		tg.SendMsg(&app.bot, tg.BaseSentMessage{
			Text: "Используйте команду /start",
			ChatId: upd.ChatId(),
		})
		return
	case common.ErrNoGroupId:
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
			if errors.Is(err, common.ErrNoUser) {
				err = app.db.CreateUser(upd.ChatId())
				if err != nil {
					return errors.Join(common.ErrCreateUser, err)
				}
			} else if err != nil {
				return errors.Join(common.ErrGetUserById, err)
			}
			return app.initInstituteChoice(upd)
		default:
			return fmt.Errorf("Unsupported command: %s", upd.Message.Text[1:])
	}
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
		return errors.Join(common.ErrSetGroup, err)
	}
	err := tg.EditMsg(&app.bot, tg.BaseEditedMessage{
		Text: "Группа изменена успешно",
		MessageId: query.MessageId,
		ChatId: upd.ChatId(),
	})
	if err != nil {
		return errors.Join(common.ErrEditMsg, err)
	}
	return tg.SendMsg(&app.bot, tg.SentMessage[tg.ReplyKeyboardMarkup]{
		ChatId: upd.ChatId(),
		Text: groupName,
		ReplyMarkup: defaultInlineKeyboard(groupName, common.Weeknames[user.Week]),
	})
}

func (app *MainApp) acceptInstituteChoice(upd tg.Update, user db.User, query common.CallbackData) error {
	if err := app.db.SetUserInstitute(user.Id, query.Data); err != nil {
		return errors.Join(common.ErrSetUserInst, err)
	}
	err := app.initGroupChoice(upd, query)
	if err != nil {
		return errors.Join(common.ErrInitGroupChoice, err)
	}
	return nil
}
 
func (app *MainApp) handleCallbackQuery(upd tg.Update) error {
	query := common.ParseCallbackData(upd.CallbackQuery.Data)
	user, err := app.db.GetUserById(upd.ChatId())
	if err != nil {
		return errors.Join(common.ErrGetUserById, err)
	}
	if (user == db.User{}) {
		return common.ErrNoUser
	}
	if query.MessageId == 0 {
		query.MessageId = upd.CallbackQuery.Message.MessageId
	}
	switch (query.Typ) {
	case common.CallbackQueryTypeInstitute:
		err = app.acceptInstituteChoice(upd, user, query)
		if err != nil {
			return errors.Join(common.ErrAcceptInstChoice, err)
		}
	case common.CallbackQueryTypeGroups:
		err = app.acceptGroupChoice(upd, user, query)
		if err != nil {
			return errors.Join(common.ErrAcceptGroupChoice, err)
		}
	case common.CallbackQueryTypeWeek:
		err = app.acceptWeekChoice(upd, user, query)
		if err != nil {
			return errors.Join(common.ErrAcceptWeekChoice, err)
		}
	case common.CallbackQueryTypeChangeInstitute:
		err = app.initInstituteChoiceQuery(upd, query)
		if err != nil {
			return errors.Join(common.ErrInitInstChoice, err)
		}
	default:
		return fmt.Errorf("Unsupported callback query typ: %s", query.Typ)
	}
	return nil
}

func (app *MainApp) _getSchedule(user db.User) (schedule api.GroupResponse, err error) {
	if user.GroupId == 0 {
		err = common.ErrNoGroupId
		return
	}
	var ok bool
	if schedule, ok = app.groupsSchedules[user.GroupId]; !ok {
		schedule, err = api.GetGroup(user.GroupId)
		if err != nil {
			err = errors.Join(common.ErrSetGroup, err)
			return
		}
		app.groupsSchedules[user.GroupId] = schedule
	}
	return
}

func (app *MainApp) getSchedule(upd tg.Update, user db.User, t time.Time) error {
	s, err := app._getSchedule(user)
	if err != nil {
		return errors.Join(common.ErrGetSchedule, err)
	}
	return tg.SendMsg(&app.bot, tg.BaseSentMessage{
		ChatId: upd.ChatId(),
		Text: s.ByDate(t, user.Week),
	})
}

func (app *MainApp) getExams(upd tg.Update, user db.User) error {
	s, err := app._getSchedule(user)
	if err != nil {
		return errors.Join(common.ErrGetSchedule, err)
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
		return errors.Join(common.ErrWeekFromData, err)
	}
	err = app.db.SetUserWeek(user.Id, week)
	if err != nil {
		return errors.Join(common.ErrSetWeek, err)
	}
	err = tg.EditMsg(&app.bot, tg.BaseEditedMessage{
		ChatId: upd.ChatId(),
		MessageId: query.MessageId,
		Text: "Неделя сменена успешно",
	})
	if err != nil {
		return errors.Join(common.ErrEditMsg, err)
	}
	weekName := common.Weeknames[week]
	return tg.SendMsg(&app.bot, tg.SentMessage[tg.ReplyKeyboardMarkup]{
		ChatId: upd.ChatId(),
		Text: weekName,
		ReplyMarkup: defaultInlineKeyboard(user.GroupName, weekName),
	})
}

func (app *MainApp) handleMessage(upd tg.Update) error {
	var err error
	if upd.Message.Text[0] == '/' {
		err = app.handleCommand(upd)
		if err != nil {
			return errors.Join(common.ErrCommand, err)
		}
		return nil
	}
	user, err := app.db.GetUserById(upd.ChatId())
	if err != nil {
		return errors.Join(common.ErrGetUserById, err)
	}
	if (user == db.User{}) {
		return common.ErrNoUser
	}
	t := time.Now().Add(time.Hour * 3);
	if common.StartsWith(upd.Message.Text, common.ReplyKeyboardButtonChangeGroup) {
		err = app.initInstituteChoice(upd)
		if err != nil {
			return errors.Join(common.ErrInitInstChoice, err)
		}
		return nil
	}
	if common.StartsWith(upd.Message.Text, common.ReplyKeyboardButtonChangeWeek) {
		err = app.initWeekChoice(upd)
		if err != nil {
			return errors.Join(common.ErrInitWeekChoice, err)
		}
		return nil
	}
	switch (upd.Message.Text) {
	case common.ReplyKeyboardButtonExams:
		err = app.getExams(upd, user)
		if err != nil {
			return errors.Join(common.ErrGetExams, err)
		}
	case common.ReplyKeyboardButtonToday:
		user.Week = 0
		err = app.getSchedule(upd, user, t)
		if err != nil {
			return errors.Join(common.ErrGetToday, err)
		}
	case common.ReplyKeyboardButtonTomorrow:
		user.Week = 0
		err = app.getSchedule(upd, user, t.AddDate(0, 0, 1))
		if err != nil {
			return errors.Join(common.ErrGetTomorrow, err)
		}
	case common.ReplyKeyboardButtonMonday:
		err = app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 1))
		if err != nil {
			return errors.Join(common.ErrGetMon, err)
		}
	case common.ReplyKeyboardButtonTuesday:
		err = app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 2))
		if err != nil {
			return errors.Join(common.ErrGetTue, err)
		}
	case common.ReplyKeyboardButtonWednesday:
		err = app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 3))
		if err != nil {
			return errors.Join(common.ErrGetWed, err)
		}
	case common.ReplyKeyboardButtonThursday:
		err = app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 4))
		if err != nil {
			return errors.Join(common.ErrGetThu, err)
		}
	case common.ReplyKeyboardButtonFriday:
		err = app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 5))
		if err != nil {
			return errors.Join(common.ErrGetFri, err)
		}
	case common.ReplyKeyboardButtonSaturday:
		err = app.getSchedule(upd, user, t.AddDate(0, 0, -int(t.Weekday()) + 6))
		if err != nil {
			return errors.Join(common.ErrGetSat, err)
		}
	}
	return nil
}

func (app *MainApp) handleUpdate(upd tg.Update) error {
	var err error
	if upd.IsCallbackQuery() {
		err = app.handleCallbackQuery(upd)
		if err != nil {
			err = errors.Join(common.ErrHandleQuery, err)
		}
		return err 
	}
	err = app.handleMessage(upd)
	if err != nil {
		err = errors.Join(common.ErrHandleMessage, err)
	}
	return err
}

func (app *MainApp) NewWorker() {
	for upd := range app.updChan {
		if len(app.whitelist) > 0 && !slices.Contains(app.whitelist, strconv.Itoa(upd.ChatId())) {
			continue	
		}
		app.handleError(app.handleUpdate(upd), upd)
	}
}

func (app *MainApp) GetUpdates() {
	for range app.numWorkers {
		app.logger.Log(LogInfo, "Worker created")
		go app.NewWorker()
	}
	for {
		upds, err := app.bot.GetUpdates()
		if err != nil {
			app.handleError(errors.Join(common.ErrGetUpdates, err), tg.Update{})
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
	logfile, err := os.OpenFile("ivgpu-schedule.log", os.O_CREATE | os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Cannot open log file:", err)
	}
	logger := NewFileLogger(logfile)
	godotenv.Load()

	token := os.Getenv("TOKEN")
	if token == "" {
		logger.Fatal("Provide token through env")
	}

	numWorkers, err := strconv.Atoi(os.Getenv("NUM_WORKERS"))
	if err != nil {
		logger.Log(LogWarn, "Using default workers of count: 1")
		numWorkers = 1
	}

	whitelist := []string{}
	if whitelistEnv := os.Getenv("WHITELIST"); whitelistEnv != "" {
		whitelist = strings.Split(whitelistEnv, ",")
	} else {
		logger.Log(LogWarn, "Using empty whitelist")
	}

	mainApp, err := initMainApp(token, numWorkers, whitelist, logger)
	if err != nil {
		logger.Fatal(errors.Join(common.ErrInit, err))
	}
	logger.Log(LogInfo, "App running")
	mainApp.GetUpdates()
}
