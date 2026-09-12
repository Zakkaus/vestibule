package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
)

type panelAPICaller struct {
	admin                bool
	editErr              error
	chatUsername         string
	memberCalls          atomic.Int32
	commandMenus         atomic.Int32
	lastEditText         string
	lastAnswerText       string
	lastAnswerAlert      bool
	lastSendText         string
	lastURL              string
	sendChats            []int64
	sendTexts            []string
	messageID            int
	replyKeyboardRemoved bool
	senderUnbans         []telego.UnbanChatSenderChatParams
}

func (c *panelAPICaller) Call(_ context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	method := endpoint[strings.LastIndexByte(endpoint, '/')+1:]
	switch method {
	case "getMe":
		return panelAPIResponse(&telego.User{ID: 500, Username: "settings_test_bot", IsBot: true})
	case "getChat":
		var request struct {
			ChatID int64 `json:"chat_id"`
		}
		if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
			return nil, err
		}
		return panelAPIResponse(&telego.ChatFullInfo{
			ID: request.ChatID, Type: "supergroup", Title: fmt.Sprintf("Group %d", request.ChatID), Username: c.chatUsername,
		})
	case "getChatMember":
		c.memberCalls.Add(1)
		if c.admin {
			return panelAPIResponse(&telego.ChatMemberAdministrator{
				Status: telego.MemberStatusAdministrator, CanInviteUsers: true,
				CanRestrictMembers: true, CanDeleteMessages: true,
			})
		}
		return panelAPIResponse(&telego.ChatMemberMember{Status: telego.MemberStatusMember})
	case "setMyCommands":
		c.commandMenus.Add(1)
		return panelAPIResponse(true)
	case "editMessageText":
		return c.callEditMessageText(data)
	case "answerCallbackQuery":
		var request telego.AnswerCallbackQueryParams
		if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
			return nil, err
		}
		c.lastAnswerText = request.Text
		c.lastAnswerAlert = request.ShowAlert
		return panelAPIResponse(true)
	case "sendMessage":
		return c.callSendMessage(data)
	case "deleteMessage":
		return panelAPIResponse(true)
	case "unbanChatSenderChat":
		var request telego.UnbanChatSenderChatParams
		if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
			return nil, err
		}
		c.senderUnbans = append(c.senderUnbans, request)
		return panelAPIResponse(true)
	default:
		return nil, fmt.Errorf("unexpected Telegram method %q", method)
	}
}

func (c *panelAPICaller) callEditMessageText(data *ta.RequestData) (*ta.Response, error) {
	var request struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
		return nil, err
	}
	c.lastEditText = request.Text
	if c.editErr != nil {
		return nil, c.editErr
	}
	return panelAPIResponse(&telego.Message{MessageID: 90})
}

func (c *panelAPICaller) callSendMessage(data *ta.RequestData) (*ta.Response, error) {
	var request struct {
		ChatID      int64  `json:"chat_id"`
		Text        string `json:"text"`
		ReplyMarkup struct {
			InlineKeyboard [][]struct {
				URL string `json:"url"`
			} `json:"inline_keyboard"`
			RemoveKeyboard bool `json:"remove_keyboard"`
		} `json:"reply_markup"`
	}
	if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
		return nil, err
	}
	c.lastSendText = request.Text
	c.sendChats = append(c.sendChats, request.ChatID)
	c.sendTexts = append(c.sendTexts, request.Text)
	if len(request.ReplyMarkup.InlineKeyboard) > 0 && len(request.ReplyMarkup.InlineKeyboard[0]) > 0 {
		c.lastURL = request.ReplyMarkup.InlineKeyboard[0][0].URL
	}
	c.replyKeyboardRemoved = c.replyKeyboardRemoved || request.ReplyMarkup.RemoveKeyboard
	c.messageID++
	return panelAPIResponse(&telego.Message{MessageID: c.messageID})
}

func panelAPIResponse(value any) (*ta.Response, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &ta.Response{Ok: true, Result: raw}, nil
}

type blockingPanelCaller struct {
	delegate ta.Caller

	memberCalls     atomic.Int32
	blockMemberCall atomic.Int32
	memberStarted   chan struct{}
	releaseMember   chan struct{}

	editCalls     atomic.Int32
	blockEditCall atomic.Int32
	editStarted   chan struct{}
	releaseEdit   chan struct{}

	sendCalls     atomic.Int32
	blockSendCall atomic.Int32
	sendStarted   chan struct{}
	releaseSend   chan struct{}
}

func newBlockingPanelCaller(delegate ta.Caller) *blockingPanelCaller {
	return &blockingPanelCaller{
		delegate:      delegate,
		memberStarted: make(chan struct{}), releaseMember: make(chan struct{}),
		editStarted: make(chan struct{}), releaseEdit: make(chan struct{}),
		sendStarted: make(chan struct{}), releaseSend: make(chan struct{}),
	}
}

func (c *blockingPanelCaller) Call(ctx context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	method := endpoint[strings.LastIndexByte(endpoint, '/')+1:]
	switch method {
	case "getChatMember":
		call := c.memberCalls.Add(1)
		if call == c.blockMemberCall.Load() {
			c.memberStarted <- struct{}{}
			<-c.releaseMember
		}
	case "editMessageText":
		call := c.editCalls.Add(1)
		if call == c.blockEditCall.Load() {
			c.editStarted <- struct{}{}
			<-c.releaseEdit
		}
	case "sendMessage":
		call := c.sendCalls.Add(1)
		if call == c.blockSendCall.Load() {
			c.sendStarted <- struct{}{}
			<-c.releaseSend
		}
	}
	return c.delegate.Call(ctx, endpoint, data)
}
