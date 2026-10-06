package main

import (
	"net/http"
	"strings"

	"reasonix/internal/bot"
	"reasonix/internal/botruntime"
	appconfig "reasonix/internal/config"
)

type botPairingView struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Requests        []bot.PairingRequest `json:"requests"`
}
type botPairingChange struct {
	Action string `json:"action"`
	Code   string `json:"code"`
}

func (b *bridgeServer) botPairing(w http.ResponseWriter, _ *http.Request) {
	requests, err := bot.ListPairingRequests()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read pairing requests")
		return
	}
	if requests == nil {
		requests = []bot.PairingRequest{}
	}
	writeJSON(w, http.StatusOK, botPairingView{1, requests})
}

func (b *bridgeServer) changeBotPairing(w http.ResponseWriter, r *http.Request) {
	var input botPairingChange
	if err := decodeJSONBody(w, r, 1024, &input); err != nil || (input.Action != "approve" && input.Action != "reject") || len(input.Code) > 32 || strings.TrimSpace(input.Code) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid pairing action")
		return
	}
	if input.Action == "approve" {
		// Never turn a stale connection-owned request into a global platform grant.
		requests, err := bot.ListPairingRequests()
		cfg, cfgErr := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
		if err != nil || cfgErr != nil {
			writeProtocolError(w, http.StatusConflict, "conflict", "pairing configuration unavailable; retry after fixing it")
			return
		}
		valid := false
		for _, req := range requests {
			if !strings.EqualFold(req.Code, strings.TrimSpace(input.Code)) {
				continue
			}
			valid = req.ConnectionID == "" || req.ConnectionID == string(req.Platform)
			if req.ConnectionID != "" && req.ConnectionID != string(req.Platform) {
				for _, conn := range cfg.Bot.Connections {
					if botruntime.ConnectionRuntimeID(conn) == req.ConnectionID && conn.Provider == string(req.Platform) && normalizedBotDomain(conn.Domain) == normalizedBotDomain(req.Domain) {
						valid = true
						break
					}
				}
			}
		}
		if !valid {
			writeProtocolError(w, http.StatusConflict, "conflict", "pairing request expired or its connection was removed")
			return
		}
		if _, err := bot.ApprovePairingCode(input.Code); err != nil {
			writeProtocolError(w, http.StatusConflict, "conflict", "pairing approval could not be saved; retry")
			return
		}
		b.botRuntime.refreshAsync(nil)
	} else {
		if _, err := bot.RejectPairingCode(input.Code); err != nil {
			writeProtocolError(w, http.StatusConflict, "conflict", "pairing request not found or expired")
			return
		}
	}
	b.botPairing(w, r)
}
