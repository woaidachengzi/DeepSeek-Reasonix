package main

import (
	"net/http"

	"reasonix/internal/boot"
)

type runtimeDoctorView struct {
	Text                  string `json:"text"`
	PublishedGen          uint64 `json:"publishedGeneration"`
	AllowResume           bool   `json:"allowResume"`
	CleanRollback         bool   `json:"cleanRollback"`
	HasIrreversible       bool   `json:"hasIrreversible"`
	NoOpRebuilds          uint64 `json:"noOpRebuilds"`
	FullRebuilds          uint64 `json:"fullRebuilds"`
	SubgraphRebuilds      uint64 `json:"subgraphRebuilds"`
	StaleDrops            uint64 `json:"staleDrops"`
	AdmissionRejected     uint64 `json:"admissionRejected"`
	RuntimeOwnerFallbacks uint64 `json:"runtimeOwnerFallbacks"`
}

func collectRuntimeDoctorView() runtimeDoctorView {
	report := boot.CollectRuntimeDoctor(nil)
	return runtimeDoctorView{
		Text:                  boot.RenderRuntimeDoctorText(report),
		PublishedGen:          report.PublishedGen,
		AllowResume:           report.Resume.AllowResume,
		CleanRollback:         report.Resume.CleanRollback,
		HasIrreversible:       report.Resume.HasIrreversible,
		NoOpRebuilds:          report.Metrics.NoOpRebuilds,
		FullRebuilds:          report.Metrics.FullRebuilds,
		SubgraphRebuilds:      report.Metrics.SubgraphRebuilds,
		StaleDrops:            report.Metrics.StaleDrops,
		AdmissionRejected:     report.Metrics.AdmissionRejected,
		RuntimeOwnerFallbacks: report.RuntimeOwnerFallbacks,
	}
}

func (b *bridgeServer) runtimeDoctor(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, collectRuntimeDoctorView())
}
